package lab18

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// File ini sudah jadi. Jangan diubah.

// newTracing membuat Tracing yang menyimpan semua span yang sudah selesai di
// memory. Exporter hanya menerima span yang sudah di-End().
func newTracing(t *testing.T) (Tracing, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})
	return Tracing{
		Tracer:     provider.Tracer("belajar/eventdriven/lab18"),
		Propagator: propagation.TraceContext{},
	}, exporter
}

// fakeBroker menampung message yang dipublikasikan.
type fakeBroker struct {
	published []*message.Message
}

func (b *fakeBroker) Publish(topic string, messages ...*message.Message) error {
	b.published = append(b.published, messages...)
	return nil
}

// deliver meniru perjalanan message ke proses lain. Yang menyeberang hanya
// UUID, payload, dan metadata. context.Context milik pengirim tidak ikut.
func (b *fakeBroker) deliver(t *testing.T) *message.Message {
	t.Helper()
	if len(b.published) != 1 {
		t.Fatalf("ada %d message yang dipublikasikan, seharusnya 1", len(b.published))
	}
	sent := b.published[0]
	received := message.NewMessage(sent.UUID, append([]byte(nil), sent.Payload...))
	for key, value := range sent.Metadata {
		received.Metadata.Set(key, value)
	}
	return received
}

// publishFromHTTPRequest meniru handler HTTP "POST /bookings": ia punya span
// sendiri, lalu mempublikasikan event di dalam span itu.
func publishFromHTTPRequest(t *testing.T, tracing Tracing, broker *fakeBroker, bookingID string) trace.SpanContext {
	t.Helper()
	ctx, httpSpan := tracing.Tracer.Start(context.Background(), "POST /bookings", trace.WithSpanKind(trace.SpanKindServer))
	if err := tracing.PublishBookingCreated(ctx, broker, bookingID); err != nil {
		t.Fatalf("PublishBookingCreated gagal: %v", err)
	}
	httpSpan.End()
	return httpSpan.SpanContext()
}

func spansOfKind(spans tracetest.SpanStubs, kind trace.SpanKind) []tracetest.SpanStub {
	var found []tracetest.SpanStub
	for _, span := range spans {
		if span.SpanKind == kind {
			found = append(found, span)
		}
	}
	return found
}

func oneSpanOfKind(t *testing.T, spans tracetest.SpanStubs, kind trace.SpanKind) tracetest.SpanStub {
	t.Helper()
	found := spansOfKind(spans, kind)
	if len(found) != 1 {
		t.Fatalf("ada %d span dengan kind %s, seharusnya 1: span harus dibuat dengan kind yang benar dan ditutup dengan End()", len(found), kind)
	}
	return found[0]
}

func attributeValue(span tracetest.SpanStub, key string) string {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}

func consumerSpanOf(t *testing.T, spans tracetest.SpanStubs, handlerName string) tracetest.SpanStub {
	t.Helper()
	for _, span := range spansOfKind(spans, trace.SpanKindConsumer) {
		if attributeValue(span, "messaging.consumer.group.name") == handlerName {
			return span
		}
	}
	t.Fatalf("tidak ada consumer span dengan attribute messaging.consumer.group.name=%q", handlerName)
	return tracetest.SpanStub{}
}

// Dunia ideal: booking sampai ke handler. Tracing tidak boleh mengubah ini.
func TestStep0_MessageReachesHandler(t *testing.T) {
	tracing, _ := newTracing(t)
	broker := &fakeBroker{}
	publishFromHTTPRequest(t, tracing, broker, "B-1")

	var got string
	handler := tracing.Middleware(BookingCreatedTopic, "send_confirmation_email")(func(msg *message.Message) ([]*message.Message, error) {
		got = string(msg.Payload)
		return nil, nil
	})
	if _, err := handler(broker.deliver(t)); err != nil {
		t.Fatalf("handler gagal: %v", err)
	}
	if got != "B-1" {
		t.Fatalf("handler menerima %q, seharusnya B-1", got)
	}
}

// Publish adalah pekerjaan yang layak punya span sendiri, sebagai child dari
// span HTTP request.
func TestStep1_PublisherStartsProducerSpan(t *testing.T) {
	tracing, exporter := newTracing(t)
	broker := &fakeBroker{}
	httpSpan := publishFromHTTPRequest(t, tracing, broker, "B-1")

	producer := oneSpanOfKind(t, exporter.GetSpans(), trace.SpanKindProducer)
	if producer.SpanContext.TraceID() != httpSpan.TraceID() {
		t.Fatalf("producer span ada di trace lain: span harus dimulai dari ctx yang diterima, bukan context.Background()")
	}
	if producer.Parent.SpanID() != httpSpan.SpanID() {
		t.Fatalf("parent producer span bukan span POST /bookings")
	}
	if got := attributeValue(producer, "messaging.destination.name"); got != BookingCreatedTopic {
		t.Errorf("attribute messaging.destination.name = %q, seharusnya %q", got, BookingCreatedTopic)
	}
	if got := attributeValue(producer, "messaging.message.id"); got != broker.published[0].UUID {
		t.Errorf("attribute messaging.message.id = %q, seharusnya UUID message %q", got, broker.published[0].UUID)
	}
}

// context.Context tidak ikut menyeberang ke proses lain. Yang menyeberang
// hanya metadata, jadi trace context harus dititipkan di sana.
func TestStep2_TraceContextTravelsInMetadata(t *testing.T) {
	tracing, exporter := newTracing(t)
	broker := &fakeBroker{}
	httpSpan := publishFromHTTPRequest(t, tracing, broker, "B-1")
	producer := oneSpanOfKind(t, exporter.GetSpans(), trace.SpanKindProducer)

	traceparent := broker.deliver(t).Metadata.Get("traceparent")
	if traceparent == "" {
		t.Fatalf("metadata message tidak punya key traceparent: trace context belum di-inject")
	}
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		t.Fatalf("traceparent = %q, bentuknya seharusnya version-traceid-spanid-flags", traceparent)
	}
	if parts[1] != httpSpan.TraceID().String() {
		t.Fatalf("trace ID di traceparent = %s, seharusnya %s", parts[1], httpSpan.TraceID())
	}
	if parts[2] == httpSpan.SpanID().String() {
		t.Fatalf("span ID di traceparent adalah milik span POST /bookings: yang di-inject harus ctx hasil Tracer.Start, bukan ctx yang masuk")
	}
	if parts[2] != producer.SpanContext.SpanID().String() {
		t.Fatalf("span ID di traceparent = %s, seharusnya milik producer span %s", parts[2], producer.SpanContext.SpanID())
	}
}

// Di proses consumer, middleware membaca traceparent dan menyambung trace.
// Span yang dibuat di dalam handler harus ikut tersambung.
func TestStep3_ConsumerContinuesTheTrace(t *testing.T) {
	tracing, exporter := newTracing(t)
	broker := &fakeBroker{}
	httpSpan := publishFromHTTPRequest(t, tracing, broker, "B-1")

	handler := tracing.Middleware(BookingCreatedTopic, "send_confirmation_email")(func(msg *message.Message) ([]*message.Message, error) {
		// Handler memanggil SMTP dan memberi pekerjaan itu span sendiri.
		_, smtpSpan := tracing.Tracer.Start(msg.Context(), "smtp send")
		smtpSpan.End()
		return nil, nil
	})
	if _, err := handler(broker.deliver(t)); err != nil {
		t.Fatalf("handler gagal: %v", err)
	}

	spans := exporter.GetSpans()
	producer := oneSpanOfKind(t, spans, trace.SpanKindProducer)
	consumer := oneSpanOfKind(t, spans, trace.SpanKindConsumer)

	if consumer.SpanContext.TraceID() != httpSpan.TraceID() {
		t.Fatalf("consumer span ada di trace baru: trace context dari metadata belum di-extract")
	}
	if consumer.Parent.SpanID() != producer.SpanContext.SpanID() {
		t.Fatalf("parent consumer span bukan producer span")
	}
	if !consumer.Parent.IsRemote() {
		t.Fatalf("parent consumer span tidak bertanda remote: parent harus datang dari Propagator.Extract")
	}

	var smtp *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == "smtp send" {
			smtp = &spans[i]
		}
	}
	if smtp == nil {
		t.Fatalf("span smtp send tidak ditemukan")
	}
	if smtp.SpanContext.TraceID() != httpSpan.TraceID() || smtp.Parent.SpanID() != consumer.SpanContext.SpanID() {
		t.Fatalf("span smtp send tidak menjadi child consumer span: context berisi span harus dipasang ke message dengan msg.SetContext")
	}
}

// Satu booking, dua handler di dua consumer group. Satu gagal. Semuanya
// harus terlihat sebagai satu trace, dan yang gagal harus bertanda error.
func TestStep4_OneTraceAcrossHandlersWithErrors(t *testing.T) {
	tracing, exporter := newTracing(t)
	broker := &fakeBroker{}
	httpSpan := publishFromHTTPRequest(t, tracing, broker, "B-1")

	errSMTP := errors.New("SMTP timeout")
	email := tracing.Middleware(BookingCreatedTopic, "send_confirmation_email")(func(msg *message.Message) ([]*message.Message, error) {
		return nil, errSMTP
	})
	channel := tracing.Middleware(BookingCreatedTopic, "sync_channel_manager")(func(msg *message.Message) ([]*message.Message, error) {
		return nil, nil
	})

	if _, err := email(broker.deliver(t)); !errors.Is(err, errSMTP) {
		t.Fatalf("error handler email = %v, seharusnya diteruskan apa adanya supaya message di-nack", err)
	}
	if _, err := channel(broker.deliver(t)); err != nil {
		t.Fatalf("handler channel gagal: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 4 {
		t.Fatalf("ada %d span, seharusnya 4: POST /bookings, publish, dan dua process", len(spans))
	}
	for _, span := range spans {
		if span.SpanContext.TraceID() != httpSpan.TraceID() {
			t.Fatalf("span %q ada di trace lain: satu booking harus menjadi satu trace", span.Name)
		}
	}

	emailSpan := consumerSpanOf(t, spans, "send_confirmation_email")
	if emailSpan.Status.Code != codes.Error {
		t.Errorf("status span handler email = %v, seharusnya Error: RecordError saja tidak mengubah status", emailSpan.Status.Code)
	}
	recorded := false
	for _, event := range emailSpan.Events {
		if event.Name == "exception" {
			recorded = true
		}
	}
	if !recorded {
		t.Errorf("span handler email tidak punya event exception: error belum dicatat dengan RecordError")
	}

	channelSpan := consumerSpanOf(t, spans, "sync_channel_manager")
	if channelSpan.Status.Code == codes.Error {
		t.Errorf("span handler channel bertanda Error padahal handler berhasil")
	}
	if got := attributeValue(channelSpan, "messaging.destination.name"); got != BookingCreatedTopic {
		t.Errorf("attribute messaging.destination.name di consumer span = %q, seharusnya %q", got, BookingCreatedTopic)
	}
}
