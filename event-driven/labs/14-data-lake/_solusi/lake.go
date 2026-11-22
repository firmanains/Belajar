package lab14

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Lake menyimpan semua event ke satu tabel Postgres.
type Lake struct {
	pool *pgxpool.Pool
}

func NewLake(pool *pgxpool.Pool) *Lake {
	return &Lake{pool: pool}
}

// Migrate membuat tabel events yang append-only.
// occurred_at adalah waktu kejadian menurut pengirim; recorded_at adalah waktu
// data lake menerimanya.
func (l *Lake) Migrate(ctx context.Context) error {
	_, err := l.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS events (
			event_id    text        PRIMARY KEY,
			name        text        NOT NULL,
			occurred_at timestamptz NOT NULL,
			payload     jsonb       NOT NULL,
			recorded_at timestamptz NOT NULL DEFAULT now()
		);

		-- Query selalu menyaring name lalu rentang occurred_at.
		CREATE INDEX IF NOT EXISTS events_name_occurred_at ON events (name, occurred_at);

		CREATE OR REPLACE FUNCTION events_append_only() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'tabel events append-only: % ditolak', TG_OP;
		END;
		$$ LANGUAGE plpgsql;

		CREATE OR REPLACE TRIGGER events_append_only
			BEFORE UPDATE OR DELETE ON events
			FOR EACH ROW EXECUTE FUNCTION events_append_only();`)
	if err != nil {
		return fmt.Errorf("membuat tabel events: %w", err)
	}
	return nil
}

// Append menyimpan satu event. Event dengan ID yang sudah ada dilewati tanpa
// error, karena forwarder menerima event at-least-once.
func (l *Lake) Append(ctx context.Context, e Envelope) error {
	_, err := l.pool.Exec(ctx,
		`INSERT INTO events (event_id, name, occurred_at, payload) VALUES ($1, $2, $3, $4::jsonb)
		 ON CONFLICT (event_id) DO NOTHING`,
		e.ID, e.Name, e.OccurredAt, string(e.Payload))
	if err != nil {
		return fmt.Errorf("menyimpan event %s: %w", e.ID, err)
	}
	return nil
}

// Forward membaca event dari in sampai channel ditutup atau ctx selesai, dan
// menyimpannya apa adanya. Data lake tidak menafsirkan payload dan tidak
// memilih event berdasarkan nama.
func (l *Lake) Forward(ctx context.Context, in <-chan Envelope) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e, ok := <-in:
			if !ok {
				return nil
			}
			if err := l.Append(ctx, e); err != nil {
				return err
			}
		}
	}
}

// Query mengembalikan event bernama name yang terjadi pada rentang
// [from, to): from termasuk, to tidak. Hasil diurutkan dari yang paling lama.
func (l *Lake) Query(ctx context.Context, name string, from, to time.Time) ([]Envelope, error) {
	rows, err := l.pool.Query(ctx,
		`SELECT event_id, name, occurred_at, payload::text
		   FROM events
		  WHERE name = $1 AND occurred_at >= $2 AND occurred_at < $3
		  ORDER BY occurred_at, event_id`, name, from, to)
	if err != nil {
		return nil, fmt.Errorf("query events %s: %w", name, err)
	}
	defer rows.Close()

	var out []Envelope
	for rows.Next() {
		var e Envelope
		var payload string
		if err := rows.Scan(&e.ID, &e.Name, &e.OccurredAt, &payload); err != nil {
			return nil, fmt.Errorf("membaca baris events: %w", err)
		}
		e.Payload = json.RawMessage(payload)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("membaca events: %w", err)
	}
	return out, nil
}
