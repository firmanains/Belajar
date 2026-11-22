package lab04

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

// App adalah service pengirim confirmation email.
type App struct {
	router *message.Router
}

// NewApp membuat Router dan mendaftarkan satu handler. Tidak ada consume loop:
// Router yang melakukan subscribe, memanggil handler, lalu mengirim ack atau
// nack sesuai nilai yang dikembalikan handler.
func NewApp(sub message.Subscriber, email Emailer, logger watermill.LoggerAdapter) (*App, error) {
	router, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	router.AddConsumerHandler(
		"send_confirmation_email",
		TopicBookingCreated,
		sub,
		func(msg *message.Message) error {
			var event BookingCreated
			if err := json.Unmarshal(msg.Payload, &event); err != nil || event.BookingID == "" {
				// Pesan ini tidak akan pernah bisa diproses, berapa kali pun
				// dikirim ulang. Return nil berarti ack: pesan dibuang supaya
				// pesan di belakangnya tidak tertahan.
				logger.Error("dropping malformed message", err, watermill.LogFields{"message_uuid": msg.UUID})
				return nil
			}
			if err := email.SendConfirmation(msg.Context(), event.BookingID); err != nil {
				// Return error berarti nack: Pub/Sub akan mengirim ulang.
				return fmt.Errorf("send confirmation for %s: %w", event.BookingID, err)
			}
			return nil
		},
	)
	return &App{router: router}, nil
}

// Run menyerahkan ctx ke Router. Saat ctx dibatalkan, Router berhenti
// mengambil pesan baru, menunggu handler yang sedang berjalan, lalu kembali.
func (a *App) Run(ctx context.Context) error {
	if err := a.router.Run(ctx); err != nil {
		return fmt.Errorf("run router: %w", err)
	}
	return nil
}

// HealthHandler mengikuti keadaan Router: siap setelah semua handler
// melakukan subscribe, tidak siap lagi setelah Router ditutup.
func (a *App) HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !a.router.IsRunning() || a.router.IsClosed() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}
