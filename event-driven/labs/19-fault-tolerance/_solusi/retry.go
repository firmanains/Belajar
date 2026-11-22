package lab19

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RetryConfig mengatur perilaku Retry. Sleep dan Jitter disuntikkan supaya
// test tidak perlu menunggu sungguhan dan tidak bergantung pada angka acak.
type RetryConfig struct {
	MaxAttempts int           // jumlah percobaan total, termasuk yang pertama
	BaseDelay   time.Duration // backoff sebelum percobaan kedua
	MaxDelay    time.Duration // batas atas backoff
	Sleep       func(ctx context.Context, d time.Duration) error
	Jitter      func() float64 // angka di rentang [0, 1)
}

// Retry menjalankan op sampai berhasil, sampai error-nya permanent, atau
// sampai MaxAttempts habis. Jeda antar percobaan memakai exponential backoff
// dengan full jitter: angka acak di antara nol dan backoff saat itu.
func Retry(ctx context.Context, cfg RetryConfig, op func(ctx context.Context) error) error {
	backoff := cfg.BaseDelay
	for attempt := 1; ; attempt++ {
		err := op(ctx)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrPermanent) {
			return err
		}
		if attempt >= cfg.MaxAttempts {
			return fmt.Errorf("menyerah setelah %d percobaan: %w", attempt, err)
		}

		delay := time.Duration(cfg.Jitter() * float64(backoff))
		if err := cfg.Sleep(ctx, delay); err != nil {
			return fmt.Errorf("retry dibatalkan: %w", err)
		}
		backoff = min(backoff*2, cfg.MaxDelay)
	}
}
