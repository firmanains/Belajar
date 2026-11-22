package outbox

import (
	"context"
	"fmt"
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

type pendingRow struct {
	id            int64
	bookingID     string
	eventID       string
	eventName     string
	correlationID string
	payload       []byte
}

// ForwardBatch mengirim satu batch baris yang belum terkirim dan mengembalikan
// jumlah yang berhasil. Baris ditandai terkirim hanya setelah broker memberi
// ack. Kalau proses mati di antara publish dan commit, baris itu dikirim lagi:
// forwarder ini at-least-once, tidak pernah kehilangan dan kadang dobel.
func (f *Forwarder) ForwardBatch(ctx context.Context) (int, error) {
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // tidak berefek setelah Commit

	// SKIP LOCKED: forwarder lain yang berjalan bersamaan melewati baris yang
	// sedang dipegang, bukan menunggu dan bukan mengirim baris yang sama.
	rows, err := tx.Query(ctx, `
		SELECT id, booking_id, event_id, event_name, correlation_id, payload
		FROM outbox
		WHERE sent_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, f.batchSize)
	if err != nil {
		return 0, fmt.Errorf("select outbox: %w", err)
	}
	var batch []pendingRow
	for rows.Next() {
		var r pendingRow
		if err := rows.Scan(&r.id, &r.bookingID, &r.eventID, &r.eventName, &r.correlationID, &r.payload); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox: %w", err)
		}
		batch = append(batch, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("select outbox: %w", err)
	}

	// Berhenti di kegagalan pertama supaya urutan per booking tidak terlompati.
	var sent []int64
	var publishErr error
	for _, r := range batch {
		publishErr = f.pub.Publish(ctx, f.topic, platform.Message{
			Key:   []byte(r.bookingID), // key = booking ID: event satu booking selalu di partition yang sama
			Value: r.payload,
			Headers: map[string]string{
				event.HeaderEventID:       r.eventID,
				event.HeaderEventName:     r.eventName,
				event.HeaderCorrelationID: r.correlationID,
			},
		})
		if publishErr != nil {
			break
		}
		sent = append(sent, r.id)
	}

	if len(sent) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE outbox SET sent_at = now() WHERE id = ANY($1)`, sent); err != nil {
			return 0, fmt.Errorf("mark sent: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	if publishErr != nil {
		return len(sent), fmt.Errorf("publish event: %w", publishErr)
	}
	return len(sent), nil
}
