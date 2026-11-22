package lab01

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// ErrShuttingDown dikembalikan kalau booking datang setelah Shutdown dipanggil.
var ErrShuttingDown = errors.New("booking service is shutting down")

// job adalah satu pekerjaan background: nama untuk log, dan fungsi yang
// dijalankan worker.
type job struct {
	name string
	run  func(ctx context.Context) error
}

// BookingService menerima booking baru.
type BookingService struct {
	store   *BookingStore
	email   Emailer
	channel ChannelNotifier
	cfg     Config

	jobs chan job
	wg   sync.WaitGroup

	mu     sync.RWMutex
	closed bool
}

func NewBookingService(store *BookingStore, email Emailer, channel ChannelNotifier, cfg Config) *BookingService {
	s := &BookingService{
		store:   store,
		email:   email,
		channel: channel,
		cfg:     cfg,
		jobs:    make(chan job, 100),
	}
	s.wg.Add(1)
	go s.worker()
	return s
}

// CreateBooking hanya mengerjakan bagian yang harus selesai sebelum tamu
// mendapat jawaban: menyimpan booking. Email dan notifikasi channel manager
// dititipkan ke queue sebagai dua pekerjaan terpisah, supaya kegagalan yang
// satu tidak menahan yang lain.
func (s *BookingService) CreateBooking(ctx context.Context, b Booking) error {
	if err := s.store.Save(ctx, b); err != nil {
		return fmt.Errorf("save booking %s: %w", b.ID, err)
	}
	if err := s.enqueue(ctx, job{
		name: "confirmation email " + b.ID,
		run:  func(ctx context.Context) error { return s.email.SendConfirmation(ctx, b) },
	}); err != nil {
		return fmt.Errorf("enqueue confirmation email for %s: %w", b.ID, err)
	}
	if err := s.enqueue(ctx, job{
		name: "channel notification " + b.ID,
		run:  func(ctx context.Context) error { return s.channel.NotifyBooking(ctx, b) },
	}); err != nil {
		return fmt.Errorf("enqueue channel notification for %s: %w", b.ID, err)
	}
	return nil
}

func (s *BookingService) enqueue(ctx context.Context, j job) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrShuttingDown
	}
	select {
	case s.jobs <- j:
		return nil
	case <-ctx.Done():
		// Queue penuh dan pemanggil sudah tidak mau menunggu.
		return ctx.Err()
	}
}

func (s *BookingService) worker() {
	defer s.wg.Done()
	for j := range s.jobs {
		if err := s.runWithRetry(j); err != nil {
			// Tidak ada lagi yang bisa diberi tahu: tamu sudah menerima
			// jawaban. Pekerjaan ini hilang, dan hanya log yang mencatatnya.
			log.Printf("job dropped: %v", err)
		}
	}
}

func (s *BookingService) runWithRetry(j job) error {
	// Context dari request tidak dipakai di sini. Context itu dibatalkan
	// begitu response terkirim, padahal pekerjaan ini baru mulai sesudahnya.
	ctx := context.Background()

	maxAttempts := max(s.cfg.MaxAttempts, 1)
	for attempt := 1; ; attempt++ {
		err := j.run(ctx)
		if err == nil {
			return nil
		}
		if attempt >= maxAttempts {
			return fmt.Errorf("%s: gave up after %d attempts: %w", j.name, attempt, err)
		}
		time.Sleep(s.cfg.RetryDelay)
	}
}

// Shutdown menutup queue, lalu menunggu worker menghabiskan isinya.
func (s *BookingService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.jobs)
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown: %w", ctx.Err())
	}
}
