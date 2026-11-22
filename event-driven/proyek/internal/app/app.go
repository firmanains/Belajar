// Package app merangkai semua bagian menjadi satu service.
//
// File ini sudah jadi. Jangan diubah.
package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"belajar/eventdriven/proyek/internal/email"
	"belajar/eventdriven/proyek/internal/inbox"
	"belajar/eventdriven/proyek/internal/obs"
	"belajar/eventdriven/proyek/internal/outbox"
	"belajar/eventdriven/proyek/internal/platform"
	"belajar/eventdriven/proyek/internal/readmodel"
	"belajar/eventdriven/proyek/internal/store"
)

const HeaderSecret = "X-Webhook-Secret"

type Config struct {
	PostgresURL  string
	Schema       string
	KafkaBrokers []string

	EventsTopic     string
	DeadLetterTopic string
	GroupPrefix     string // consumer group: <prefix>.projector dan <prefix>.email

	WebhookSecret string
	EmailSender   email.Sender
	Logger        *slog.Logger

	ForwarderInterval time.Duration // jeda polling outbox
	EmailMaxAttempts  int
	EmailBackoff      time.Duration // jeda sebelum percobaan kedua; berlipat dua tiap percobaan
	ConsumerBackoff   time.Duration // jeda sebelum consumer mengulang record yang handler-nya gagal

	Faults *platform.Faults // hanya untuk test
}

type App struct {
	handler       http.Handler
	pool          *pgxpool.Pool
	publisher     *platform.KafkaPublisher
	consumers     []*platform.Consumer
	stopForwarder context.CancelFunc
	forwarderDone chan struct{}
}

// Start menyambung ke Postgres dan Kafka, lalu menjalankan forwarder dan kedua
// consumer di background.
func Start(ctx context.Context, cfg Config) (*App, error) {
	if cfg.WebhookSecret == "" {
		return nil, errors.New("WebhookSecret wajib diisi")
	}
	log := cfg.Logger

	pool, err := platform.Connect(ctx, cfg.PostgresURL, cfg.Schema)
	if err != nil {
		return nil, err
	}
	if err := platform.EnsureTopics(ctx, cfg.KafkaBrokers, map[string]int32{cfg.EventsTopic: 3, cfg.DeadLetterTopic: 1}); err != nil {
		pool.Close()
		return nil, err
	}
	pub, err := platform.NewKafkaPublisher(cfg.KafkaBrokers, cfg.Faults)
	if err != nil {
		pool.Close()
		return nil, err
	}

	metrics := obs.New()
	outboxStore := outbox.NewStore(cfg.Faults)
	metrics.RegisterOutboxBacklog(func(ctx context.Context) (int, float64, error) {
		st, err := outboxStore.Stats(ctx, pool)
		return st.Pending, st.OldestPendingAgeSeconds, err
	})

	a := &App{pool: pool, publisher: pub, forwarderDone: make(chan struct{})}

	projector := readmodel.NewProjector(pool, log)
	emailer := email.NewConsumer(pool, cfg.EmailSender, pub, cfg.DeadLetterTopic, cfg.EmailMaxAttempts, cfg.EmailBackoff, log)
	for name, h := range map[string]platform.Handler{"projector": projector.Handle, "email": emailer.Handle} {
		c, err := platform.NewConsumer(cfg.KafkaBrokers, cfg.GroupPrefix+"."+name, cfg.EventsTopic, name,
			cfg.ConsumerBackoff, metrics.Instrument(name, log, h), log)
		if err != nil {
			return nil, fmt.Errorf("start consumer: %w", err)
		}
		a.consumers = append(a.consumers, c)
		go c.Run()
	}

	fwdCtx, stop := context.WithCancel(context.Background())
	a.stopForwarder = stop
	fwd := outbox.NewForwarder(pool, pub, cfg.EventsTopic, cfg.ForwarderInterval, log)
	go func() {
		defer close(a.forwarderDone)
		fwd.Run(fwdCtx)
	}()

	secured := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderSecret)), []byte(cfg.WebhookSecret)) != 1 {
				platform.WriteError(w, http.StatusUnauthorized, "secret salah atau tidak ada")
				return
			}
			h.ServeHTTP(w, r)
		})
	}
	mux := http.NewServeMux()
	mux.Handle("POST /webhooks/booking-revisions", secured(inbox.Webhook(inbox.NewService(pool, outboxStore), log)))
	mux.Handle("GET /admin/availability", secured(readmodel.AvailabilityHandler(pool, store.WriteModel, log)))
	mux.Handle("GET /admin/outbox", secured(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st, err := outboxStore.Stats(r.Context(), pool)
		if err != nil {
			platform.WriteError(w, http.StatusInternalServerError, "gagal membaca outbox")
			return
		}
		platform.WriteJSON(w, http.StatusOK, st)
	})))
	mux.Handle("GET /tape-chart", readmodel.AvailabilityHandler(pool, store.ReadModel, log))
	mux.Handle("GET /bookings/{id}", readmodel.BookingHandler(pool, log))
	mux.Handle("GET /metrics", metrics.Handler())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			platform.WriteError(w, http.StatusServiceUnavailable, "postgres tidak terjangkau")
			return
		}
		platform.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	a.handler = mux
	return a, nil
}

// Handler mengembalikan semua endpoint HTTP service ini.
func (a *App) Handler() http.Handler { return a.handler }

// Shutdown menghentikan forwarder dan consumer, lalu menutup sambungan.
// Hentikan HTTP server lebih dulu supaya tidak ada request yang masih berjalan.
func (a *App) Shutdown(ctx context.Context) error {
	a.stopForwarder()
	<-a.forwarderDone

	errs := make([]error, len(a.consumers))
	var wg sync.WaitGroup
	for i, c := range a.consumers {
		wg.Go(func() { errs[i] = c.Shutdown(ctx) })
	}
	wg.Wait()

	a.publisher.Close()
	a.pool.Close()
	return errors.Join(errs...)
}
