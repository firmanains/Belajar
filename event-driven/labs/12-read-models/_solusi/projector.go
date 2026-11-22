package lab12

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Projector mengubah event menjadi baris read model.
type Projector struct {
	pool *pgxpool.Pool
}

func NewProjector(pool *pgxpool.Pool) *Projector {
	return &Projector{pool: pool}
}

// Handle memproyeksikan satu event dalam satu transaction: catat event ID,
// ubah read model, commit. Kalau event ID sudah tercatat, tidak ada yang diubah.
func (p *Projector) Handle(ctx context.Context, env Envelope) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	// Rollback setelah Commit berhasil tidak berbuat apa-apa, jadi error-nya diabaikan.
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT (event_id) DO NOTHING`,
		env.EventID)
	if err != nil {
		return fmt.Errorf("catat event %s: %w", env.EventID, err)
	}
	if tag.RowsAffected() == 0 {
		return nil // duplikat: event ini sudah diproyeksikan
	}

	switch e := env.Event.(type) {
	case BookingCreated:
		err = projectCreated(ctx, tx, e)
	case BookingCancelled:
		err = projectCancelled(ctx, tx, e)
	default:
		err = fmt.Errorf("event tidak dikenal: %T", env.Event)
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit event %s: %w", env.EventID, err)
	}
	return nil
}

func projectCreated(ctx context.Context, tx pgx.Tx, e BookingCreated) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO booking_list_view (booking_id, guest_name, room_type, check_in, check_out, rooms, status)
		VALUES ($1, $2, $3, $4::date, $5::date, $6, $7)
		ON CONFLICT (booking_id) DO UPDATE SET
			guest_name = EXCLUDED.guest_name,
			room_type  = EXCLUDED.room_type,
			check_in   = EXCLUDED.check_in,
			check_out  = EXCLUDED.check_out,
			rooms      = EXCLUDED.rooms`,
		e.BookingID, e.GuestName, e.RoomType, e.CheckIn, e.CheckOut, e.Rooms, StatusConfirmed)
	if err != nil {
		return fmt.Errorf("proyeksikan BookingCreated %s ke booking_list_view: %w", e.BookingID, err)
	}

	// Satu baris per malam menginap. CheckOut bukan malam menginap.
	_, err = tx.Exec(ctx, `
		INSERT INTO availability_view (room_type, day, rooms_sold)
		SELECT $1::text, d::date, $4::int
		FROM generate_series($2::date, $3::date - 1, interval '1 day') AS d
		ON CONFLICT (room_type, day) DO UPDATE
			SET rooms_sold = availability_view.rooms_sold + EXCLUDED.rooms_sold`,
		e.RoomType, e.CheckIn, e.CheckOut, e.Rooms)
	if err != nil {
		return fmt.Errorf("proyeksikan BookingCreated %s ke availability_view: %w", e.BookingID, err)
	}
	return nil
}

// projectCancelled melepas kamar hanya kalau status booking berubah dari
// confirmed ke cancelled di statement ini. Pembatalan kedua untuk booking yang
// sama, dengan event ID apa pun, tidak menemukan baris dan tidak melepas apa-apa.
func projectCancelled(ctx context.Context, tx pgx.Tx, e BookingCancelled) error {
	_, err := tx.Exec(ctx, `
		WITH cancelled AS (
			UPDATE booking_list_view
			SET status = $2
			WHERE booking_id = $1 AND status = $3
			RETURNING room_type, check_in, check_out, rooms
		)
		UPDATE availability_view v
		SET rooms_sold = v.rooms_sold - c.rooms
		FROM cancelled c
		WHERE v.room_type = c.room_type AND v.day >= c.check_in AND v.day < c.check_out`,
		e.BookingID, StatusCancelled, StatusConfirmed)
	if err != nil {
		return fmt.Errorf("proyeksikan BookingCancelled %s: %w", e.BookingID, err)
	}
	return nil
}
