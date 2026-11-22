package inbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/outbox"
	"belajar/eventdriven/proyek/internal/platform"
	"belajar/eventdriven/proyek/internal/store"
)

// Service menerapkan revision ke write model.
type Service struct {
	pool   *pgxpool.Pool
	outbox *outbox.Store
}

func NewService(pool *pgxpool.Pool, ob *outbox.Store) *Service {
	return &Service{pool: pool, outbox: ob}
}

func (s *Service) roomTypeExists(ctx context.Context, roomType string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM room_types WHERE room_type = $1)`, roomType).Scan(&ok)
	return ok, err
}

// markReceived mencatat revision_id di tabel inbox. Hasil false berarti
// revision ini sudah pernah dicatat.
func markReceived(ctx context.Context, q platform.DBTX, revisionID string) (bool, error) {
	tag, err := q.Exec(ctx, `INSERT INTO inbox (revision_id) VALUES ($1) ON CONFLICT DO NOTHING`, revisionID)
	if err != nil {
		return false, fmt.Errorf("insert inbox: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// Apply menerapkan satu revision. Empat hal terjadi dalam satu transaction,
// sehingga semuanya tersimpan atau tidak ada yang tersimpan: catatan inbox,
// baris booking, pemakaian kamar, dan baris outbox.
func (s *Service) Apply(ctx context.Context, rev Revision, correlationID string) (Result, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // tidak berefek setelah Commit

	// Idempotent: revision_id yang sama hanya diterapkan sekali. Catatan ini
	// ikut rollback kalau langkah di bawah gagal, sehingga kiriman ulang dari
	// channel manager tidak dianggap duplicate.
	fresh, err := markReceived(ctx, tx, rev.RevisionID)
	if err != nil {
		return "", err
	}
	if !fresh {
		return Duplicate, nil
	}

	wm := store.WriteModel
	old, err := wm.LockAndGetBooking(ctx, tx, rev.BookingID)
	if err != nil {
		return "", err
	}
	// Stale: revision yang lebih baru sudah diterapkan. Commit tetap dilakukan
	// supaya catatan inbox-nya tersimpan.
	if old != nil && rev.RevisionNumber <= old.Revision {
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit: %w", err)
		}
		return Stale, nil
	}

	// Lepas pemakaian kamar dari state lama, lalu pasang dari state baru.
	next := rev.Booking()
	if old != nil && old.Status == event.StatusConfirmed {
		if err := wm.Release(ctx, tx, *old); err != nil {
			return "", err
		}
	}
	if next.Status == event.StatusConfirmed {
		if err := wm.Occupy(ctx, tx, next); err != nil {
			return "", err
		}
	}
	if err := wm.SaveBooking(ctx, tx, next); err != nil {
		return "", err
	}

	if err := s.outbox.Insert(ctx, tx, event.New(eventName(old, next), correlationID, next)); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return Applied, nil
}

// eventName ditentukan dari apa yang berubah di sini, bukan dari label yang
// dikirim channel manager: revision "modified" yang tiba lebih dulu daripada
// revision "new" tetap menghasilkan BookingCreated.
func eventName(old *event.Booking, next event.Booking) string {
	switch {
	case next.Status == event.StatusCancelled:
		return event.BookingCancelled
	case old == nil:
		return event.BookingCreated
	default:
		return event.BookingModified
	}
}
