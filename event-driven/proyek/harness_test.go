package proyek_test

// File ini sudah jadi. Jangan diubah.
//
// Harness menjalankan seluruh service di dalam proses test, dengan Postgres dan
// Kafka sungguhan. Setiap test memakai schema, topic, dan consumer group
// sendiri, jadi test bisa diulang tanpa membersihkan apa pun.

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"belajar/eventdriven/proyek/internal/app"
	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/inbox"
	"belajar/eventdriven/proyek/internal/outbox"
	"belajar/eventdriven/proyek/internal/platform"
	"belajar/eventdriven/proyek/internal/store"
)

const (
	secret  = "rahasia-untuk-test"
	waitFor = 10 * time.Second // batas tunggu eventually
)

var (
	postgresURL  = cmp.Or(os.Getenv("ED_POSTGRES_URL"), "postgres://lab:lab@localhost:55432/lab")
	kafkaBrokers = strings.Split(cmp.Or(os.Getenv("ED_KAFKA_BROKERS"), "localhost:59092"), ",")
)

// eventually menunggu sampai cond benar. Test asinkron tidak boleh memakai
// sleep dengan durasi tebakan: terlalu pendek membuat test kadang gagal,
// terlalu panjang membuat test lambat.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitFor)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("belum terjadi setelah %s: %s", waitFor, what)
}

// ---------------------------------------------------------------- harness

type harness struct {
	t      *testing.T
	cfg    app.Config
	app    *app.App
	srv    *httptest.Server
	sender *fakeSender
	faults *platform.Faults
	logs   *logBuffer
	events *topicTap
	dlq    *topicTap
	admin  *kgo.Client

	stopOnce sync.Once
	stopErr  error
}

// newConfig menyiapkan nama unik dan membersihkannya setelah test selesai.
func newConfig(t *testing.T) app.Config {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const hint = "jalankan `make up` di root kursus"

	conn, err := pgx.Connect(ctx, postgresURL)
	if err != nil {
		t.Fatalf("Postgres tidak terjangkau di %s: %v. %s", postgresURL, err, hint)
	}
	_ = conn.Close(ctx)
	admin, err := kgo.NewClient(kgo.SeedBrokers(kafkaBrokers...))
	if err != nil {
		t.Fatalf("Kafka client: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		t.Fatalf("Kafka tidak terjangkau di %v: %v. %s", kafkaBrokers, err, hint)
	}

	id := event.NewID()[:10]
	cfg := app.Config{
		PostgresURL:       postgresURL,
		Schema:            "proyek_" + id,
		KafkaBrokers:      kafkaBrokers,
		EventsTopic:       "proyek." + id + ".booking-events",
		DeadLetterTopic:   "proyek." + id + ".booking-events.dlq",
		GroupPrefix:       "proyek." + id,
		WebhookSecret:     secret,
		ForwarderInterval: 20 * time.Millisecond,
		EmailMaxAttempts:  3,
		EmailBackoff:      5 * time.Millisecond,
		ConsumerBackoff:   20 * time.Millisecond,
		Faults:            &platform.Faults{},
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if conn, err := pgx.Connect(ctx, postgresURL); err == nil {
			_, _ = conn.Exec(ctx, "DROP SCHEMA IF EXISTS "+cfg.Schema+" CASCADE")
			_ = conn.Close(ctx)
		}
		_, _ = kadm.NewClient(admin).DeleteTopics(ctx, cfg.EventsTopic, cfg.DeadLetterTopic)
		admin.Close()
	})
	return cfg
}

func start(t *testing.T) *harness {
	t.Helper()
	return startWith(t, newConfig(t), newFakeSender())
}

// startWith menjalankan service dengan cfg tertentu. Dipanggil dua kali dengan
// cfg yang sama untuk meniru restart.
func startWith(t *testing.T, cfg app.Config, sender *fakeSender) *harness {
	t.Helper()
	h := &harness{t: t, sender: sender, faults: cfg.Faults, logs: &logBuffer{}}
	cfg.EmailSender = sender
	cfg.Logger = slog.New(slog.NewJSONHandler(h.logs, nil))
	h.cfg = cfg

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, err := app.Start(ctx, cfg)
	if err != nil {
		t.Fatalf("service gagal start: %v", err)
	}
	h.app = a
	h.srv = httptest.NewServer(a.Handler())

	h.admin, err = kgo.NewClient(kgo.SeedBrokers(cfg.KafkaBrokers...))
	if err != nil {
		t.Fatalf("Kafka client: %v", err)
	}
	h.events = tap(t, cfg.KafkaBrokers, cfg.EventsTopic)
	h.dlq = tap(t, cfg.KafkaBrokers, cfg.DeadLetterTopic)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.stop(ctx)
		h.admin.Close()
	})
	return h
}

// stop menghentikan HTTP server lalu service. Aman dipanggil lebih dari sekali.
func (h *harness) stop(ctx context.Context) error {
	h.stopOnce.Do(func() {
		h.srv.Close()
		h.stopErr = h.app.Shutdown(ctx)
	})
	return h.stopErr
}

// ---------------------------------------------------------------- HTTP

func rev(bookingID string, number int, status string, rooms int, checkIn, checkOut string) inbox.Revision {
	return inbox.Revision{
		RevisionID:     fmt.Sprintf("%s-rev-%d", bookingID, number),
		BookingID:      bookingID,
		RevisionNumber: number,
		Status:         status,
		RoomType:       "deluxe",
		CheckIn:        checkIn,
		CheckOut:       checkOut,
		Rooms:          rooms,
		GuestEmail:     "tamu@example.com",
	}
}

func (h *harness) do(method, path string, body any, headers map[string]string) (int, []byte) {
	h.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.srv.URL+path, rd)
	if err != nil {
		h.t.Fatalf("request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("baca response: %v", err)
	}
	return resp.StatusCode, out
}

// post mengirim satu revision ke webhook dan mengembalikan status dan result.
func (h *harness) post(r inbox.Revision, extraHeaders ...string) (int, string) {
	h.t.Helper()
	headers := map[string]string{app.HeaderSecret: secret}
	for i := 0; i+1 < len(extraHeaders); i += 2 {
		headers[extraHeaders[i]] = extraHeaders[i+1]
	}
	code, body := h.do(http.MethodPost, "/webhooks/booking-revisions", r, headers)
	var out struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal(body, &out)
	return code, out.Result
}

// mustPost mengirim revision dan menuntut 200.
func (h *harness) mustPost(r inbox.Revision) string {
	h.t.Helper()
	code, result := h.post(r)
	if code != http.StatusOK {
		h.t.Fatalf("POST revision %s: status %d, seharusnya 200", r.RevisionID, code)
	}
	return result
}

func (h *harness) get(path string, out any) int {
	h.t.Helper()
	code, body := h.do(http.MethodGet, path, nil, map[string]string{app.HeaderSecret: secret})
	if code == http.StatusOK && out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			h.t.Fatalf("GET %s: response bukan JSON yang diharapkan: %v", path, err)
		}
	}
	return code
}

// booked mengembalikan jumlah kamar terpakai untuk satu room type pada satu
// tanggal. endpoint: "/admin/availability" (write model) atau "/tape-chart"
// (read model).
func (h *harness) booked(endpoint, roomType, date string) int {
	h.t.Helper()
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		h.t.Fatalf("tanggal %q: %v", date, err)
	}
	var days []store.Day
	path := fmt.Sprintf("%s?from=%s&to=%s", endpoint, date, d.AddDate(0, 0, 1).Format("2006-01-02"))
	if code := h.get(path, &days); code != http.StatusOK {
		h.t.Fatalf("GET %s: status %d", path, code)
	}
	for _, day := range days {
		if day.RoomType == roomType {
			return day.Booked
		}
	}
	h.t.Fatalf("room type %s tidak ada di response %s", roomType, path)
	return 0
}

func (h *harness) outbox() outbox.Stats {
	h.t.Helper()
	var st outbox.Stats
	if code := h.get("/admin/outbox", &st); code != http.StatusOK {
		h.t.Fatalf("GET /admin/outbox: status %d", code)
	}
	return st
}

func (h *harness) booking(id string) (event.Booking, int) {
	h.t.Helper()
	var b event.Booking
	code := h.get("/bookings/"+id, &b)
	return b, code
}

// metric membaca satu series dari /metrics, misalnya
// `channelsync_messages_processed_total{consumer="email",outcome="ok"}`.
func (h *harness) metric(series string) (float64, bool) {
	h.t.Helper()
	code, body := h.do(http.MethodGet, "/metrics", nil, nil)
	if code != http.StatusOK {
		h.t.Fatalf("GET /metrics: status %d", code)
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		if rest, ok := strings.CutPrefix(line, series+" "); ok {
			v, err := strconv.ParseFloat(strings.TrimSpace(rest), 64)
			if err != nil {
				h.t.Fatalf("nilai metric %s tidak terbaca: %q", series, rest)
			}
			return v, true
		}
	}
	return 0, false
}

// ---------------------------------------------------------------- Kafka

// topicTap membaca satu topic dari awal tanpa consumer group dan mengingat
// semua record yang lewat.
type topicTap struct {
	mu   sync.Mutex
	recs []*kgo.Record
}

func tap(t *testing.T, brokers []string, topic string) *topicTap {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.FetchMaxWait(250*time.Millisecond)) // lihat catatan FetchMaxWait di internal/platform/consume.go
	if err != nil {
		t.Fatalf("Kafka client: %v", err)
	}
	tp := &topicTap{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			fetches := cl.PollFetches(ctx)
			tp.mu.Lock()
			tp.recs = append(tp.recs, fetches.Records()...)
			tp.mu.Unlock()
		}
	}()
	t.Cleanup(func() { cancel(); <-done; cl.Close() })
	return tp
}

func (tp *topicTap) records() []*kgo.Record {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return append([]*kgo.Record(nil), tp.recs...)
}

// forBooking mengembalikan record yang key-nya booking ID tertentu.
func (tp *topicTap) forBooking(bookingID string) []*kgo.Record {
	var out []*kgo.Record
	for _, r := range tp.records() {
		if string(r.Key) == bookingID {
			out = append(out, r)
		}
	}
	return out
}

func header(r *kgo.Record, key string) string {
	for _, h := range r.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// produceRaw menulis langsung ke topic event, melewati webhook dan outbox.
// Dipakai untuk meniru hal yang tidak bisa dipicu lewat HTTP: duplicate,
// urutan terbalik, dan message rusak.
func (h *harness) produceRaw(key string, value []byte, headers map[string]string) {
	h.t.Helper()
	r := &kgo.Record{Topic: h.cfg.EventsTopic, Key: []byte(key), Value: value}
	for k, v := range headers {
		r.Headers = append(r.Headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitFor)
	defer cancel()
	if err := h.admin.ProduceSync(ctx, r).FirstErr(); err != nil {
		h.t.Fatalf("produce ke %s: %v", h.cfg.EventsTopic, err)
	}
}

func (h *harness) produceEvent(key string, env event.Envelope) {
	h.t.Helper()
	value, err := json.Marshal(env)
	if err != nil {
		h.t.Fatalf("marshal envelope: %v", err)
	}
	h.produceRaw(key, value, env.Headers())
}

// committed mengembalikan jumlah offset yang sudah di-commit sebuah consumer
// group di topic event, dijumlahkan dari semua partition. Topic baru mulai
// dari offset 0, jadi angka ini sama dengan jumlah record yang sudah di-ack.
func (h *harness) committed(consumer string) int64 {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitFor)
	defer cancel()
	offsets, err := kadm.NewClient(h.admin).FetchOffsets(ctx, h.cfg.GroupPrefix+"."+consumer)
	if err != nil {
		h.t.Fatalf("fetch offsets: %v", err)
	}
	var total int64
	offsets.Each(func(o kadm.OffsetResponse) {
		if o.Topic == h.cfg.EventsTopic && o.At > 0 {
			total += o.At
		}
	})
	return total
}

// ---------------------------------------------------------------- fakes

// fakeSender menggantikan SMTP. Perilakunya diatur per booking ID.
type fakeSender struct {
	mu       sync.Mutex
	failures map[string]int           // sisa kegagalan; -1 berarti selalu gagal
	delay    map[string]time.Duration // lama kirim
	block    map[string]bool          // menggantung sampai context dibatalkan
	attempts map[string]int
	sent     map[string]int
}

func newFakeSender() *fakeSender {
	return &fakeSender{
		failures: map[string]int{}, delay: map[string]time.Duration{}, block: map[string]bool{},
		attempts: map[string]int{}, sent: map[string]int{},
	}
}

func (f *fakeSender) SendConfirmation(ctx context.Context, b event.Booking) error {
	id := b.BookingID
	f.mu.Lock()
	f.attempts[id]++
	delay, block := f.delay[id], f.block[id]
	fail := f.failures[id] != 0
	if f.failures[id] > 0 {
		f.failures[id]--
	}
	f.mu.Unlock()

	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if fail {
		return errors.New("SMTP timeout (tiruan)")
	}
	f.mu.Lock()
	f.sent[id]++
	f.mu.Unlock()
	return nil
}

func (f *fakeSender) set(fn func()) { f.mu.Lock(); defer f.mu.Unlock(); fn() }

func (f *fakeSender) attemptsFor(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attempts[id]
}

func (f *fakeSender) sentFor(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[id]
}

// logBuffer menampung log JSON service supaya test bisa memeriksanya.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// has melaporkan apakah ada baris log yang memuat semua pasangan key/value.
func (l *logBuffer) has(kv map[string]string) bool {
	l.mu.Lock()
	data := l.buf.String()
	l.mu.Unlock()
lines:
	for line := range strings.SplitSeq(data, "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		for k, v := range kv {
			if s, _ := entry[k].(string); s != v {
				continue lines
			}
		}
		return true
	}
	return false
}
