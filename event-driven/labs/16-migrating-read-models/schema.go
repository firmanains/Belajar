package lab16

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// File ini sudah jadi. Jangan diubah.

const (
	BookingCreated   = "BookingCreated"
	BookingCancelled = "BookingCancelled"
)

// Envelope adalah event seperti yang tersimpan di data lake.
type Envelope struct {
	ID         string
	Name       string
	OccurredAt time.Time
	Payload    json.RawMessage
}

// BookingPayload adalah isi BookingCreated dan BookingCancelled. Kedua event
// membawa data booking lengkap, jadi handler tidak perlu mencari booking-nya.
type BookingPayload struct {
	BookingID string `json:"booking_id"`
	RoomType  string `json:"room_type"`
	RatePlan  string `json:"rate_plan"`
	StayDate  string `json:"stay_date"`
	Rooms     int    `json:"rooms"`
}

// Decode membaca payload dan mengembalikan perubahan jumlah kamar terjual:
// positif untuk BookingCreated, negatif untuk BookingCancelled.
func Decode(ev Envelope) (BookingPayload, int, error) {
	var p BookingPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return BookingPayload{}, 0, fmt.Errorf("payload event %s: %w", ev.ID, err)
	}
	switch ev.Name {
	case BookingCreated:
		return p, p.Rooms, nil
	case BookingCancelled:
		return p, -p.Rooms, nil
	default:
		return BookingPayload{}, 0, fmt.Errorf("event %s: nama %q tidak dikenal", ev.ID, ev.Name)
	}
}

// DBTX dipenuhi oleh *pgxpool.Pool dan pgx.Tx, supaya fungsi yang sama bisa
// dipakai di dalam maupun di luar transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Migrate membuat data lake (events), read model lama (v1), read model baru
// (v2), dan penanda versi mana yang sedang dibaca aplikasi.
func Migrate(ctx context.Context, db DBTX) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE events (
			event_id    text        PRIMARY KEY,
			name        text        NOT NULL,
			occurred_at timestamptz NOT NULL,
			payload     jsonb       NOT NULL
		);

		-- v1: kamar terjual per tipe kamar dan tanggal. Rate plan tidak disimpan.
		CREATE TABLE availability_v1 (
			room_type  text NOT NULL,
			stay_date  text NOT NULL,
			rooms_sold int  NOT NULL,
			PRIMARY KEY (room_type, stay_date)
		);
		CREATE TABLE applied_v1 (event_id text PRIMARY KEY);

		-- v2: kamar terjual per tipe kamar, rate plan, dan tanggal.
		CREATE TABLE availability_v2 (
			room_type  text NOT NULL,
			rate_plan  text NOT NULL,
			stay_date  text NOT NULL,
			rooms_sold int  NOT NULL,
			PRIMARY KEY (room_type, rate_plan, stay_date)
		);
		CREATE TABLE applied_v2 (event_id text PRIMARY KEY);

		CREATE TABLE read_model_version (
			only_row bool PRIMARY KEY DEFAULT true CHECK (only_row),
			active   int  NOT NULL
		);
		INSERT INTO read_model_version (active) VALUES (1);`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// ApplyV1 menerapkan satu event ke read model v1 secara idempotent: event
// yang ID-nya sudah tercatat di applied_v1 dilewati. Panggil di dalam
// transaction, supaya catatan dan perubahan angka tersimpan bersama.
func ApplyV1(ctx context.Context, db DBTX, ev Envelope) error {
	tag, err := db.Exec(ctx, `INSERT INTO applied_v1 (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, ev.ID)
	if err != nil {
		return fmt.Errorf("v1: mencatat event %s: %w", ev.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return nil // sudah pernah diterapkan
	}
	p, delta, err := Decode(ev)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO availability_v1 (room_type, stay_date, rooms_sold) VALUES ($1, $2, $3)
		ON CONFLICT (room_type, stay_date)
		DO UPDATE SET rooms_sold = availability_v1.rooms_sold + EXCLUDED.rooms_sold`,
		p.RoomType, p.StayDate, delta)
	if err != nil {
		return fmt.Errorf("v1: menerapkan event %s: %w", ev.ID, err)
	}
	return nil
}

// ResetV2 mengosongkan read model v2 supaya bisa dibangun ulang dari nol.
// Aman dilakukan kapan saja selama aplikasi belum membaca dari v2.
func ResetV2(ctx context.Context, db DBTX) error {
	if _, err := db.Exec(ctx, `TRUNCATE availability_v2, applied_v2`); err != nil {
		return fmt.Errorf("mengosongkan v2: %w", err)
	}
	return nil
}

// KeyV1 dan KeyV2 adalah kunci baris kedua read model.
type KeyV1 struct{ RoomType, StayDate string }
type KeyV2 struct{ RoomType, RatePlan, StayDate string }

// AvailabilityV1 membaca seluruh read model v1. Baris bernilai 0 dilewati.
func AvailabilityV1(ctx context.Context, db DBTX) (map[KeyV1]int, error) {
	rows, err := db.Query(ctx, `SELECT room_type, stay_date, rooms_sold FROM availability_v1 WHERE rooms_sold <> 0`)
	if err != nil {
		return nil, fmt.Errorf("membaca v1: %w", err)
	}
	defer rows.Close()
	out := map[KeyV1]int{}
	for rows.Next() {
		var k KeyV1
		var n int
		if err := rows.Scan(&k.RoomType, &k.StayDate, &n); err != nil {
			return nil, fmt.Errorf("membaca baris v1: %w", err)
		}
		out[k] = n
	}
	return out, rows.Err()
}

// AvailabilityV2 membaca seluruh read model v2. Baris bernilai 0 dilewati.
func AvailabilityV2(ctx context.Context, db DBTX) (map[KeyV2]int, error) {
	rows, err := db.Query(ctx, `SELECT room_type, rate_plan, stay_date, rooms_sold FROM availability_v2 WHERE rooms_sold <> 0`)
	if err != nil {
		return nil, fmt.Errorf("membaca v2: %w", err)
	}
	defer rows.Close()
	out := map[KeyV2]int{}
	for rows.Next() {
		var k KeyV2
		var n int
		if err := rows.Scan(&k.RoomType, &k.RatePlan, &k.StayDate, &n); err != nil {
			return nil, fmt.Errorf("membaca baris v2: %w", err)
		}
		out[k] = n
	}
	return out, rows.Err()
}

// ActiveVersion mengembalikan versi read model yang sedang dibaca aplikasi.
func ActiveVersion(ctx context.Context, db DBTX) (int, error) {
	var v int
	if err := db.QueryRow(ctx, `SELECT active FROM read_model_version`).Scan(&v); err != nil {
		return 0, fmt.Errorf("membaca versi aktif: %w", err)
	}
	return v, nil
}

// Diff adalah satu ketidakcocokan antara v1 dan v2 untuk satu tipe kamar dan
// tanggal. V2 adalah jumlah semua rate plan di v2.
type Diff struct {
	RoomType string
	StayDate string
	V1       int
	V2       int
}
