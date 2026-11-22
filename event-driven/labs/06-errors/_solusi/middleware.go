package lab06

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
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

// retryable memutuskan apakah mengulang masuk akal. Payload rusak dan bug
// kode akan gagal dengan cara yang sama pada percobaan berikutnya.
func retryable(err error) bool {
	var panicErr middleware.RecoveredPanicError
	return !IsPermanent(err) && !errors.As(err, &panicErr)
}

// Retry mengulang handler di tempat ketika ia mengembalikan error yang
// sifatnya sementara, dengan jeda yang makin panjang dan jumlah yang dibatasi.
func Retry(cfg RetryConfig, logger *slog.Logger) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			delay := cfg.InitialInterval
			for attempt := 1; ; attempt++ {
				produced, err := h(msg)
				if err == nil {
					return produced, nil
				}
				if !retryable(err) || attempt >= cfg.MaxAttempts {
					return nil, err
				}

				// WARN, bukan ERROR: belum ada yang perlu ditindaklanjuti manusia.
				logger.Warn("retrying message",
					"message_uuid", msg.UUID,
					"attempt", attempt,
					"delay_ms", delay.Milliseconds(),
					"error", err.Error(),
				)

				select {
				case <-time.After(delay):
				case <-msg.Context().Done():
					return nil, msg.Context().Err()
				}
				delay = time.Duration(float64(delay) * cfg.Multiplier)
			}
		}
	}
}

// Park menyingkirkan message yang gagal ke ParkingLot supaya message di
// belakangnya bisa diproses. Error yang sampai ke sini sudah final: permanen,
// atau sudah habis jatah retry.
func Park(lot *ParkingLot, logger *slog.Logger) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			produced, err := h(msg)
			if err == nil {
				return produced, nil
			}
			// Service sedang berhenti: biarkan nack, supaya message diantar
			// lagi setelah service hidup kembali.
			if errors.Is(err, context.Canceled) {
				return nil, err
			}

			lot.Add(msg, err)
			// Satu-satunya baris ERROR untuk message ini.
			logger.Error("message parked", "message_uuid", msg.UUID, "error", err.Error())

			// nil berarti ack: message sudah aman di ParkingLot.
			return nil, nil
		}
	}
}

// Middlewares menentukan middleware yang dipasang di router dan urutannya.
// Recoverer paling dalam supaya panic berubah menjadi error sebelum melewati
// Retry dan Park.
func Middlewares(retry RetryConfig, lot *ParkingLot, logger *slog.Logger) []message.HandlerMiddleware {
	return []message.HandlerMiddleware{
		Park(lot, logger),
		Retry(retry, logger),
		middleware.Recoverer,
	}
}
