package obs

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/platform"
)

// Metrics memegang registry sendiri supaya tiap instance service (dan tiap
// test) punya angka yang terpisah.
type Metrics struct {
	registry  *prometheus.Registry
	processed *prometheus.CounterVec
	duration  *prometheus.HistogramVec
}

func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		processed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "channelsync_messages_processed_total",
			Help: "Jumlah message yang selesai ditangani, per consumer dan outcome (ok atau error).",
		}, []string{"consumer", "outcome"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "channelsync_message_duration_seconds",
			Help:    "Lama handler memproses satu message.",
			Buckets: prometheus.DefBuckets,
		}, []string{"consumer"}),
	}
	m.registry.MustRegister(m.processed, m.duration)
	return m
}

// Handler melayani GET /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Instrument membungkus handler sebuah consumer. Label hanya berisi nilai yang
// jumlahnya terbatas (nama consumer, outcome). Booking ID dan correlation ID
// masuk log, bukan label.
func (m *Metrics) Instrument(consumer string, log *slog.Logger, next platform.Handler) platform.Handler {
	return func(ctx context.Context, r platform.Record) error {
		l := log.With(
			"consumer", consumer,
			"correlation_id", r.Headers[event.HeaderCorrelationID],
			"event_name", r.Headers[event.HeaderEventName],
			"partition", r.Partition,
			"offset", r.Offset,
		)
		start := time.Now()
		err := next(WithLogger(ctx, l), r)
		elapsed := time.Since(start)

		outcome := "ok"
		if err != nil {
			outcome = "error"
		}
		m.processed.WithLabelValues(consumer, outcome).Inc()
		m.duration.WithLabelValues(consumer).Observe(elapsed.Seconds())
		if err != nil {
			l.Error("message gagal diproses", "error", err, "duration_ms", elapsed.Milliseconds())
		} else {
			l.Info("message diproses", "duration_ms", elapsed.Milliseconds())
		}
		return err
	}
}

// RegisterOutboxBacklog mendaftarkan dua gauge yang dihitung saat /metrics
// dibaca. Jumlah pending menunjukkan besarnya antrean. Umur baris tertua
// menunjukkan apakah antrean itu bergerak: 50 baris berumur 1 detik itu sehat,
// 1 baris berumur 10 menit berarti forwarder macet.
func (m *Metrics) RegisterOutboxBacklog(stats func(ctx context.Context) (pending int, oldestAgeSeconds float64, err error)) {
	// shortcut: tiap gauge menjalankan query sendiri saat scrape; kalau jadi
	// mahal, ganti dengan satu prometheus.Collector yang query sekali.
	read := func() (int, float64) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pending, age, err := stats(ctx)
		if err != nil {
			return -1, -1 // nilai negatif terlihat jelas di grafik dan bisa di-alert
		}
		return pending, age
	}
	m.registry.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "channelsync_outbox_pending",
			Help: "Jumlah baris outbox yang belum terkirim ke Kafka (-1 kalau query gagal).",
		}, func() float64 { p, _ := read(); return float64(p) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "channelsync_outbox_oldest_pending_age_seconds",
			Help: "Umur baris outbox tertua yang belum terkirim (-1 kalau query gagal).",
		}, func() float64 { _, a := read(); return a }),
	)
}
