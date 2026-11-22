package lab16

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ApplyV2 menerapkan satu event ke read model v2 secara idempotent: event
// yang ID-nya sudah tercatat di applied_v2 dilewati. Panggil di dalam
// transaction, supaya catatan dan perubahan angka tersimpan bersama.
func ApplyV2(ctx context.Context, db DBTX, ev Envelope) error {
	tag, err := db.Exec(ctx, `INSERT INTO applied_v2 (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, ev.ID)
	if err != nil {
		return fmt.Errorf("v2: mencatat event %s: %w", ev.ID, err)
	}
	if tag.RowsAffected() == 0 {
		return nil // sudah diterapkan, oleh replay sebelumnya atau oleh jalur live
	}
	p, delta, err := Decode(ev)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO availability_v2 (room_type, rate_plan, stay_date, rooms_sold) VALUES ($1, $2, $3, $4)
		ON CONFLICT (room_type, rate_plan, stay_date)
		DO UPDATE SET rooms_sold = availability_v2.rooms_sold + EXCLUDED.rooms_sold`,
		p.RoomType, p.RatePlan, p.StayDate, delta)
	if err != nil {
		return fmt.Errorf("v2: menerapkan event %s: %w", ev.ID, err)
	}
	return nil
}

// Rebuild membangun read model v2 dengan memutar ulang seluruh sejarah dari
// data lake. Aman dijalankan berulang dan aman dihentikan di tengah, karena
// ApplyV2 melewati event yang sudah diterapkan.
func Rebuild(ctx context.Context, pool *pgxpool.Pool) error {
	// shortcut: seluruh sejarah dimuat ke memory; pakai keyset pagination
	// (WHERE (occurred_at, event_id) > ($1, $2) LIMIT n) saat data lake besar.
	rows, err := pool.Query(ctx, `SELECT event_id, name, occurred_at, payload::text FROM events ORDER BY occurred_at, event_id`)
	if err != nil {
		return fmt.Errorf("rebuild v2: membaca data lake: %w", err)
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Envelope, error) {
		var ev Envelope
		var payload string
		err := row.Scan(&ev.ID, &ev.Name, &ev.OccurredAt, &payload)
		ev.Payload = json.RawMessage(payload)
		return ev, err
	})
	if err != nil {
		return fmt.Errorf("rebuild v2: membaca data lake: %w", err)
	}

	for _, ev := range events {
		// Satu transaction per event: kalau proses mati di tengah, event yang
		// sudah commit tidak diulang dan yang belum tidak setengah jadi.
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			return ApplyV2(ctx, tx, ev)
		})
		if err != nil {
			return fmt.Errorf("rebuild v2: %w", err)
		}
	}
	return nil
}

// ApplyLive dipanggil untuk setiap event baru yang datang dari broker.
// Selama migrasi, event diterapkan ke v1 dan v2 dalam satu transaction, jadi
// keduanya tidak pernah berbeda karena salah satu gagal.
func ApplyLive(ctx context.Context, pool *pgxpool.Pool, ev Envelope) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := ApplyV1(ctx, tx, ev); err != nil {
			return err
		}
		return ApplyV2(ctx, tx, ev)
	})
}

// Compare mengembalikan ketidakcocokan antara v1 dan v2 pada butir terhalus
// yang dimiliki keduanya: tipe kamar dan tanggal. Hasil kosong berarti
// keduanya sepakat.
func Compare(ctx context.Context, db DBTX) ([]Diff, error) {
	rows, err := db.Query(ctx, `
		SELECT coalesce(a.room_type, b.room_type),
		       coalesce(a.stay_date, b.stay_date),
		       coalesce(a.rooms_sold, 0),
		       coalesce(b.rooms_sold, 0)
		  FROM availability_v1 a
		  FULL OUTER JOIN (
		        SELECT room_type, stay_date, sum(rooms_sold)::int AS rooms_sold
		          FROM availability_v2
		         GROUP BY room_type, stay_date
		       ) b ON a.room_type = b.room_type AND a.stay_date = b.stay_date
		 WHERE coalesce(a.rooms_sold, 0) <> coalesce(b.rooms_sold, 0)
		 ORDER BY 1, 2`)
	if err != nil {
		return nil, fmt.Errorf("membandingkan v1 dan v2: %w", err)
	}
	diffs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Diff, error) {
		var d Diff
		err := row.Scan(&d.RoomType, &d.StayDate, &d.V1, &d.V2)
		return d, err
	})
	if err != nil {
		return nil, fmt.Errorf("membandingkan v1 dan v2: %w", err)
	}
	return diffs, nil
}

// SwitchToV2 memindahkan pembacaan aplikasi ke v2, hanya kalau Compare tidak
// menemukan ketidakcocokan.
func SwitchToV2(ctx context.Context, pool *pgxpool.Pool) error {
	diffs, err := Compare(ctx, pool)
	if err != nil {
		return err
	}
	if len(diffs) > 0 {
		return fmt.Errorf("v2 belum cocok dengan v1: %d perbedaan, contoh %+v", len(diffs), diffs[0])
	}
	if _, err := pool.Exec(ctx, `UPDATE read_model_version SET active = 2`); err != nil {
		return fmt.Errorf("pindah ke v2: %w", err)
	}
	return nil
}
