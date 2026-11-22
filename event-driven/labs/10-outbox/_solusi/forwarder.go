package lab10

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Forwarder memindahkan baris outbox ke broker.
type Forwarder struct {
	pool *pgxpool.Pool
	pub  Publisher

	// BatchSize adalah jumlah baris maksimum per RunOnce.
	BatchSize int
	// MaxAttempts adalah batas percobaan sebelum sebuah baris menjadi DEAD.
	MaxAttempts int
}

func NewForwarder(pool *pgxpool.Pool, pub Publisher) *Forwarder {
	return &Forwarder{pool: pool, pub: pub, BatchSize: 50, MaxAttempts: 3}
}

type outboxRow struct {
	id      int64
	topic   string
	payload []byte
}

// RunOnce memproses satu batch baris PENDING, urut berdasarkan id.
//
// Baris dikunci dengan FOR UPDATE SKIP LOCKED sampai transaction selesai.
// Forwarder lain yang berjalan bersamaan melewati baris terkunci dan mengambil
// baris berikutnya, jadi tidak ada baris yang dikerjakan dua forwarder sekaligus.
func (f *Forwarder) RunOnce(ctx context.Context) (int, error) {
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	// Rollback setelah Commit berhasil tidak berbuat apa-apa, jadi error-nya diabaikan.
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, topic, payload
		FROM outbox
		WHERE status = $1
		ORDER BY id
		LIMIT $2
		FOR UPDATE SKIP LOCKED`, StatusPending, f.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("select outbox: %w", err)
	}
	batch, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (outboxRow, error) {
		var o outboxRow
		err := r.Scan(&o.id, &o.topic, &o.payload)
		return o, err
	})
	if err != nil {
		return 0, fmt.Errorf("baca outbox: %w", err)
	}

	published := 0
	for _, o := range batch {
		msg := Message{ID: strconv.FormatInt(o.id, 10), Topic: o.topic, Payload: o.payload}
		if pubErr := f.pub.Publish(ctx, msg); pubErr != nil {
			// Publish gagal dicatat di barisnya; baris berikutnya tetap dikerjakan.
			if _, err := tx.Exec(ctx, `
				UPDATE outbox
				SET attempts   = attempts + 1,
				    last_error = $2,
				    status     = CASE WHEN attempts + 1 >= $3 THEN $4 ELSE status END
				WHERE id = $1`, o.id, pubErr.Error(), f.MaxAttempts, StatusDead); err != nil {
				return published, fmt.Errorf("catat kegagalan outbox %d: %w", o.id, err)
			}
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE outbox SET status = $2, sent_at = now() WHERE id = $1`,
			o.id, StatusSent); err != nil {
			return published, fmt.Errorf("tandai outbox %d terkirim: %w", o.id, err)
		}
		published++
	}

	if err := tx.Commit(ctx); err != nil {
		// Message sudah di broker tetapi tandanya tidak tersimpan: batch ini akan
		// terkirim lagi di RunOnce berikutnya. Itulah at-least-once.
		return 0, fmt.Errorf("commit: %w", err)
	}
	return published, nil
}
