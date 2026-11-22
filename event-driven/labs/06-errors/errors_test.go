package lab06

// File ini sudah jadi. Jangan diubah.

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

const waitTimeout = 2 * time.Second

var testRetry = RetryConfig{MaxAttempts: 3, InitialInterval: 20 * time.Millisecond, Multiplier: 2}

// fakeEmailer meniru SMTP: bisa gagal beberapa kali lalu pulih, atau mati
// total untuk satu alamat.
type fakeEmailer struct {
	mu        sync.Mutex
	failFirst int
	downFor   string
	calls     map[string][]time.Time
	sent      []string
}

func (f *fakeEmailer) Send(_ context.Context, to, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string][]time.Time{}
	}
	f.calls[to] = append(f.calls[to], time.Now())
	if to == f.downFor || len(f.calls[to]) <= f.failFirst {
		return ErrSMTPTimeout
	}
	f.sent = append(f.sent, to)
	return nil
}

func (f *fakeEmailer) sentTo(to string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, s := range f.sent {
		if s == to {
			n++
		}
	}
	return n
}

func (f *fakeEmailer) callTimes(to string) []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.calls[to]...)
}

type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// withLevel mengembalikan baris log JSON pada level tertentu untuk satu message.
func (b *logBuffer) withLevel(t *testing.T, level, messageUUID string) []map[string]any {
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
		if rec["level"] == level && rec["message_uuid"] == messageUUID {
			out = append(out, rec)
		}
	}
	return out
}

type harness struct {
	broker  *Broker
	emailer *fakeEmailer
	lot     *ParkingLot
	logs    *logBuffer

	mu       sync.Mutex
	attempts map[string]int // message UUID -> berapa kali handler dipanggil
}

func (h *harness) attemptsFor(uuid string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.attempts[uuid]
}

func start(t *testing.T, emailer *fakeEmailer) *harness {
	t.Helper()
	h := &harness{
		broker:   NewBroker(),
		emailer:  emailer,
		lot:      &ParkingLot{},
		logs:     &logBuffer{},
		attempts: map[string]int{},
	}

	inner := EmailHandler{Emailer: emailer}.Handle
	counting := func(msg *message.Message) error {
		h.mu.Lock()
		h.attempts[msg.UUID]++
		h.mu.Unlock()
		return inner(msg)
	}

	router, err := NewRouter(h.broker, counting, testRetry, h.lot, slog.New(slog.NewJSONHandler(h.logs, nil)))
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
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
		_ = h.broker.Close()
	})

	select {
	case <-router.Running():
	case <-time.After(waitTimeout):
		t.Fatal("router tidak siap dalam 2 detik")
	}
	return h
}

func (h *harness) publishRaw(t *testing.T, payload string) *message.Message {
	t.Helper()
	msg := message.NewMessage(watermill.NewUUID(), []byte(payload))
	if err := h.broker.Publish(TopicEmailRequested, msg); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return msg
}

func (h *harness) publishEmail(t *testing.T, bookingID, to string) *message.Message {
	t.Helper()
	payload, err := json.Marshal(SendConfirmationEmail{BookingID: bookingID, To: to, Guest: &Guest{Name: "Sari"}})
	if err != nil {
		t.Fatal(err)
	}
	return h.publishRaw(t, string(payload))
}

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

// waitForNextEmail menunggu email untuk message yang antre di belakang
// message bermasalah. Kalau ini gagal, antrean tersumbat.
func (h *harness) waitForNextEmail(t *testing.T) {
	t.Helper()
	waitFor(t, "email untuk message berikutnya (B-2) terkirim; antrean tersumbat oleh message di depannya", func() bool {
		return h.emailer.sentTo("berikutnya@example.com") == 1
	})
}

func (h *harness) assertParked(t *testing.T, msg *message.Message, reasonContains string) {
	t.Helper()
	parked := h.lot.Messages()
	if len(parked) != 1 {
		t.Fatalf("isi ParkingLot = %d message, seharusnya 1", len(parked))
	}
	if parked[0].UUID != msg.UUID {
		t.Fatalf("message yang diparkir = %s, seharusnya %s", parked[0].UUID, msg.UUID)
	}
	if !strings.Contains(strings.ToLower(parked[0].Reason), reasonContains) {
		t.Fatalf("alasan parkir = %q, seharusnya memuat %q", parked[0].Reason, reasonContains)
	}
}

// Dunia ideal: SMTP sehat, payload benar. Test ini lulus dari awal.
func TestStep0_HealthyMessageIsSent(t *testing.T) {
	h := start(t, &fakeEmailer{})
	h.publishEmail(t, "B-1", "tamu@example.com")

	waitFor(t, "email terkirim", func() bool { return h.emailer.sentTo("tamu@example.com") == 1 })
}

// SMTP timeout dua kali lalu pulih. Email harus terkirim, dan jeda antar
// percobaan harus makin panjang.
func TestStep1_TransientErrorIsRetriedWithBackoff(t *testing.T) {
	h := start(t, &fakeEmailer{failFirst: 2})
	h.publishEmail(t, "B-1", "tamu@example.com")

	waitFor(t, "email terkirim setelah SMTP pulih", func() bool { return h.emailer.sentTo("tamu@example.com") == 1 })

	calls := h.emailer.callTimes("tamu@example.com")
	if len(calls) != 3 {
		t.Fatalf("SMTP dipanggil %d kali, seharusnya 3", len(calls))
	}
	if gap := calls[1].Sub(calls[0]); gap < testRetry.InitialInterval {
		t.Fatalf("jeda percobaan 1 ke 2 = %s, seharusnya minimal %s: SMTP yang sedang sekarat dihantam tanpa jeda", gap, testRetry.InitialInterval)
	}
	if gap := calls[2].Sub(calls[1]); gap < 2*testRetry.InitialInterval {
		t.Fatalf("jeda percobaan 2 ke 3 = %s, seharusnya minimal %s: jeda harus makin panjang (backoff)", gap, 2*testRetry.InitialInterval)
	}
}

// SMTP mati total untuk satu alamat. Setelah jatah retry habis, message itu
// harus minggir supaya message di belakangnya diproses.
func TestStep2_ExhaustedRetriesDoNotBlockTheQueue(t *testing.T) {
	h := start(t, &fakeEmailer{downFor: "mati@example.com"})
	stuck := h.publishEmail(t, "B-1", "mati@example.com")
	h.publishEmail(t, "B-2", "berikutnya@example.com")

	h.waitForNextEmail(t)

	if got := len(h.emailer.callTimes("mati@example.com")); got != testRetry.MaxAttempts {
		t.Fatalf("SMTP dipanggil %d kali untuk B-1, seharusnya %d (MaxAttempts)", got, testRetry.MaxAttempts)
	}
	h.assertParked(t, stuck, "smtp")
}

// Payload rusak tidak akan pernah berhasil. Mengulanginya hanya membuang waktu.
func TestStep3_MalformedMessageIsParkedWithoutRetry(t *testing.T) {
	h := start(t, &fakeEmailer{})
	broken := h.publishRaw(t, `{"booking_id": "B-1", "to": `)
	h.publishEmail(t, "B-2", "berikutnya@example.com")

	h.waitForNextEmail(t)

	if got := h.attemptsFor(broken.UUID); got != 1 {
		t.Fatalf("message rusak diproses %d kali, seharusnya 1: payload rusak tidak sembuh dengan retry", got)
	}
	h.assertParked(t, broken, "decode")
}

// Message ini memicu bug di buildSubject (panic). Service tidak boleh
// tersumbat, dan bug tidak sembuh dengan retry.
func TestStep4_CodeBugDoesNotBlockTheQueue(t *testing.T) {
	h := start(t, &fakeEmailer{})
	trigger := h.publishRaw(t, `{"booking_id": "B-1", "to": "tamu@example.com", "guest": null}`)
	h.publishEmail(t, "B-2", "berikutnya@example.com")

	h.waitForNextEmail(t)

	if got := h.attemptsFor(trigger.UUID); got != 1 {
		t.Fatalf("message pemicu bug diproses %d kali, seharusnya 1: bug tidak sembuh dengan retry", got)
	}
	h.assertParked(t, trigger, "panic")
}

// Satu message yang gagal total menghasilkan tepat satu baris ERROR.
// Percobaan ulang cukup WARN.
func TestStep5_OneErrorLogPerFailedMessage(t *testing.T) {
	h := start(t, &fakeEmailer{downFor: "mati@example.com"})
	stuck := h.publishEmail(t, "B-1", "mati@example.com")
	ok := h.publishEmail(t, "B-2", "berikutnya@example.com")

	h.waitForNextEmail(t)

	errorRecords := h.logs.withLevel(t, "ERROR", stuck.UUID)
	if len(errorRecords) != 1 {
		t.Fatalf("baris ERROR untuk message yang gagal = %d, seharusnya tepat 1", len(errorRecords))
	}
	if errText, _ := errorRecords[0]["error"].(string); !strings.Contains(errText, "smtp") {
		t.Fatalf(`field "error" = %q, seharusnya memuat penyebab aslinya`, errText)
	}
	if got := len(h.logs.withLevel(t, "WARN", stuck.UUID)); got != testRetry.MaxAttempts-1 {
		t.Fatalf("baris WARN untuk message yang gagal = %d, seharusnya %d (satu per percobaan ulang)", got, testRetry.MaxAttempts-1)
	}
	if got := len(h.logs.withLevel(t, "ERROR", ok.UUID)) + len(h.logs.withLevel(t, "WARN", ok.UUID)); got != 0 {
		t.Fatalf("message yang berhasil menghasilkan %d baris WARN/ERROR, seharusnya 0", got)
	}
}
