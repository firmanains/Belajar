package readmodel

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/obs"
	"belajar/eventdriven/proyek/internal/platform"
	"belajar/eventdriven/proyek/internal/store"
)

// Projector membangun rm_bookings dan rm_tape_chart dari event booking.
type Projector struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewProjector(pool *pgxpool.Pool, log *slog.Logger) *Projector {
	return &Projector{pool: pool, log: log}
}

// Handle menerapkan satu event ke read model (store.ReadModel).
//
// TODO(step 5): event yang sama bisa datang dua kali, event lama bisa datang
// setelah event baru, dan value-nya bisa rusak.
//  1. event.Decode. Kalau gagal, tulis log lalu return nil: message malformed
//     tidak akan pernah berhasil, dan return error membuat partition ini macet.
//  2. Dalam satu transaction: LockAndGetBooking, abaikan event yang revision-nya
//     tidak lebih besar dari state di read model, Release state lama, Occupy
//     state baru, SaveBooking.
//
// Logikanya sama dengan inbox di step 2. Bedanya, di sini tidak ada tabel
// inbox: nomor revision saja sudah cukup untuk mengenali duplicate.
func (p *Projector) Handle(ctx context.Context, r platform.Record) error {
	return nil
}

// Dipakai di step 5. Hapus baris ini setelah terpakai.
var (
	_ = event.Decode
	_ = obs.LoggerFrom
	_ = store.ReadModel
)
