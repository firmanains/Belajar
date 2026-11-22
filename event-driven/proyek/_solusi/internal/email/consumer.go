package email

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
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

// Handle memproses satu record. Tiga jenis kegagalan diperlakukan berbeda:
// malformed langsung ke dead letter topic, kegagalan kirim dicoba ulang dengan
// backoff lalu ke dead letter topic, dan shutdown mengembalikan error supaya
// record dikirim ulang oleh Kafka nanti.
func (c *Consumer) Handle(ctx context.Context, r platform.Record) error {
	log := obs.LoggerFrom(ctx, c.log)

	env, err := event.Decode(r.Value)
	if err != nil {
		return c.deadLetter(ctx, r, "malformed: "+err.Error())
	}
	if env.EventName != event.BookingCreated {
		return nil
	}

	// Event yang sama bisa datang dua kali (forwarder at-least-once).
	sent, err := alreadySent(ctx, c.pool, env.EventID)
	if err != nil {
		return err
	}
	if sent {
		log.Info("email sudah pernah terkirim, dilewati", "booking_id", env.Booking.BookingID)
		return nil
	}

	var sendErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		sendErr = c.sender.SendConfirmation(ctx, env.Booking)
		if sendErr == nil {
			// Kalau proses mati di antara kirim dan catat, email ini terkirim
			// dua kali. Email tidak bisa ikut transaction database.
			return markSent(ctx, c.pool, env.EventID)
		}
		if ctx.Err() != nil {
			// Service sedang berhenti. Ini bukan salah message-nya, jadi jangan
			// dibuang ke dead letter topic.
			return fmt.Errorf("kirim email dibatalkan: %w", ctx.Err())
		}
		log.Warn("kirim email gagal", "booking_id", env.Booking.BookingID, "attempt", attempt, "error", sendErr)
		if attempt == c.maxAttempts {
			break
		}
		select {
		case <-time.After(c.backoff << (attempt - 1)): // 1x, 2x, 4x, ...
		case <-ctx.Done():
			return fmt.Errorf("kirim email dibatalkan: %w", ctx.Err())
		}
	}
	return c.deadLetter(ctx, r, fmt.Sprintf("gagal setelah %d percobaan: %v", c.maxAttempts, sendErr))
}

// deadLetter memindahkan record ke dead letter topic beserta alasannya, supaya
// record berikutnya tetap mengalir. Kalau pemindahan gagal, error dikembalikan
// dan record ini belum dianggap selesai.
func (c *Consumer) deadLetter(ctx context.Context, r platform.Record, reason string) error {
	headers := map[string]string{
		event.HeaderDLQReason: reason,
		event.HeaderDLQSource: r.Topic + "/" + strconv.Itoa(int(r.Partition)) + "/" + strconv.FormatInt(r.Offset, 10),
	}
	for k, v := range r.Headers {
		headers[k] = v
	}
	if err := c.pub.Publish(ctx, c.dlqTopic, platform.Message{Key: r.Key, Value: r.Value, Headers: headers}); err != nil {
		return fmt.Errorf("publish ke dead letter topic: %w", err)
	}
	obs.LoggerFrom(ctx, c.log).Error("message dipindahkan ke dead letter topic", "reason", reason)
	return nil
}
