// Package outbox menyimpan event yang belum terkirim dan meneruskannya ke Kafka.
package outbox

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/platform"
)

type Store struct {
	faults *platform.Faults
}

func NewStore(faults *platform.Faults) *Store { return &Store{faults: faults} }

// Insert menulis satu event ke tabel outbox. Panggil dengan transaction yang
// sama dengan perubahan datanya: event tersimpan kalau dan hanya kalau
// perubahannya tersimpan.
func (s *Store) Insert(ctx context.Context, q platform.DBTX, env event.Envelope) error {
	if s.faults.FailOutboxInsert() {
		return errors.New("fault injection: insert outbox gagal")
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO outbox (event_id, event_name, booking_id, correlation_id, payload)
		VALUES ($1, $2, $3, $4, $5)`,
		env.EventID, env.EventName, env.Booking.BookingID, env.CorrelationID, payload)
	if err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	return nil
}

// Stats menggambarkan backlog outbox.
type Stats struct {
	Pending                 int     `json:"pending"`
	Sent                    int     `json:"sent"`
	Total                   int     `json:"total"`
	OldestPendingAgeSeconds float64 `json:"oldest_pending_age_seconds"`
}

func (s *Store) Stats(ctx context.Context, q platform.DBTX) (Stats, error) {
	var st Stats
	err := q.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE sent_at IS NULL),
		       count(*) FILTER (WHERE sent_at IS NOT NULL),
		       COALESCE(EXTRACT(EPOCH FROM now() - min(created_at) FILTER (WHERE sent_at IS NULL)), 0)::float8
		FROM outbox`).Scan(&st.Pending, &st.Sent, &st.OldestPendingAgeSeconds)
	if err != nil {
		return st, fmt.Errorf("outbox stats: %w", err)
	}
	st.Total = st.Pending + st.Sent
	return st, nil
}
