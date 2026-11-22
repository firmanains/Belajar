package lab17

import (
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/prometheus/client_golang/prometheus"
)

// File ini yang kamu ubah.

// Metrics memegang semua metric milik consumer. Field untuk metric
// (counter, histogram) kamu tambahkan sendiri.
type Metrics struct {
	now func() time.Time
}

// NewMetrics membuat metric dan mendaftarkannya ke reg. now disuntikkan
// supaya test bisa mengatur waktu.
func NewMetrics(reg prometheus.Registerer, now func() time.Time) *Metrics {
	return &Metrics{now: now}
}

// Middleware membungkus sebuah handler dan mencatat hasil serta lama
// pemrosesannya. Versi ini belum mencatat apa pun.
func (m *Metrics) Middleware(handlerName string) message.HandlerMiddleware {
	return func(next message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			return next(msg)
		}
	}
}

// RegisterQueueMetrics mendaftarkan gauge untuk kondisi antrean: jumlah
// message yang menunggu dan umur message tertua. Versi ini belum mendaftarkan
// apa pun.
func RegisterQueueMetrics(reg prometheus.Registerer, queue string, b *Backlog, now func() time.Time) error {
	return nil
}
