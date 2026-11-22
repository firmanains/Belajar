package lab17

import (
	"fmt"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics memegang semua metric milik consumer.
type Metrics struct {
	now       func() time.Time
	processed *prometheus.CounterVec
	duration  *prometheus.HistogramVec
}

// NewMetrics membuat metric dan mendaftarkannya ke reg. now disuntikkan
// supaya test bisa mengatur waktu.
func NewMetrics(reg prometheus.Registerer, now func() time.Time) *Metrics {
	m := &Metrics{
		now: now,
		processed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pms_messages_processed_total",
			Help: "Jumlah message yang selesai diproses, per handler dan outcome.",
		}, []string{"handler", "outcome"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "pms_message_processing_duration_seconds",
			Help: "Lama satu message diproses oleh handler.",
			// Handler di sini memanggil SMTP dan HTTP, jadi bucket mulai dari
			// puluhan milidetik sampai puluhan detik.
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"handler"}),
	}
	reg.MustRegister(m.processed, m.duration)
	return m
}

// Middleware membungkus sebuah handler dan mencatat hasil serta lama
// pemrosesannya. Label hanya handler dan outcome: keduanya punya jumlah nilai
// yang kecil dan tetap.
func (m *Metrics) Middleware(handlerName string) message.HandlerMiddleware {
	return func(next message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			start := m.now()
			produced, err := next(msg)

			outcome := "success"
			if err != nil {
				outcome = "failure"
			}
			m.processed.WithLabelValues(handlerName, outcome).Inc()
			// Durasi dicatat juga untuk yang gagal: timeout 30 detik adalah
			// informasi yang paling ingin dilihat.
			m.duration.WithLabelValues(handlerName).Observe(m.now().Sub(start).Seconds())
			return produced, err
		}
	}
}

// RegisterQueueMetrics mendaftarkan gauge untuk kondisi antrean. GaugeFunc
// dihitung saat Prometheus melakukan scrape, jadi nilainya tetap bergerak
// walaupun consumer berhenti total.
func RegisterQueueMetrics(reg prometheus.Registerer, queue string, b *Backlog, now func() time.Time) error {
	labels := prometheus.Labels{"queue": queue}

	backlog := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name:        "pms_queue_backlog_messages",
		Help:        "Jumlah message yang belum diproses.",
		ConstLabels: labels,
	}, func() float64 { return float64(b.Len()) })
	if err := reg.Register(backlog); err != nil {
		return fmt.Errorf("register backlog gauge: %w", err)
	}

	oldest := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name:        "pms_queue_oldest_message_age_seconds",
		Help:        "Umur message tertua yang belum diproses.",
		ConstLabels: labels,
	}, func() float64 { return b.OldestAge(now()).Seconds() })
	if err := reg.Register(oldest); err != nil {
		return fmt.Errorf("register oldest message age gauge: %w", err)
	}
	return nil
}
