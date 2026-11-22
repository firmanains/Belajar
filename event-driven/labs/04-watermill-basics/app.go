package lab04

// File ini yang kamu ubah.

import (
	"context"
	"net/http"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

// App adalah service pengirim confirmation email.
type App struct{}

// NewApp menyiapkan service. Versi ini belum mendaftarkan handler apa pun.
func NewApp(sub message.Subscriber, email Emailer, logger watermill.LoggerAdapter) (*App, error) {
	return &App{}, nil
}

// Run menjalankan service sampai ctx dibatalkan. Ia baru boleh kembali setelah
// pesan yang sedang diproses selesai.
func (a *App) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// HealthHandler menjawab 200 hanya selama service siap memproses pesan, dan
// 503 di luar itu.
func (a *App) HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	})
}
