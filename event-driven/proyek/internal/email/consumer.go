package email

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/obs"
	"belajar/eventdriven/proyek/internal/platform"
)

// Consumer mengirim confirmation email untuk setiap BookingCreated.
type Consumer struct {
	pool        *pgxpool.Pool
	sender      Sender
	pub         platform.Publisher
	dlqTopic    string
	maxAttempts int
	backoff     time.Duration
	log         *slog.Logger
}

func NewConsumer(pool *pgxpool.Pool, sender Sender, pub platform.Publisher, dlqTopic string, maxAttempts int, backoff time.Duration, log *slog.Logger) *Consumer {
	return &Consumer{pool: pool, sender: sender, pub: pub, dlqTopic: dlqTopic, maxAttempts: maxAttempts, backoff: backoff, log: log}
}

// Handle mengirim confirmation email untuk event BookingCreated.
//
// TODO(step 6):
//  1. event.Decode. Message malformed langsung ke dead letter topic, tanpa retry.
//  2. Hanya BookingCreated yang mengirim email. Event lain: return nil.
//  3. Lewati event yang sudah pernah dikirimi email (alreadySent), dan catat
//     dengan markSent setelah berhasil.
//  4. Coba kirim paling banyak c.maxAttempts kali. Jeda sebelum percobaan
//     berikutnya berlipat dua: c.backoff, 2x, 4x. Jeda harus berhenti kalau ctx
//     dibatalkan.
//  5. Kalau semua percobaan gagal, pindahkan record ke c.dlqTopic lewat c.pub
//     dengan key, value, dan header asli, ditambah event.HeaderDLQReason dan
//     event.HeaderDLQSource. Lalu return nil supaya record berikutnya mengalir.
//  6. Kalau ctx dibatalkan (service berhenti), return error dan jangan pindahkan
//     ke dead letter topic. Ini diuji di step 8.
func (c *Consumer) Handle(ctx context.Context, r platform.Record) error {
	return nil
}

// Dipakai di step 6. Hapus baris ini setelah terpakai.
var (
	_ = event.Decode
	_ = obs.LoggerFrom
	_ = alreadySent
	_ = markSent
)
