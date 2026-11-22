package lab19

import (
	"context"
	"errors"
)

// File ini sudah jadi. Jangan diubah.

// Email adalah confirmation email untuk satu booking.
type Email struct {
	BookingID string
	To        string
}

// Message adalah satu message di stream confirmation email.
type Message struct {
	ID    string
	Email Email
}

// Sender adalah dependency keluar: server SMTP. Ia bisa lambat, bisa mati.
type Sender interface {
	Send(ctx context.Context, email Email) error
}

// ErrPermanent menandai kegagalan yang tidak akan sembuh dengan mencoba
// lagi, misalnya alamat email yang tidak valid. Bungkus dengan
// fmt.Errorf("...: %w", ErrPermanent).
var ErrPermanent = errors.New("permanent failure")

// ErrCircuitOpen dikembalikan Breaker ketika ia menolak panggilan tanpa
// meneruskannya ke dependency.
var ErrCircuitOpen = errors.New("circuit open")

// ErrNotInDeadLetterQueue dikembalikan ketika message yang diminta tidak ada
// di dead letter queue.
var ErrNotInDeadLetterQueue = errors.New("message not in dead letter queue")

// DeadLetter adalah message yang sudah menyerah, beserta alasannya.
type DeadLetter struct {
	Message  Message
	Reason   string
	Attempts int
}

// DeadLetterQueue menampung message yang dipinggirkan. Di Kafka ini topic
// tersendiri; di sini cukup slice.
type DeadLetterQueue struct {
	entries []DeadLetter
}

// Add menaruh satu message ke dead letter queue.
func (q *DeadLetterQueue) Add(entry DeadLetter) {
	q.entries = append(q.entries, entry)
}

// Entries mengembalikan salinan isi dead letter queue.
func (q *DeadLetterQueue) Entries() []DeadLetter {
	return append([]DeadLetter(nil), q.entries...)
}
