// Command channelsync menjalankan service untuk dicoba dengan tangan.
//
// File ini sudah jadi. Jangan diubah.
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"belajar/eventdriven/proyek/internal/app"
	"belajar/eventdriven/proyek/internal/email"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("service berhenti karena error", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.Start(ctx, app.Config{
		PostgresURL:       cmp.Or(os.Getenv("ED_POSTGRES_URL"), "postgres://lab:lab@localhost:55432/lab"),
		Schema:            "channelsync",
		KafkaBrokers:      strings.Split(cmp.Or(os.Getenv("ED_KAFKA_BROKERS"), "localhost:59092"), ","),
		EventsTopic:       "channelsync.booking-events",
		DeadLetterTopic:   "channelsync.booking-events.dlq",
		GroupPrefix:       "channelsync",
		WebhookSecret:     os.Getenv("WEBHOOK_SECRET"),
		EmailSender:       email.LogSender{Log: log},
		Logger:            log,
		ForwarderInterval: 500 * time.Millisecond,
		EmailMaxAttempts:  5,
		EmailBackoff:      time.Second,
		ConsumerBackoff:   time.Second,
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cmp.Or(os.Getenv("ADDR"), "localhost:8080"),
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	log.Info("service berjalan", "addr", srv.Addr)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	// Urutan berhenti: HTTP dulu (tidak ada request baru), lalu consumer.
	log.Info("berhenti: menunggu pekerjaan yang sedang berjalan")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http shutdown", "error", err)
	}
	return a.Shutdown(shutdownCtx)
}
