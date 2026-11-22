package lab05

// File ini sudah jadi. Jangan diubah; baca saja.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

const (
	TopicBookingCreated = "booking.created"
	TopicEmailRequested = "email.confirmation-requested"

	HandlerReserveRooms = "reserve-rooms"
)

// BookingCreated adalah event yang masuk dari channel manager.
type BookingCreated struct {
	BookingID  string `json:"booking_id"`
	RoomType   string `json:"room_type"`
	Rooms      int    `json:"rooms"`
	GuestEmail string `json:"guest_email"`
}

// SendConfirmationEmail adalah message keluar untuk layanan email.
type SendConfirmationEmail struct {
	BookingID string `json:"booking_id"`
	To        string `json:"to"`
}

// Availability adalah layanan lain yang dipanggil handler. Di production ini
// HTTP call atau query database, dan ctx ikut dibawa ke sana.
type Availability interface {
	Decrease(ctx context.Context, roomType string, rooms int) error
}

// PubSub adalah gabungan Publisher dan Subscriber; gochannel memenuhi keduanya.
type PubSub interface {
	message.Publisher
	message.Subscriber
}

// NewRouter merakit satu handler: terima BookingCreated, kurangi availability,
// lalu terbitkan SendConfirmationEmail. Handler tidak tahu apa-apa soal
// correlation ID atau logging; itu urusan middleware di middleware.go.
func NewRouter(pubSub PubSub, availability Availability, logger *slog.Logger) (*message.Router, error) {
	router, err := message.NewRouter(message.RouterConfig{}, watermill.NopLogger{})
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	router.AddMiddleware(Middlewares(logger)...)

	router.AddHandler(
		HandlerReserveRooms,
		TopicBookingCreated, pubSub,
		TopicEmailRequested, pubSub,
		func(msg *message.Message) ([]*message.Message, error) {
			var event BookingCreated
			if err := json.Unmarshal(msg.Payload, &event); err != nil {
				return nil, fmt.Errorf("decode BookingCreated: %w", err)
			}

			if err := availability.Decrease(msg.Context(), event.RoomType, event.Rooms); err != nil {
				return nil, fmt.Errorf("decrease availability for booking %s: %w", event.BookingID, err)
			}

			payload, err := json.Marshal(SendConfirmationEmail{BookingID: event.BookingID, To: event.GuestEmail})
			if err != nil {
				return nil, fmt.Errorf("encode SendConfirmationEmail: %w", err)
			}
			return []*message.Message{message.NewMessage(watermill.NewUUID(), payload)}, nil
		},
	)

	return router, nil
}
