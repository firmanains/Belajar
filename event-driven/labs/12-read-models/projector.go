package lab12

// File ini kamu ubah di Step 1 sampai Step 3.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Projector mengubah event menjadi baris read model.
type Projector struct {
	pool *pgxpool.Pool
}

func NewProjector(pool *pgxpool.Pool) *Projector {
	return &Projector{pool: pool}
}

// Handle memproyeksikan satu event. Versi awal hanya mengisi booking_list_view
// untuk BookingCreated.
func (p *Projector) Handle(ctx context.Context, env Envelope) error {
	switch e := env.Event.(type) {
	case BookingCreated:
		// Upsert: memproyeksikan booking yang sama dua kali tidak menambah baris.
		_, err := p.pool.Exec(ctx, `
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
			return fmt.Errorf("proyeksikan BookingCreated %s: %w", e.BookingID, err)
		}
		return nil
	case BookingCancelled:
		// Belum diproyeksikan.
		return nil
	default:
		return fmt.Errorf("event tidak dikenal: %T", env.Event)
	}
}
