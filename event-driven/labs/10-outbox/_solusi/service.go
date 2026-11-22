package lab10

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BookingService menyimpan booking dan mengabarkan BookingCreated.
type BookingService struct {
	pool *pgxpool.Pool
}

// NewBookingService masih menerima Publisher supaya test tidak berubah, tetapi
// service tidak lagi memakainya: yang berbicara dengan broker hanya Forwarder.
func NewBookingService(pool *pgxpool.Pool, _ Publisher) *BookingService {
	return &BookingService{pool: pool}
}

// CreateBooking menulis booking dan baris outbox dalam satu transaction.
// Keduanya tersimpan bersama atau tidak sama sekali.
func (s *BookingService) CreateBooking(ctx context.Context, b Booking) error {
	payload, err := json.Marshal(BookingCreated{BookingID: b.ID, RoomType: b.RoomType, Rooms: b.Rooms})
	if err != nil {
		return fmt.Errorf("marshal BookingCreated %s: %w", b.ID, err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	// Rollback setelah Commit berhasil tidak berbuat apa-apa, jadi error-nya diabaikan.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO bookings (id, room_type, rooms) VALUES ($1, $2, $3)`,
		b.ID, b.RoomType, b.Rooms); err != nil {
		return fmt.Errorf("insert booking %s: %w", b.ID, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO outbox (topic, payload) VALUES ($1, $2)`,
		TopicBookingCreated, payload); err != nil {
		return fmt.Errorf("insert outbox untuk booking %s: %w", b.ID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit booking %s: %w", b.ID, err)
	}
	return nil
}
