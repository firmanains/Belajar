package lab04

// File ini sudah jadi. Jangan diubah; baca sebagai pembanding.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
)

// TopicBookingCreated adalah topic tempat event BookingCreated diterbitkan.
const TopicBookingCreated = "booking.created"

// BookingCreated adalah payload event di topic booking.created.
type BookingCreated struct {
	BookingID string `json:"booking_id"`
}

// Emailer mengirim confirmation email. Bisa lambat, bisa gagal.
type Emailer interface {
	SendConfirmation(ctx context.Context, bookingID string) error
}

// ConsumeManually adalah consume loop yang ditulis tangan, seperti di lab 02.
// Semua yang ada di sini selain dua baris di tengah adalah pekerjaan rutin:
// subscribe, loop, ack, nack. Router Watermill mengambil alih pekerjaan itu.
func ConsumeManually(ctx context.Context, sub message.Subscriber, email Emailer) error {
	messages, err := sub.Subscribe(ctx, TopicBookingCreated)
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", TopicBookingCreated, err)
	}
	for msg := range messages {
		var event BookingCreated
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			msg.Nack()
			continue
		}
		if err := email.SendConfirmation(msg.Context(), event.BookingID); err != nil {
			msg.Nack() // minta dikirim ulang
			continue
		}
		msg.Ack() // sudah diproses, jangan kirim lagi
	}
	return nil
}
