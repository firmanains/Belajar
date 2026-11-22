package lab01

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeEmailer meniru SMTP: bisa ditahan (gate) dan bisa gagal beberapa kali.
type fakeEmailer struct {
	gate      chan struct{} // kalau tidak nil, Send menunggu sampai channel ini ditutup
	failFirst int           // jumlah percobaan pertama yang gagal; -1 berarti selalu gagal

	mu       sync.Mutex
	attempts int
	sent     map[string]int
}

func (f *fakeEmailer) SendConfirmation(_ context.Context, b Booking) error {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if f.failFirst < 0 || f.attempts <= f.failFirst {
		return errors.New("SMTP timeout")
	}
	if f.sent == nil {
		f.sent = map[string]int{}
	}
	f.sent[b.ID]++
	return nil
}

func (f *fakeEmailer) sentCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[id]
}

func (f *fakeEmailer) totalSent() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.sent {
		n += c
	}
	return n
}

func (f *fakeEmailer) attemptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attempts
}

type fakeChannel struct {
	mu       sync.Mutex
	notified map[string]int
}

func (f *fakeChannel) NotifyBooking(_ context.Context, b Booking) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.notified == nil {
		f.notified = map[string]int{}
	}
	f.notified[b.ID]++
	return nil
}

func (f *fakeChannel) count(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.notified[id]
}

func newBooking(id string) Booking {
	return Booking{ID: id, GuestEmail: "tamu@example.com", RoomType: "deluxe", Rooms: 2}
}

// eventually menunggu sampai cond benar, paling lama 2 detik.
func eventually(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// createWithin memanggil CreateBooking dan melaporkan apakah ia kembali dalam
// 1 detik.
func createWithin(svc *BookingService, b Booking) (returned bool, err error) {
	done := make(chan error, 1)
	go func() { done <- svc.CreateBooking(context.Background(), b) }()
	select {
	case err := <-done:
		return true, err
	case <-time.After(time.Second):
		return false, nil
	}
}

func shutdown(t *testing.T, svc *BookingService) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown gagal: %v", err)
	}
}

// Dunia ideal: SMTP dan channel manager cepat dan sehat.
func TestStep0_HappyPath(t *testing.T) {
	store, email, channel := NewBookingStore(), &fakeEmailer{}, &fakeChannel{}
	svc := NewBookingService(store, email, channel, Config{MaxAttempts: 1})
	defer shutdown(t, svc)

	if err := svc.CreateBooking(context.Background(), newBooking("B-1")); err != nil {
		t.Fatalf("CreateBooking gagal: %v", err)
	}
	if !store.Exists("B-1") {
		t.Fatal("booking B-1 tidak tersimpan")
	}
	if !eventually(func() bool { return email.sentCount("B-1") == 1 }) {
		t.Fatalf("confirmation email terkirim %d kali, seharusnya 1", email.sentCount("B-1"))
	}
	if !eventually(func() bool { return channel.count("B-1") == 1 }) {
		t.Fatalf("channel manager diberi tahu %d kali, seharusnya 1", channel.count("B-1"))
	}
}

// SMTP sedang lambat. Tamu tidak boleh ikut menunggu.
func TestStep1_SlowEmailDoesNotBlockTheGuest(t *testing.T) {
	gate := make(chan struct{})
	store, email, channel := NewBookingStore(), &fakeEmailer{gate: gate}, &fakeChannel{}
	svc := NewBookingService(store, email, channel, Config{MaxAttempts: 1})
	defer shutdown(t, svc)
	defer close(gate) // lepaskan SMTP supaya tidak ada goroutine yang tertinggal

	returned, err := createWithin(svc, newBooking("B-1"))
	if !returned {
		t.Fatal("CreateBooking belum kembali setelah 1 detik: tamu ikut menunggu SMTP yang lambat")
	}
	if err != nil {
		t.Fatalf("CreateBooking gagal: %v", err)
	}
	if !store.Exists("B-1") {
		t.Fatal("booking B-1 harus sudah tersimpan saat CreateBooking kembali")
	}
}

// SMTP mati total. Booking tetap sah, dan channel manager tetap harus tahu.
func TestStep2_EmailFailureDoesNotFailTheBooking(t *testing.T) {
	store, email, channel := NewBookingStore(), &fakeEmailer{failFirst: -1}, &fakeChannel{}
	svc := NewBookingService(store, email, channel, Config{MaxAttempts: 3, RetryDelay: time.Millisecond})
	defer shutdown(t, svc)

	if err := svc.CreateBooking(context.Background(), newBooking("B-1")); err != nil {
		t.Fatalf("CreateBooking gagal karena SMTP mati, padahal kamarnya ada: %v", err)
	}
	if !eventually(func() bool { return channel.count("B-1") == 1 }) {
		t.Fatalf("channel manager diberi tahu %d kali, seharusnya 1: kegagalan email tidak boleh menahan pekerjaan lain", channel.count("B-1"))
	}
}

// SMTP gagal dua kali lalu pulih. Worker harus mencoba lagi sampai berhasil.
func TestStep3_WorkerRetriesUntilSuccess(t *testing.T) {
	store, email, channel := NewBookingStore(), &fakeEmailer{failFirst: 2}, &fakeChannel{}
	svc := NewBookingService(store, email, channel, Config{MaxAttempts: 5, RetryDelay: time.Millisecond})
	defer shutdown(t, svc)

	if err := svc.CreateBooking(context.Background(), newBooking("B-1")); err != nil {
		t.Fatalf("CreateBooking gagal: %v", err)
	}
	if !eventually(func() bool { return email.sentCount("B-1") == 1 }) {
		t.Fatalf("confirmation email terkirim %d kali setelah %d percobaan, seharusnya terkirim 1 kali pada percobaan ke-3", email.sentCount("B-1"), email.attemptCount())
	}
	if got := email.attemptCount(); got != 3 {
		t.Fatalf("jumlah percobaan = %d, seharusnya 3 (gagal, gagal, berhasil)", got)
	}
}

// Deploy versi baru: service dimatikan saat masih ada pekerjaan di queue.
// Shutdown harus menunggu semuanya selesai.
func TestStep4_ShutdownDrainsTheQueue(t *testing.T) {
	gate := make(chan struct{})
	store, email, channel := NewBookingStore(), &fakeEmailer{gate: gate}, &fakeChannel{}
	svc := NewBookingService(store, email, channel, Config{MaxAttempts: 1})

	const n = 5
	for i := range n {
		returned, err := createWithin(svc, newBooking(fmt.Sprintf("B-%d", i)))
		if !returned {
			close(gate)
			t.Fatalf("CreateBooking ke-%d belum kembali setelah 1 detik", i+1)
		}
		if err != nil {
			close(gate)
			t.Fatalf("CreateBooking ke-%d gagal: %v", i+1, err)
		}
	}

	close(gate) // SMTP pulih, lalu service langsung dimatikan
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown gagal: %v", err)
	}
	if got := email.totalSent(); got != n {
		t.Fatalf("setelah Shutdown kembali, email terkirim %d dari %d: sisanya hilang bersama proses", got, n)
	}
}
