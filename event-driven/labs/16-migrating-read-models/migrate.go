package lab16

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// File ini yang kamu ubah.

// DefaultRatePlan dipakai Rebuild versi awal untuk mengisi kolom yang tidak
// dimiliki v1.
const DefaultRatePlan = "standard"

// ApplyV2 menerapkan satu event ke read model v2. Panggil di dalam transaction.
func ApplyV2(ctx context.Context, db DBTX, ev Envelope) error {
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

// Rebuild mengisi read model v2. Versi ini mengambil jalan pintas: menyalin
// angka dari v1 dan mengarang rate plan-nya.
func Rebuild(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO availability_v2 (room_type, rate_plan, stay_date, rooms_sold)
		SELECT room_type, $1, stay_date, rooms_sold FROM availability_v1
		ON CONFLICT (room_type, rate_plan, stay_date)
		DO UPDATE SET rooms_sold = EXCLUDED.rooms_sold`, DefaultRatePlan)
	if err != nil {
		return fmt.Errorf("rebuild v2: %w", err)
	}
	return nil
}

// ApplyLive dipanggil untuk setiap event baru yang datang dari broker.
func ApplyLive(ctx context.Context, pool *pgxpool.Pool, ev Envelope) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		return ApplyV1(ctx, tx, ev)
	})
}

// Compare mengembalikan ketidakcocokan antara v1 dan v2. Hasil kosong berarti
// keduanya sepakat. Versi ini hanya membandingkan total keseluruhan.
func Compare(ctx context.Context, db DBTX) ([]Diff, error) {
	var v1, v2 int
	err := db.QueryRow(ctx, `
		SELECT (SELECT coalesce(sum(rooms_sold), 0) FROM availability_v1),
		       (SELECT coalesce(sum(rooms_sold), 0) FROM availability_v2)`).Scan(&v1, &v2)
	if err != nil {
		return nil, fmt.Errorf("membandingkan v1 dan v2: %w", err)
	}
	if v1 != v2 {
		return []Diff{{RoomType: "*", StayDate: "*", V1: v1, V2: v2}}, nil
	}
	return nil, nil
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
