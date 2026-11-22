package lab04

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

const waitTimeout = 3 * time.Second

// fakeEmailer meniru SMTP: bisa ditahan (gate) dan bisa gagal beberapa kali.
type fakeEmailer struct {
	gate      chan struct{} // kalau tidak nil, Send menunggu sampai channel ini ditutup
	entered   chan struct{} // kalau tidak nil, ditutup saat Send pertama kali dipanggil
	failFirst int

	mu       sync.Mutex
	once     sync.Once
	attempts int
	sent     map[string]int
}

func (f *fakeEmailer) SendConfirmation(_ context.Context, bookingID string) error {
	if f.entered != nil {
		f.once.Do(func() { close(f.entered) })
	}
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.attempts <= f.failFirst {
		return errors.New("SMTP timeout")
	}
	if f.sent == nil {
		f.sent = map[string]int{}
	}
	f.sent[bookingID]++
	return nil
}

func (f *fakeEmailer) sentCount(bookingID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[bookingID]
}

func (f *fakeEmailer) attemptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attempts
}

// countingSubscriber meneruskan pesan apa adanya dan menghitung berapa kali
// setiap payload dikirim ke handler.
type countingSubscriber struct {
	message.Subscriber

	mu    sync.Mutex
	count map[string]int
}

func (c *countingSubscriber) Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error) {
	in, err := c.Subscriber.Subscribe(ctx, topic)
	if err != nil {
		return nil, err
	}
	out := make(chan *message.Message)
	go func() {
		defer close(out)
		for msg := range in {
			c.mu.Lock()
			if c.count == nil {
				c.count = map[string]int{}
			}
			c.count[string(msg.Payload)]++
			c.mu.Unlock()
			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (c *countingSubscriber) deliveries(payload string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count[payload]
}

// newPubSub membuat Pub/Sub di memori. Persistent berarti pesan yang
// diterbitkan sebelum ada subscriber tetap dikirim saat subscriber datang.
func newPubSub(t *testing.T) *gochannel.GoChannel {
	t.Helper()
	pubSub := gochannel.NewGoChannel(gochannel.Config{Persistent: true}, watermill.NopLogger{})
	t.Cleanup(func() { _ = pubSub.Close() })
	return pubSub
}

func publish(t *testing.T, pub message.Publisher, payload string) {
	t.Helper()
	msg := message.NewMessage(watermill.NewUUID(), []byte(payload))
	if err := pub.Publish(TopicBookingCreated, msg); err != nil {
		t.Fatalf("publish gagal: %v", err)
	}
}

// runApp menjalankan App di goroutine. Fungsi stop yang dikembalikan
// membatalkan ctx; channel done menerima hasil Run.
func runApp(t *testing.T, sub message.Subscriber, email Emailer) (app *App, stop context.CancelFunc, done <-chan error) {
	t.Helper()
	app, err := NewApp(sub, email, watermill.NopLogger{})
	if err != nil {
		t.Fatalf("NewApp gagal: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- app.Run(ctx) }()
	t.Cleanup(cancel)
	return app, cancel, result
}

func eventually(cond func() bool) bool {
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func healthStatus(app *App) int {
	rec := httptest.NewRecorder()
	app.HealthHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	return rec.Code
}

// Consume loop tulisan tangan bekerja. Titik awalnya bukan kode yang rusak,
// tapi kode yang harus ditulis ulang di setiap service.
func TestStep0_ManualLoopWorks(t *testing.T) {
	pubSub := newPubSub(t)
	email := &fakeEmailer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ConsumeManually(ctx, pubSub, email) }()

	publish(t, pubSub, `{"booking_id":"B-1"}`)
	if !eventually(func() bool { return email.sentCount("B-1") == 1 }) {
		t.Fatalf("email terkirim %d kali, seharusnya 1", email.sentCount("B-1"))
	}
}

// Handler yang didaftarkan ke Router menerima pesan tanpa consume loop.
func TestStep1_RouterCallsTheHandler(t *testing.T) {
	pubSub := newPubSub(t)
	email := &fakeEmailer{}
	runApp(t, pubSub, email)

	publish(t, pubSub, `{"booking_id":"B-1"}`)
	if !eventually(func() bool { return email.sentCount("B-1") == 1 }) {
		t.Fatalf("email untuk B-1 terkirim %d kali, seharusnya 1: belum ada handler yang menerima pesan dari %s", email.sentCount("B-1"), TopicBookingCreated)
	}
}

// SMTP gagal dua kali. Handler yang mengembalikan error membuat Router
// mengirim nack, dan pesan datang lagi.
func TestStep2_ErrorMeansNackAndRedelivery(t *testing.T) {
	pubSub := newPubSub(t)
	email := &fakeEmailer{failFirst: 2}
	runApp(t, pubSub, email)

	publish(t, pubSub, `{"booking_id":"B-1"}`)
	if !eventually(func() bool { return email.sentCount("B-1") == 1 }) {
		t.Fatalf("email untuk B-1 terkirim %d kali setelah %d percobaan, seharusnya terkirim pada percobaan ke-3: error dari SMTP harus sampai ke Router supaya pesan dikirim ulang", email.sentCount("B-1"), email.attemptCount())
	}
	if got := email.attemptCount(); got != 3 {
		t.Fatalf("jumlah percobaan = %d, seharusnya 3", got)
	}
}

// Pesan rusak tidak akan pernah bisa diproses. Mengirimnya ulang hanya
// membuang tenaga, jadi ia harus di-ack dan dicatat, bukan di-nack.
func TestStep3_MalformedMessageIsNotRedeliveredForever(t *testing.T) {
	const malformed = `ini bukan JSON`
	sub := &countingSubscriber{Subscriber: newPubSub(t)}
	email := &fakeEmailer{}
	runApp(t, sub, email)

	publish(t, sub.Subscriber.(*gochannel.GoChannel), malformed)
	publish(t, sub.Subscriber.(*gochannel.GoChannel), `{"booking_id":"B-1"}`)
	if !eventually(func() bool { return email.sentCount("B-1") == 1 }) {
		t.Fatalf("email untuk B-1 terkirim %d kali, seharusnya 1", email.sentCount("B-1"))
	}
	time.Sleep(100 * time.Millisecond) // beri waktu kalau pesan rusak masih berputar
	if got := sub.deliveries(malformed); got != 1 {
		t.Fatalf("pesan rusak dikirim ke handler %d kali, seharusnya 1: nack membuatnya dikirim ulang tanpa akhir", got)
	}
}

// Deploy versi baru saat satu email sedang dikirim. Run harus menunggu
// handler itu selesai.
func TestStep4_ShutdownWaitsForInFlightMessage(t *testing.T) {
	pubSub := newPubSub(t)
	email := &fakeEmailer{gate: make(chan struct{}), entered: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(email.gate) }) }
	defer release()
	_, stop, done := runApp(t, pubSub, email)

	publish(t, pubSub, `{"booking_id":"B-1"}`)
	select {
	case <-email.entered:
	case <-time.After(waitTimeout):
		t.Fatal("handler tidak pernah mulai memproses B-1")
	}

	stop() // sinyal berhenti datang saat SMTP masih bekerja
	select {
	case <-done:
		t.Fatal("Run kembali saat handler masih mengirim email: proses akan mati di tengah pekerjaan")
	case <-time.After(200 * time.Millisecond):
	}

	release() // SMTP selesai
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run mengembalikan error: %v", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Run tidak kembali setelah handler selesai")
	}
	if got := email.sentCount("B-1"); got != 1 {
		t.Fatalf("email untuk B-1 terkirim %d kali, seharusnya 1", got)
	}
}

// Health check mengikuti keadaan Router, bukan sekadar "proses masih hidup".
func TestStep5_HealthCheckFollowsTheRouter(t *testing.T) {
	pubSub := newPubSub(t)
	app, stop, done := runApp(t, pubSub, &fakeEmailer{})

	if !eventually(func() bool { return healthStatus(app) == http.StatusOK }) {
		t.Fatalf("health check = %d, seharusnya 200 setelah Router berjalan", healthStatus(app))
	}

	stop()
	select {
	case <-done:
	case <-time.After(waitTimeout):
		t.Fatal("Run tidak kembali setelah ctx dibatalkan")
	}
	if got := healthStatus(app); got != http.StatusServiceUnavailable {
		t.Fatalf("health check = %d setelah service berhenti, seharusnya 503", got)
	}

	idle, err := NewApp(pubSub, &fakeEmailer{}, watermill.NopLogger{})
	if err != nil {
		t.Fatalf("NewApp gagal: %v", err)
	}
	if got := healthStatus(idle); got != http.StatusServiceUnavailable {
		t.Fatalf("health check = %d sebelum Run dipanggil, seharusnya 503", got)
	}
}
