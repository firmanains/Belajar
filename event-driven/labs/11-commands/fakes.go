package lab11

// File ini sudah jadi. Jangan diubah; baca saja.

import (
	"context"
	"errors"
	"sync"
)

// PaymentGateway adalah pihak luar yang menarik uang dari kartu.
type PaymentGateway interface {
	Capture(ctx context.Context, bookingID string, amountIDR int64) error
}

var (
	// ErrCardDeclined adalah penolakan bisnis: kartu ditolak bank.
	ErrCardDeclined = errors.New("kartu ditolak bank")
	// ErrGatewayTimeout adalah kegagalan teknis: gateway tidak menjawab.
	ErrGatewayTimeout = errors.New("payment gateway timeout")
)

// Capture mencatat satu penarikan yang berhasil.
type Capture struct {
	BookingID string
	AmountIDR int64
}

// FakeGateway menggantikan payment gateway sungguhan.
type FakeGateway struct {
	mu       sync.Mutex
	captures []Capture
	calls    map[string]int
	declined map[string]bool
	timeouts int
}

func NewFakeGateway() *FakeGateway {
	return &FakeGateway{calls: map[string]int{}, declined: map[string]bool{}}
}

// Decline membuat setiap Capture untuk booking ini ditolak dengan ErrCardDeclined.
func (g *FakeGateway) Decline(bookingID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.declined[bookingID] = true
}

// TimeoutNext membuat n panggilan Capture berikutnya gagal dengan ErrGatewayTimeout.
func (g *FakeGateway) TimeoutNext(n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.timeouts = n
}

func (g *FakeGateway) Capture(_ context.Context, bookingID string, amountIDR int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls[bookingID]++
	if g.timeouts > 0 {
		g.timeouts--
		return ErrGatewayTimeout
	}
	if g.declined[bookingID] {
		return ErrCardDeclined
	}
	g.captures = append(g.captures, Capture{BookingID: bookingID, AmountIDR: amountIDR})
	return nil
}

// Captures mengembalikan salinan semua penarikan yang berhasil.
func (g *FakeGateway) Captures() []Capture {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]Capture(nil), g.captures...)
}

// Calls mengembalikan berapa kali Capture dipanggil untuk satu booking.
func (g *FakeGateway) Calls(bookingID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls[bookingID]
}

// Bookings menyimpan status pembayaran tiap booking.
type Bookings struct {
	mu     sync.Mutex
	status map[string]string
}

func NewBookings() *Bookings { return &Bookings{status: map[string]string{}} }

func (b *Bookings) SetStatus(bookingID, status string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status[bookingID] = status
}

func (b *Bookings) Status(bookingID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status[bookingID]
}

// Receipts mencatat booking yang kuitansinya sudah dijadwalkan untuk dikirim.
type Receipts struct {
	mu     sync.Mutex
	queued map[string]bool
}

func NewReceipts() *Receipts { return &Receipts{queued: map[string]bool{}} }

func (r *Receipts) Queue(bookingID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queued[bookingID] = true
}

func (r *Receipts) Queued(bookingID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.queued[bookingID]
}
