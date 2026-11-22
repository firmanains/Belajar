// Package platform berisi sambungan ke Postgres dan Kafka.
//
// File ini sudah jadi. Jangan diubah.
package platform

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX dipenuhi oleh *pgxpool.Pool dan pgx.Tx, sehingga helper SQL yang sama
// bisa dipakai di dalam maupun di luar transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

var schemaName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS room_types (
		room_type text PRIMARY KEY,
		total     int  NOT NULL)`,
	`INSERT INTO room_types VALUES ('deluxe', 10), ('suite', 4) ON CONFLICT DO NOTHING`,

	// Write model: sumber kebenaran.
	`CREATE TABLE IF NOT EXISTS inbox (
		revision_id text PRIMARY KEY,
		received_at timestamptz NOT NULL DEFAULT now())`,
	`CREATE TABLE IF NOT EXISTS bookings (
		booking_id  text PRIMARY KEY,
		revision    int  NOT NULL,
		status      text NOT NULL,
		room_type   text NOT NULL REFERENCES room_types,
		check_in    date NOT NULL,
		check_out   date NOT NULL,
		rooms       int  NOT NULL,
		guest_email text NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS inventory (
		room_type text NOT NULL,
		day       date NOT NULL,
		booked    int  NOT NULL,
		PRIMARY KEY (room_type, day))`,
	`CREATE TABLE IF NOT EXISTS outbox (
		id             bigserial PRIMARY KEY,
		event_id       text NOT NULL UNIQUE,
		event_name     text NOT NULL,
		booking_id     text NOT NULL,
		correlation_id text NOT NULL,
		payload        jsonb NOT NULL,
		created_at     timestamptz NOT NULL DEFAULT now(),
		sent_at        timestamptz)`,
	`CREATE INDEX IF NOT EXISTS outbox_pending ON outbox (id) WHERE sent_at IS NULL`,

	// Read model: dibangun dari event, boleh dihapus dan dibangun ulang.
	`CREATE TABLE IF NOT EXISTS rm_bookings (LIKE bookings INCLUDING ALL)`,
	`CREATE TABLE IF NOT EXISTS rm_tape_chart (LIKE inventory INCLUDING ALL)`,

	`CREATE TABLE IF NOT EXISTS email_log (
		event_id text PRIMARY KEY,
		sent_at  timestamptz NOT NULL DEFAULT now())`,
}

// Connect membuka pool ke Postgres, membuat schema kalau belum ada, lalu
// menjalankan migration. Semua query pool ini berjalan di dalam schema itu.
func Connect(ctx context.Context, url, schema string) (*pgxpool.Pool, error) {
	if !schemaName.MatchString(schema) {
		return nil, fmt.Errorf("nama schema %q tidak valid", schema)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse postgres url: %w", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	for _, stmt := range migrations {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			pool.Close()
			return nil, fmt.Errorf("migration: %w", err)
		}
	}
	return pool, nil
}
