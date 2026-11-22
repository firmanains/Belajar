package lab19

import (
	"context"
	"time"
)

// File ini yang kamu ubah di Step 2.

// RetryConfig mengatur perilaku Retry. Sleep dan Jitter disuntikkan supaya
// test tidak perlu menunggu sungguhan dan tidak bergantung pada angka acak.
type RetryConfig struct {
	MaxAttempts int           // jumlah percobaan total, termasuk yang pertama
	BaseDelay   time.Duration // backoff sebelum percobaan kedua
	MaxDelay    time.Duration // batas atas backoff
	Sleep       func(ctx context.Context, d time.Duration) error
	Jitter      func() float64 // angka di rentang [0, 1)
}

// Retry menjalankan op sampai berhasil atau menyerah. Versi ini hanya
// mencoba satu kali.
func Retry(ctx context.Context, cfg RetryConfig, op func(ctx context.Context) error) error {
	return op(ctx)
}
