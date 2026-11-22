// Package event mendefinisikan kontrak event yang keluar dari service ini.
//
// File ini sudah jadi. Jangan diubah.
package event

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Nama event: fakta yang sudah terjadi, bentuk lampau.
const (
	BookingCreated   = "BookingCreated"
	BookingModified  = "BookingModified"
	BookingCancelled = "BookingCancelled"
)

const (
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
)

// Header Kafka. Consumer bisa membaca metadata tanpa membongkar payload.
const (
	HeaderEventID       = "event-id"
	HeaderEventName     = "event-name"
	HeaderCorrelationID = "correlation-id"

	// Hanya ada di dead letter topic.
	HeaderDLQReason = "dlq-reason"
	HeaderDLQSource = "dlq-source"
)

// Booking adalah state lengkap sebuah booking pada satu revision. Event membawa
// state lengkap, bukan selisih, supaya consumer tetap benar walau ada event
// yang datang terlambat atau terlewat.
type Booking struct {
	BookingID  string `json:"booking_id"`
	Revision   int    `json:"revision"`
	Status     string `json:"status"`
	RoomType   string `json:"room_type"`
	CheckIn    string `json:"check_in"`  // YYYY-MM-DD, malam pertama
	CheckOut   string `json:"check_out"` // YYYY-MM-DD, tidak ikut dihitung
	Rooms      int    `json:"rooms"`
	GuestEmail string `json:"guest_email"`
}

// Envelope membungkus payload dengan metadata yang sama untuk semua event.
type Envelope struct {
	EventID       string    `json:"event_id"`
	EventName     string    `json:"event_name"`
	OccurredAt    time.Time `json:"occurred_at"`
	CorrelationID string    `json:"correlation_id"`
	Booking       Booking   `json:"booking"`
}

// NewID menghasilkan ID acak 128 bit dalam bentuk hex.
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read tidak pernah gagal sejak Go 1.24
	return hex.EncodeToString(b)
}

// New membuat envelope baru dengan event ID baru.
func New(name, correlationID string, b Booking) Envelope {
	return Envelope{
		EventID:       NewID(),
		EventName:     name,
		OccurredAt:    time.Now().UTC(),
		CorrelationID: correlationID,
		Booking:       b,
	}
}

// Headers mengembalikan metadata envelope sebagai header Kafka.
func (e Envelope) Headers() map[string]string {
	return map[string]string{
		HeaderEventID:       e.EventID,
		HeaderEventName:     e.EventName,
		HeaderCorrelationID: e.CorrelationID,
	}
}

// Decode membaca envelope dari value sebuah record. Error dari fungsi ini
// berarti message malformed: mencobanya lagi tidak akan pernah berhasil.
func Decode(value []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(value, &e); err != nil {
		return e, fmt.Errorf("decode envelope: %w", err)
	}
	if e.EventID == "" || e.EventName == "" || e.Booking.BookingID == "" || e.Booking.Revision < 1 {
		return e, errors.New("envelope tidak lengkap: event_id, event_name, booking_id, dan revision wajib ada")
	}
	return e, nil
}
