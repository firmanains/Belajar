package lab07

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

const waitTimeout = 2 * time.Second

// fakeEmailer menggantikan SMTP dan merekam email yang "terkirim".
type fakeEmailer struct {
	mu   sync.Mutex
	sent []string // "bookingID -> alamat"
}

func (f *fakeEmailer) SendConfirmation(_ context.Context, bookingID, to string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, bookingID+" -> "+to)
	return nil
}

func (f *fakeEmailer) all() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

// startService menjalankan service sungguhan (router, handler, wiring) di
// dalam proses test. Yang diganti hanya dunia luarnya: broker memakai
// gochannel, channel manager dan SMTP memakai tiruan.
func startService(t *testing.T, cm *fakeChannelManager, emailer Emailer) *gochannel.GoChannel {
	t.Helper()
	pubSub := gochannel.NewGoChannel(gochannel.Config{}, watermill.NopLogger{})

	service, err := New(Config{PubSub: pubSub, ChannelManagerURL: cm.url(), Emailer: emailer})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := service.Run(ctx); err != nil {
			t.Errorf("service.Run: %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		if err := service.Close(); err != nil {
			t.Errorf("service.Close: %v", err)
		}
	})

	// gochannel membuang message yang terbit sebelum ada subscriber, jadi
	// test harus menunggu service siap. Ini menunggu sinyal, bukan sleep.
	select {
	case <-service.Running():
	case <-time.After(waitTimeout):
		t.Fatal("service tidak siap dalam 2 detik")
	}
	return pubSub
}

func publish(t *testing.T, pubSub *gochannel.GoChannel, topic string, event any) {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := pubSub.Publish(topic, message.NewMessage(watermill.NewUUID(), payload)); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

var bookingB1 = BookingCreated{BookingID: "B-1", RoomType: "deluxe", Date: "2026-12-24", Rooms: 2, GuestEmail: "tamu@example.com"}

// Helper eventually harus menunggu sampai kondisi terpenuhi, dan menyerah
// hanya setelah timeout.
func TestStep1_EventuallyPollsInsteadOfSleeping(t *testing.T) {
	t.Run("kondisi terpenuhi setelah 300ms", func(t *testing.T) {
		var done atomic.Bool
		timer := time.AfterFunc(300*time.Millisecond, func() { done.Store(true) })
		defer timer.Stop()

		if !eventually(waitTimeout, done.Load) {
			t.Fatal("eventually mengembalikan false padahal kondisi terpenuhi jauh sebelum timeout: ia berhenti memeriksa terlalu cepat")
		}
	})

	t.Run("kondisi tidak pernah terpenuhi", func(t *testing.T) {
		start := time.Now()
		ok := eventually(100*time.Millisecond, func() bool { return false })
		elapsed := time.Since(start)

		if ok {
			t.Fatal("eventually mengembalikan true untuk kondisi yang tidak pernah terpenuhi")
		}
		if elapsed < 100*time.Millisecond {
			t.Fatalf("eventually menyerah setelah %s, seharusnya menunggu sampai timeout 100ms", elapsed)
		}
		if elapsed > time.Second {
			t.Fatalf("eventually baru menyerah setelah %s, padahal timeout-nya 100ms", elapsed)
		}
	})
}

// Booking baru harus sampai ke channel manager sebagai pengurangan availability.
func TestStep2_BookingReachesChannelManager(t *testing.T) {
	cm := newFakeChannelManager(t, 0)
	pubSub := startService(t, cm, &fakeEmailer{})

	publish(t, pubSub, TopicBookingCreated, bookingB1)

	if !eventually(waitTimeout, func() bool { return len(cm.accepted()) == 1 }) {
		t.Fatalf("channel manager tiruan merekam %d update dalam %s, seharusnya 1", len(cm.accepted()), waitTimeout)
	}
	want := AvailabilityUpdate{BookingID: "B-1", RoomType: "deluxe", Date: "2026-12-24", Delta: -2}
	if got := cm.accepted()[0]; got != want {
		t.Fatalf("channel manager menerima %+v, seharusnya %+v", got, want)
	}
}

// Booking yang sama harus menghasilkan email konfirmasi.
func TestStep3_ConfirmationEmailIsSent(t *testing.T) {
	cm := newFakeChannelManager(t, 0)
	emailer := &fakeEmailer{}
	pubSub := startService(t, cm, emailer)

	publish(t, pubSub, TopicBookingCreated, bookingB1)

	if !eventually(waitTimeout, func() bool { return len(emailer.all()) == 1 }) {
		t.Fatalf("email konfirmasi tidak terkirim dalam %s, padahal unit test handler email lulus", waitTimeout)
	}
	if got := emailer.all()[0]; got != "B-1 -> tamu@example.com" {
		t.Fatalf("email terkirim sebagai %q", got)
	}
}

// Channel manager menjawab 503 dua kali lalu pulih. Update tidak boleh hilang.
func TestStep4_ChannelManagerOutageIsRetried(t *testing.T) {
	cm := newFakeChannelManager(t, 2)
	pubSub := startService(t, cm, &fakeEmailer{})

	publish(t, pubSub, TopicBookingCreated, bookingB1)

	if !eventually(waitTimeout, func() bool { return len(cm.accepted()) == 1 }) {
		t.Fatalf("update tidak pernah diterima channel manager setelah ia pulih (total request: %d): jawaban 503 dianggap berhasil dan update hilang", cm.requests())
	}
	if got := cm.requests(); got != 3 {
		t.Fatalf("channel manager menerima %d request, seharusnya 3 (dua ditolak, satu diterima)", got)
	}
}
