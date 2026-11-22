package obs

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/platform"
)

// Metrics memegang registry sendiri supaya tiap instance service (dan tiap
// test) punya angka yang terpisah.
//
// TODO(step 7): tambahkan field untuk metric yang kamu daftarkan di New.
type Metrics struct {
	registry *prometheus.Registry
}

// New membuat registry dan mendaftarkan metric.
//
// TODO(step 7): daftarkan CounterVec channelsync_messages_processed_total
// dengan label consumer dan outcome. Jangan pernah memakai booking ID atau
// correlation ID sebagai label: setiap nilai label yang berbeda menjadi satu
// time series baru.
func New() *Metrics {
	return &Metrics{registry: prometheus.NewRegistry()}
}

// Handler melayani GET /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Instrument membungkus handler sebuah consumer.
//
// TODO(step 7): kembalikan handler baru yang
//  1. membuat logger dengan field consumer dan correlation_id (dari header
//     event.HeaderCorrelationID), lalu menitipkannya di context dengan
//     WithLogger supaya log dari handler ikut membawa field itu;
//  2. memanggil next;
//  3. menaikkan counter dengan outcome "ok" atau "error";
//  4. menulis satu baris log hasilnya.
func (m *Metrics) Instrument(consumer string, log *slog.Logger, next platform.Handler) platform.Handler {
	return next
}

// RegisterOutboxBacklog mendaftarkan metric backlog outbox. stats dipanggil
// saat /metrics dibaca.
//
// TODO(step 7): daftarkan dua prometheus.NewGaugeFunc: channelsync_outbox_pending
// dan channelsync_outbox_oldest_pending_age_seconds. Beri stats context dengan
// timeout, karena ia menjalankan query ke database.
func (m *Metrics) RegisterOutboxBacklog(stats func(ctx context.Context) (pending int, oldestAgeSeconds float64, err error)) {
}

// Dipakai di step 7. Hapus baris ini setelah terpakai.
var _ = event.HeaderCorrelationID
