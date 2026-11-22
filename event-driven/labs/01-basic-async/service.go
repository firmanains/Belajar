package lab01

// File ini yang kamu ubah.

import (
	"context"
	"fmt"
)

// BookingService menerima booking baru.
type BookingService struct {
	store   *BookingStore
	email   Emailer
	channel ChannelNotifier
	cfg     Config
}

func NewBookingService(store *BookingStore, email Emailer, channel ChannelNotifier, cfg Config) *BookingService {
	return &BookingService{store: store, email: email, channel: channel, cfg: cfg}
}

// CreateBooking menyimpan booking, mengirim confirmation email, lalu memberi
// tahu channel manager. Versi ini mengerjakan ketiganya secara synchronous:
// pemanggil menunggu sampai semuanya selesai, dan ikut gagal kalau salah satu
// gagal.
func (s *BookingService) CreateBooking(ctx context.Context, b Booking) error {
	if err := s.store.Save(ctx, b); err != nil {
		return fmt.Errorf("save booking %s: %w", b.ID, err)
	}
	if err := s.email.SendConfirmation(ctx, b); err != nil {
		return fmt.Errorf("send confirmation for %s: %w", b.ID, err)
	}
	if err := s.channel.NotifyBooking(ctx, b); err != nil {
		return fmt.Errorf("notify channel manager for %s: %w", b.ID, err)
	}
	return nil
}

// Shutdown menghentikan service. Ia baru boleh kembali setelah semua pekerjaan
// yang sudah diterima selesai dikerjakan, atau setelah ctx habis.
func (s *BookingService) Shutdown(ctx context.Context) error {
	return nil
}
