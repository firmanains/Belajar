package readmodel

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/obs"
	"belajar/eventdriven/proyek/internal/platform"
	"belajar/eventdriven/proyek/internal/store"
)

// Projector membangun rm_bookings dan rm_tape_chart dari event booking.
type Projector struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewProjector(pool *pgxpool.Pool, log *slog.Logger) *Projector {
	return &Projector{pool: pool, log: log}
}

// Handle menerapkan satu event ke read model. Event bisa datang dobel dan bisa
// datang terlambat, jadi keputusan "terapkan atau abaikan" diambil dari nomor
// revision, bukan dari urutan kedatangan.
func (p *Projector) Handle(ctx context.Context, r platform.Record) error {
	log := obs.LoggerFrom(ctx, p.log)

	env, err := event.Decode(r.Value)
	if err != nil {
		// Malformed: diulang pun tidak akan berhasil. Dilewati supaya
		// partition ini tidak macet.
		log.Warn("event malformed dilewati", "error", err)
		return nil
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // tidak berefek setelah Commit

	rm := store.ReadModel
	old, err := rm.LockAndGetBooking(ctx, tx, env.Booking.BookingID)
	if err != nil {
		return err
	}
	// Revision yang sama (duplicate) atau lebih lama (stale) diabaikan.
	if old != nil && env.Booking.Revision <= old.Revision {
		log.Info("event diabaikan", "booking_id", old.BookingID, "revision", env.Booking.Revision, "current_revision", old.Revision)
		return nil
	}

	if old != nil && old.Status == event.StatusConfirmed {
		if err := rm.Release(ctx, tx, *old); err != nil {
			return err
		}
	}
	if env.Booking.Status == event.StatusConfirmed {
		if err := rm.Occupy(ctx, tx, env.Booking); err != nil {
			return err
		}
	}
	if err := rm.SaveBooking(ctx, tx, env.Booking); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
