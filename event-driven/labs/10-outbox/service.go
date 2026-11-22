package lab10

// File ini kamu ubah di Step 1 dan Step 2.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BookingService menyimpan booking dan mengabarkan BookingCreated.
type BookingService struct {
	pool *pgxpool.Pool
	pub  Publisher
}

func NewBookingService(pool *pgxpool.Pool, pub Publisher) *BookingService {
	return &BookingService{pool: pool, pub: pub}
}

// CreateBooking versi dual write: commit ke database, lalu publish ke broker.
// Kalau broker gagal di antara keduanya, booking tersimpan tapi event-nya hilang.
func (s *BookingService) CreateBooking(ctx context.Context, b Booking) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO bookings (id, room_type, rooms) VALUES ($1, $2, $3)`,
		b.ID, b.RoomType, b.Rooms)
	if err != nil {
		return fmt.Errorf("insert booking %s: %w", b.ID, err)
	}

	payload, err := json.Marshal(BookingCreated{BookingID: b.ID, RoomType: b.RoomType, Rooms: b.Rooms})
	if err != nil {
		return fmt.Errorf("marshal BookingCreated %s: %w", b.ID, err)
	}
	if err := s.pub.Publish(ctx, Message{ID: b.ID, Topic: TopicBookingCreated, Payload: payload}); err != nil {
		return fmt.Errorf("publish BookingCreated %s: %w", b.ID, err)
	}
	return nil
}
