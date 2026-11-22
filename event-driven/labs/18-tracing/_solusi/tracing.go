package lab18

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

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

// messagingAttributes mengikuti nama attribute dari OpenTelemetry semantic
// conventions untuk messaging, supaya backend tracing mengenalinya.
func messagingAttributes(topic, messageID, operation string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.destination.name", topic),
		attribute.String("messaging.message.id", messageID),
		attribute.String("messaging.operation.type", operation),
	}
}

// PublishBookingCreated mempublikasikan event BookingCreated di dalam sebuah
// producer span, dan menitipkan trace context span itu ke metadata message.
func (t Tracing) PublishBookingCreated(ctx context.Context, pub Publisher, bookingID string) error {
	msg := message.NewMessage(watermill.NewUUID(), []byte(bookingID))

	ctx, span := t.Tracer.Start(ctx, "publish "+BookingCreatedTopic,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(messagingAttributes(BookingCreatedTopic, msg.UUID, "publish")...),
	)
	defer span.End()

	// Yang dititipkan adalah ctx hasil Start, supaya consumer menjadi child
	// dari producer span, bukan dari span HTTP request.
	t.Propagator.Inject(ctx, propagation.MapCarrier(msg.Metadata))

	if err := pub.Publish(BookingCreatedTopic, msg); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("publish %s: %w", BookingCreatedTopic, err)
	}
	return nil
}

// Middleware membaca trace context dari metadata, membuat consumer span
// sebagai child-nya, dan menaruh span itu di context message supaya span lain
// di dalam handler ikut tersambung.
func (t Tracing) Middleware(topic, handlerName string) message.HandlerMiddleware {
	return func(next message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			ctx := t.Propagator.Extract(msg.Context(), propagation.MapCarrier(msg.Metadata))

			attrs := append(messagingAttributes(topic, msg.UUID, "process"),
				attribute.String("messaging.consumer.group.name", handlerName))
			ctx, span := t.Tracer.Start(ctx, "process "+topic,
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithAttributes(attrs...),
			)
			defer span.End()
			msg.SetContext(ctx)

			produced, err := next(msg)
			if err != nil {
				// RecordError hanya menambah event; status harus diisi terpisah.
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return produced, err
		}
	}
}
