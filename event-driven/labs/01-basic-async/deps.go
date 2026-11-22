package lab01

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"sync"
	"time"
)

// Booking adalah pesanan kamar yang baru masuk.
type Booking struct {
	ID         string
	GuestEmail string
	RoomType   string
	Rooms      int
}

// Emailer mengirim confirmation email. Di dunia nyata ini panggilan SMTP:
// bisa lambat, bisa gagal.
type Emailer interface {
	SendConfirmation(ctx context.Context, b Booking) error
}

// ChannelNotifier memberi tahu channel manager bahwa availability berubah.
// Di dunia nyata ini panggilan HTTP ke pihak ketiga.
type ChannelNotifier interface {
	NotifyBooking(ctx context.Context, b Booking) error
}

// Config mengatur background worker.
type Config struct {
	// MaxAttempts adalah jumlah percobaan maksimum per pekerjaan, termasuk
	// percobaan pertama. Nilai 1 berarti tanpa retry.
	MaxAttempts int
	// RetryDelay adalah jeda antar percobaan.
	RetryDelay time.Duration
}

// BookingStore adalah database tiruan di memori.
type BookingStore struct {
	mu       sync.Mutex
	bookings map[string]Booking
}

func NewBookingStore() *BookingStore {
	return &BookingStore{bookings: map[string]Booking{}}
}

func (s *BookingStore) Save(_ context.Context, b Booking) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bookings[b.ID] = b
	return nil
}

func (s *BookingStore) Exists(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.bookings[id]
	return ok
}
