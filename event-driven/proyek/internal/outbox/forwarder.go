package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/platform"
)

// Forwarder memindahkan baris outbox ke Kafka.
type Forwarder struct {
	pool      *pgxpool.Pool
	pub       platform.Publisher
	topic     string
	interval  time.Duration
	batchSize int
	log       *slog.Logger
}

func NewForwarder(pool *pgxpool.Pool, pub platform.Publisher, topic string, interval time.Duration, log *slog.Logger) *Forwarder {
	return &Forwarder{pool: pool, pub: pub, topic: topic, interval: interval, batchSize: 100, log: log}
}

// Run memanggil ForwardBatch setiap interval sampai ctx dibatalkan.
func (f *Forwarder) Run(ctx context.Context) {
	t := time.NewTicker(f.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		for { // kuras sampai outbox kosong
			n, err := f.ForwardBatch(ctx)
			if err != nil {
				if ctx.Err() == nil {
					f.log.Warn("forward outbox gagal, dicoba lagi di putaran berikutnya", "error", err)
				}
				break
			}
			if n < f.batchSize {
				break
			}
		}
	}
}

// ForwardBatch mengirim satu batch baris outbox yang belum terkirim ke Kafka dan
// mengembalikan jumlah baris yang berhasil dikirim.
//
// TODO(step 4): dalam satu transaction:
//  1. Ambil paling banyak f.batchSize baris dengan sent_at IS NULL, urut id,
//     memakai FOR UPDATE SKIP LOCKED. Kolom: id, booking_id, event_id,
//     event_name, correlation_id, payload.
//  2. Publish tiap baris ke f.topic: key = booking ID, value = payload, header
//     event.HeaderEventID, event.HeaderEventName, event.HeaderCorrelationID.
//     Berhenti di kegagalan pertama.
//  3. Isi sent_at = now() hanya untuk baris yang publish-nya berhasil, lalu commit.
//
// Urutan "publish dulu, tandai kemudian" adalah intinya. Pikirkan apa yang
// terjadi kalau proses mati di antara kedua langkah itu, untuk kedua urutan.
func (f *Forwarder) ForwardBatch(ctx context.Context) (int, error) {
	return 0, nil
}

// Dipakai di step 4. Hapus baris ini setelah terpakai.
var _ = event.HeaderEventID
