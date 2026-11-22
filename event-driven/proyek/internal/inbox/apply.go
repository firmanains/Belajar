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

// Apply menerapkan satu revision ke write model dan melaporkan nasibnya.
//
// Versi ini benar hanya di dunia ideal: setiap revision datang tepat sekali,
// berurutan, dan tidak ada yang gagal di tengah jalan.
//
// TODO(step 1): revision_id yang sama hanya boleh diterapkan sekali. Pakai
// markReceived, dan kembalikan Duplicate untuk kiriman kedua.
//
// TODO(step 2): revision bisa mengubah atau membatalkan booking, dan bisa datang
// terlambat. Ambil state lama dengan store.WriteModel.LockAndGetBooking. Kalau
// revision_number tidak lebih besar dari state lama, kembalikan Stale. Kalau
// lebih besar, Release pemakaian kamar dari state lama sebelum Occupy yang baru.
// Semua langkah harus berada dalam satu transaction (s.pool.Begin), karena lock
// dari LockAndGetBooking hanya bertahan selama transaction itu.
//
// TODO(step 3): tulis event ke outbox dengan s.outbox.Insert di transaction yang
// sama. Buat envelope-nya dengan event.New dan tentukan namanya dengan eventName.
func (s *Service) Apply(ctx context.Context, rev Revision, correlationID string) (Result, error) {
	wm := store.WriteModel
	next := rev.Booking()
	if next.Status == event.StatusConfirmed {
		if err := wm.Occupy(ctx, s.pool, next); err != nil {
			return "", err
		}
	}
	if err := wm.SaveBooking(ctx, s.pool, next); err != nil {
		return "", err
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

// Dipakai mulai step 1 dan step 3. Hapus baris ini setelah keduanya terpakai.
var _, _ = markReceived, eventName
