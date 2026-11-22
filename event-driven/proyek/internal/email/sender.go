// Package email mengirim confirmation email untuk booking baru.
package email

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"fmt"
	"log/slog"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/platform"
)

// Sender mengirim satu confirmation email. Error berarti email belum terkirim.
type Sender interface {
	SendConfirmation(ctx context.Context, b event.Booking) error
}

// LogSender hanya menulis ke log. Dipakai saat service dijalankan dengan tangan.
type LogSender struct{ Log *slog.Logger }

func (s LogSender) SendConfirmation(_ context.Context, b event.Booking) error {
	s.Log.Info("confirmation email terkirim (pura-pura)", "booking_id", b.BookingID, "to", b.GuestEmail)
	return nil
}

// alreadySent memeriksa apakah email untuk event ini sudah pernah terkirim.
func alreadySent(ctx context.Context, q platform.DBTX, eventID string) (bool, error) {
	var ok bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM email_log WHERE event_id = $1)`, eventID).Scan(&ok); err != nil {
		return false, fmt.Errorf("cek email_log: %w", err)
	}
	return ok, nil
}

// markSent mencatat bahwa email untuk event ini sudah terkirim.
func markSent(ctx context.Context, q platform.DBTX, eventID string) error {
	if _, err := q.Exec(ctx, `INSERT INTO email_log (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, eventID); err != nil {
		return fmt.Errorf("insert email_log: %w", err)
	}
	return nil
}
