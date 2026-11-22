package lab18

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// File ini yang kamu ubah.

// BookingCreatedTopic adalah topic tempat event BookingCreated dipublikasikan.
const BookingCreatedTopic = "pms.booking.created"

// Publisher adalah sisi kirim dari broker.
type Publisher interface {
	Publish(topic string, messages ...*message.Message) error
}

// Tracing memegang dua alat OpenTelemetry yang dibutuhkan lab ini: Tracer
// untuk membuat span, dan Propagator untuk menulis dan membaca trace context
// di metadata message.
type Tracing struct {
	Tracer     trace.Tracer
	Propagator propagation.TextMapPropagator
}

// PublishBookingCreated mempublikasikan event BookingCreated. ctx datang dari
// HTTP request yang membuat booking. Versi ini belum membuat span dan belum
// menitipkan trace context ke message.
func (t Tracing) PublishBookingCreated(ctx context.Context, pub Publisher, bookingID string) error {
	msg := message.NewMessage(watermill.NewUUID(), []byte(bookingID))
	if err := pub.Publish(BookingCreatedTopic, msg); err != nil {
		return fmt.Errorf("publish %s: %w", BookingCreatedTopic, err)
	}
	return nil
}

// Middleware membungkus handler di sisi consumer. handlerName dipakai sebagai
// nama consumer group. Versi ini belum membaca trace context dan belum
// membuat span.
func (t Tracing) Middleware(topic, handlerName string) message.HandlerMiddleware {
	return func(next message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			return next(msg)
		}
	}
}
