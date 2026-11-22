package lab08

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

const waitTimeout = 2 * time.Second

type fakeAvailability struct {
	mu    sync.Mutex
	calls int
	rooms int // perubahan bersih: negatif berarti kamar berkurang
}

func (f *fakeAvailability) Decrease(_ context.Context, _ string, rooms int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.rooms -= rooms
	return nil
}

func (f *fakeAvailability) Increase(_ context.Context, _ string, rooms int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.rooms += rooms
	return nil
}

func (f *fakeAvailability) net() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rooms
}

type fakeEmailer struct {
	mu        sync.Mutex
	failFirst int    // N call pertama gagal
	downFor   string // booking yang emailnya selalu gagal
	calls     int
	sent      []string
}

func (f *fakeEmailer) SendConfirmation(_ context.Context, bookingID, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if bookingID == f.downFor || f.calls <= f.failFirst {
		return errors.New("smtp: connection timeout")
	}
	f.sent = append(f.sent, bookingID)
	return nil
}

func (f *fakeEmailer) sentCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

type fakeChannelManager struct {
	mu     sync.Mutex
	pushes int
}

func (f *fakeChannelManager) PushAvailability(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushes++
	return nil
}

func (f *fakeChannelManager) pushCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pushes
}

type world struct {
	broker       *Broker
	availability *fakeAvailability
	emailer      *fakeEmailer
	channel      *fakeChannelManager
}

func newWorld(t *testing.T, emailer *fakeEmailer) *world {
	t.Helper()
	w := &world{
		broker:       NewBroker(),
		availability: &fakeAvailability{},
		emailer:      emailer,
		channel:      &fakeChannelManager{},
	}
	t.Cleanup(func() { _ = w.broker.Close() })
	return w
}

// startApp menjalankan satu instance service di atas broker milik world.
// Dipanggil dua kali, hasilnya dua instance yang berbagi broker dan dunia luar.
func (w *world) startApp(t *testing.T) *App {
	t.Helper()
	app, err := NewApp(w.broker, Deps{Availability: w.availability, Emailer: w.emailer, ChannelManager: w.channel})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := app.Run(ctx); err != nil {
			t.Errorf("app.Run: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		if err := app.Close(); err != nil {
			t.Errorf("app.Close: %v", err)
		}
	})

	select {
	case <-app.Running():
	case <-time.After(waitTimeout):
		t.Fatal("app tidak siap dalam 2 detik")
	}
	return app
}

func publishBooking(t *testing.T, app *App, bookingID string, rooms int) {
	t.Helper()
	event := BookingCreated{BookingID: bookingID, RoomType: "deluxe", Rooms: rooms, GuestEmail: "tamu@example.com"}
	if err := app.EventBus.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish BookingCreated: %v", err)
	}
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

// staysTrue memastikan cond tetap true selama jendela pendek. Membuktikan
// bahwa sesuatu tidak terjadi memang butuh menunggu.
func staysTrue(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !cond() {
			t.Fatal(what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Dunia ideal: satu booking, tiga akibat. Test ini lulus dari awal.
func TestStep0_OneEventThreeEffects(t *testing.T) {
	w := newWorld(t, &fakeEmailer{})
	app := w.startApp(t)

	publishBooking(t, app, "B-1", 2)

	waitFor(t, "availability berkurang 2", func() bool { return w.availability.net() == -2 })
	waitFor(t, "email konfirmasi terkirim", func() bool { return w.emailer.sentCount() == 1 })
	waitFor(t, "channel manager diberi tahu", func() bool { return w.channel.pushCount() == 1 })
}

// SMTP gagal dua kali lalu pulih. Hanya pengiriman email yang boleh diulang.
func TestStep1_EmailFailureDoesNotRepeatOtherWork(t *testing.T) {
	w := newWorld(t, &fakeEmailer{failFirst: 2})
	app := w.startApp(t)

	publishBooking(t, app, "B-1", 2)

	waitFor(t, "email konfirmasi terkirim setelah SMTP pulih", func() bool { return w.emailer.sentCount() == 1 })
	waitFor(t, "channel manager diberi tahu", func() bool { return w.channel.pushCount() >= 1 })
	waitFor(t, "availability berkurang", func() bool { return w.availability.net() <= -2 })

	if got := w.availability.net(); got != -2 {
		t.Fatalf("availability berubah %d, seharusnya -2: kegagalan email ikut mengulang pengurangan kamar", got)
	}
	if got := w.channel.pushCount(); got != 1 {
		t.Fatalf("channel manager diberi tahu %d kali, seharusnya 1", got)
	}
}

// SMTP mati total untuk B-1. Availability dan channel manager tetap harus
// memproses B-1 dan B-2.
func TestStep2_EmailOutageDoesNotBlockOtherHandlers(t *testing.T) {
	w := newWorld(t, &fakeEmailer{downFor: "B-1"})
	app := w.startApp(t)

	publishBooking(t, app, "B-1", 2)
	publishBooking(t, app, "B-2", 2)

	waitFor(t, "channel manager diberi tahu untuk B-1 dan B-2; handler lain ikut tersumbat oleh email B-1", func() bool { return w.channel.pushCount() == 2 })
	waitFor(t, "availability berkurang untuk B-1 dan B-2", func() bool { return w.availability.net() <= -4 })

	if got := w.availability.net(); got != -4 {
		t.Fatalf("availability berubah %d, seharusnya -4", got)
	}
}

// Dua instance service berjalan bersamaan. Setiap event tetap diproses sekali
// per handler, bukan sekali per instance.
func TestStep3_TwoInstancesShareTheWork(t *testing.T) {
	w := newWorld(t, &fakeEmailer{})
	app := w.startApp(t)
	w.startApp(t)

	const bookings = 10
	for i := 1; i <= bookings; i++ {
		publishBooking(t, app, fmt.Sprintf("B-%d", i), 1)
	}

	waitFor(t, "10 booking mengurangi availability", func() bool { return w.availability.net() <= -bookings })
	waitFor(t, "10 email terkirim", func() bool { return w.emailer.sentCount() >= bookings })
	waitFor(t, "10 kabar ke channel manager", func() bool { return w.channel.pushCount() >= bookings })

	staysTrue(t, fmt.Sprintf("dua instance memproses event yang sama: availability %d (seharusnya %d), email %d, kabar ke channel manager %d",
		w.availability.net(), -bookings, w.emailer.sentCount(), w.channel.pushCount()),
		func() bool {
			return w.availability.net() == -bookings && w.emailer.sentCount() == bookings && w.channel.pushCount() == bookings
		})
}

// Event jenis baru: booking dibatalkan, kamarnya kembali.
func TestStep4_CancelledBookingRestoresAvailability(t *testing.T) {
	w := newWorld(t, &fakeEmailer{})
	app := w.startApp(t)

	publishBooking(t, app, "B-1", 2)
	waitFor(t, "availability berkurang 2", func() bool { return w.availability.net() == -2 })

	if err := app.EventBus.Publish(context.Background(), BookingCancelled{BookingID: "B-1", RoomType: "deluxe", Rooms: 2}); err != nil {
		t.Fatalf("publish BookingCancelled: %v", err)
	}

	waitFor(t, "availability kembali ke 0 setelah BookingCancelled; belum ada handler untuk event ini", func() bool { return w.availability.net() == 0 })
}
