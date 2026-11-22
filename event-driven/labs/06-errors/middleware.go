package lab06

// File ini yang kamu ubah (Step 1, 2, 4, 5).

import (
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
)

// RetryConfig mengatur berapa kali dan seberapa sabar Retry mencoba.
type RetryConfig struct {
	// MaxAttempts adalah jumlah percobaan total, termasuk yang pertama.
	MaxAttempts int
	// InitialInterval adalah jeda sebelum percobaan kedua.
	InitialInterval time.Duration
	// Multiplier mengalikan jeda setiap kali gagal lagi.
	Multiplier float64
}

// Retry mengulang handler di tempat ketika ia mengembalikan error.
// Versi ini belum melakukan apa-apa.
func Retry(cfg RetryConfig, logger *slog.Logger) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return h
	}
}

// Park menyingkirkan message yang gagal ke ParkingLot supaya message di
// belakangnya bisa diproses. Versi ini belum melakukan apa-apa.
func Park(lot *ParkingLot, logger *slog.Logger) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return h
	}
}

// Middlewares menentukan middleware yang dipasang di router dan urutannya.
// Yang pertama di slice menjadi lapisan terluar.
func Middlewares(retry RetryConfig, lot *ParkingLot, logger *slog.Logger) []message.HandlerMiddleware {
	return nil
}
