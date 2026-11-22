package lab05

// File ini sudah jadi. Jangan diubah.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

const waitTimeout = 2 * time.Second

// fakeAvailability merekam correlation ID yang terlihat di context setiap call.
type fakeAvailability struct {
	mu        sync.Mutex
	failFirst int
	calls     int
	decreased int
	seenIDs   []string
}

func (f *fakeAvailability) Decrease(ctx context.Context, _ string, rooms int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.seenIDs = append(f.seenIDs, CorrelationIDFromContext(ctx))
	if f.calls <= f.failFirst {
		return errors.New("availability service timeout")
	}
	f.decreased += rooms
	return nil
}

func (f *fakeAvailability) snapshot() (decreased int, seenIDs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.decreased, append([]string(nil), f.seenIDs...)
}

// logBuffer adalah io.Writer yang aman dipakai dari banyak goroutine.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// records mengurai log JSON menjadi satu map per baris.
func (b *logBuffer) records(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	raw := b.buf.String()
	b.mu.Unlock()

	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("baris log bukan JSON: %q", line)
		}
		out = append(out, rec)
	}
	return out
}

func (b *logBuffer) withMsg(t *testing.T, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, rec := range b.records(t) {
		if rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

type harness struct {
	pubSub       *gochannel.GoChannel
	availability *fakeAvailability
	logs         *logBuffer

	mu     sync.Mutex
	emails []*message.Message
}

func (h *harness) producedEmails() []*message.Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*message.Message(nil), h.emails...)
}

// start menjalankan router sungguhan di atas gochannel dan menunggu sampai siap.
func start(t *testing.T, availability *fakeAvailability) *harness {
	t.Helper()
	h := &harness{
		pubSub:       gochannel.NewGoChannel(gochannel.Config{}, watermill.NopLogger{}),
		availability: availability,
		logs:         &logBuffer{},
	}
	ctx, cancel := context.WithCancel(context.Background())

	// gochannel membuang message yang terbit sebelum ada subscriber, jadi
	// penampung hasil harus subscribe lebih dulu.
	out, err := h.pubSub.Subscribe(ctx, TopicEmailRequested)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	go func() {
		for msg := range out {
			h.mu.Lock()
			h.emails = append(h.emails, msg)
			h.mu.Unlock()
			msg.Ack()
		}
	}()

	logger := slog.New(slog.NewJSONHandler(h.logs, nil))
	router, err := NewRouter(h.pubSub, availability, logger)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	go func() {
		if err := router.Run(ctx); err != nil {
			t.Errorf("router.Run: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		if err := router.Close(); err != nil {
			t.Errorf("router.Close: %v", err)
		}
	})

	select {
	case <-router.Running():
	case <-time.After(waitTimeout):
		t.Fatal("router tidak siap dalam 2 detik")
	}
	return h
}

func (h *harness) publishBooking(t *testing.T, correlationID string) *message.Message {
	t.Helper()
	payload, err := json.Marshal(BookingCreated{BookingID: "B-1", RoomType: "deluxe", Rooms: 2, GuestEmail: "tamu@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	msg := message.NewMessage(watermill.NewUUID(), payload)
	if correlationID != "" {
		msg.Metadata.Set(CorrelationIDMetadataKey, correlationID)
	}
	if err := h.pubSub.Publish(TopicBookingCreated, msg); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return msg
}

// waitFor memeriksa cond berulang-ulang sampai true atau waktu habis.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s: tidak terjadi dalam %s", what, waitTimeout)
}

func (h *harness) waitForEmail(t *testing.T) *message.Message {
	t.Helper()
	waitFor(t, "message SendConfirmationEmail terbit", func() bool { return len(h.producedEmails()) == 1 })
	return h.producedEmails()[0]
}

// Dunia tanpa middleware: handler tetap bekerja. Test ini lulus dari awal.
func TestStep0_HandlerWorksWithoutMiddleware(t *testing.T) {
	h := start(t, &fakeAvailability{})
	h.publishBooking(t, "req-123")

	out := h.waitForEmail(t)
	var cmd SendConfirmationEmail
	if err := json.Unmarshal(out.Payload, &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.BookingID != "B-1" || cmd.To != "tamu@example.com" {
		t.Fatalf("isi SendConfirmationEmail salah: %+v", cmd)
	}
	if decreased, _ := h.availability.snapshot(); decreased != 2 {
		t.Fatalf("availability berkurang %d, seharusnya 2", decreased)
	}
}

// Correlation ID di message masuk harus ikut ke message keluar.
func TestStep1_CorrelationIDIsPropagatedToProducedMessages(t *testing.T) {
	h := start(t, &fakeAvailability{})
	h.publishBooking(t, "req-123")

	out := h.waitForEmail(t)
	if got := out.Metadata.Get(CorrelationIDMetadataKey); got != "req-123" {
		t.Fatalf("correlation_id di message keluar = %q, seharusnya %q: jejak request putus di handler ini", got, "req-123")
	}
}

// Handler dan semua yang ia panggil harus bisa membaca ID dari context.
// Kalau message masuk tidak membawa ID, middleware membuatkan satu.
func TestStep2_CorrelationIDIsAvailableInContext(t *testing.T) {
	t.Run("ID dari message masuk", func(t *testing.T) {
		h := start(t, &fakeAvailability{})
		h.publishBooking(t, "req-123")
		h.waitForEmail(t)

		_, seen := h.availability.snapshot()
		if len(seen) != 1 || seen[0] != "req-123" {
			t.Fatalf("Availability.Decrease melihat correlation ID %q di context, seharusnya [\"req-123\"]", seen)
		}
	})

	t.Run("message masuk tanpa ID", func(t *testing.T) {
		h := start(t, &fakeAvailability{})
		h.publishBooking(t, "")
		out := h.waitForEmail(t)

		_, seen := h.availability.snapshot()
		if len(seen) != 1 || seen[0] == "" {
			t.Fatalf("Availability.Decrease melihat correlation ID %q di context, seharusnya ID baru yang tidak kosong", seen)
		}
		if got := out.Metadata.Get(CorrelationIDMetadataKey); got != seen[0] {
			t.Fatalf("message keluar membawa correlation_id %q, tetapi context berisi %q: keduanya harus sama", got, seen[0])
		}
	})
}

// Setiap message menghasilkan baris log terstruktur di awal dan di akhir.
func TestStep3_LoggingMiddlewareWritesStructuredRecords(t *testing.T) {
	h := start(t, &fakeAvailability{failFirst: 1})
	in := h.publishBooking(t, "req-123")
	h.waitForEmail(t)
	waitFor(t, `baris log "message handled"`, func() bool { return len(h.logs.withMsg(t, "message handled")) == 1 })

	if got := len(h.logs.withMsg(t, "message received")); got != 2 {
		t.Fatalf(`baris "message received" = %d, seharusnya 2 (percobaan pertama gagal, lalu message dikirim ulang)`, got)
	}

	failed := h.logs.withMsg(t, "message failed")
	if len(failed) != 1 {
		t.Fatalf(`baris "message failed" = %d, seharusnya 1`, len(failed))
	}
	if failed[0]["level"] != "ERROR" {
		t.Fatalf(`level baris "message failed" = %v, seharusnya ERROR`, failed[0]["level"])
	}
	if errText, _ := failed[0]["error"].(string); !strings.Contains(errText, "availability service timeout") {
		t.Fatalf(`field "error" = %q, seharusnya memuat penyebab aslinya`, errText)
	}

	for _, rec := range h.logs.records(t) {
		if rec["message_uuid"] != in.UUID {
			t.Fatalf(`field "message_uuid" = %v di baris %q, seharusnya %s`, rec["message_uuid"], rec["msg"], in.UUID)
		}
		if rec["handler"] != HandlerReserveRooms {
			t.Fatalf(`field "handler" = %v di baris %q, seharusnya %s`, rec["handler"], rec["msg"], HandlerReserveRooms)
		}
	}
}

// Baris log paling awal pun harus sudah membawa correlation ID. Ini hanya
// mungkin kalau urutan middleware benar.
func TestStep4_MiddlewareOrderMatters(t *testing.T) {
	h := start(t, &fakeAvailability{})
	h.publishBooking(t, "")
	out := h.waitForEmail(t)
	waitFor(t, `baris log "message handled"`, func() bool { return len(h.logs.withMsg(t, "message handled")) == 1 })

	want := out.Metadata.Get(CorrelationIDMetadataKey)
	if want == "" {
		t.Fatal("message keluar tidak membawa correlation_id; selesaikan Step 2 dulu")
	}
	records := h.logs.records(t)
	if len(records) != 2 {
		t.Fatalf("jumlah baris log = %d, seharusnya 2 (received dan handled)", len(records))
	}
	for _, rec := range records {
		if rec["correlation_id"] != want {
			t.Fatalf(`baris %q membawa correlation_id %v, seharusnya %q: Logging berjalan sebelum CorrelationID sempat membuat ID`, rec["msg"], rec["correlation_id"], want)
		}
	}
}
