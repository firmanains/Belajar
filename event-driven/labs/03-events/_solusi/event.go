package lab03

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrUnknownEvent berarti nama event tidak dikenal consumer ini. Consumer
	// boleh melewatinya; itu bukan kerusakan.
	ErrUnknownEvent = errors.New("unknown event")
	// ErrInvalidEvent berarti event dikenal, tapi isinya tidak bisa dipakai.
	ErrInvalidEvent = errors.New("invalid event")
)

// Envelope adalah amplop yang membungkus setiap event.
type Envelope struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}

// BookingCreated adalah fakta bahwa sebuah booking sudah dibuat. Isinya hanya
// yang dibutuhkan consumer untuk bekerja tanpa bertanya balik ke PMS.
type BookingCreated struct {
	BookingID string `json:"booking_id"`
	RoomType  string `json:"room_type"`
	// Tanggal menginap adalah tanggal kalender, bukan titik waktu. Format
	// YYYY-MM-DD menghindari pergeseran hari akibat timezone.
	CheckIn    string `json:"check_in"`
	CheckOut   string `json:"check_out"`
	Rooms      int    `json:"rooms"`
	GuestEmail string `json:"guest_email"`
}

// NewBookingCreated membuat event dari model internal. Nomor kartu dan ID
// internal sengaja tidak ikut.
func NewBookingCreated(b Booking) BookingCreated {
	return BookingCreated{
		BookingID:  b.ID,
		RoomType:   b.RoomType,
		CheckIn:    b.CheckIn.Format(time.DateOnly),
		CheckOut:   b.CheckOut.Format(time.DateOnly),
		Rooms:      b.Rooms,
		GuestEmail: b.GuestEmail,
	}
}

// NewEnvelope membungkus payload dengan metadata yang dibutuhkan semua
// consumer, apa pun isi event-nya.
func NewEnvelope(name, correlationID string, payload any) (Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal payload of %s: %w", name, err)
	}
	id, err := newID()
	if err != nil {
		return Envelope{}, fmt.Errorf("generate event id: %w", err)
	}
	return Envelope{
		ID:            id,
		Name:          name,
		OccurredAt:    time.Now().UTC(),
		CorrelationID: correlationID,
		Payload:       data,
	}, nil
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// DecodePayload membaca payload sesuai nama event. Field yang tidak dikenal
// diabaikan; field yang wajib ada diperiksa.
func DecodePayload(env Envelope) (any, error) {
	switch env.Name {
	case BookingCreatedName:
		var e BookingCreated
		if err := json.Unmarshal(env.Payload, &e); err != nil {
			return nil, fmt.Errorf("%w: decode %s: %v", ErrInvalidEvent, env.Name, err)
		}
		if e.BookingID == "" || e.RoomType == "" || e.Rooms <= 0 {
			return nil, fmt.Errorf("%w: %s needs booking_id, room_type, and rooms > 0", ErrInvalidEvent, env.Name)
		}
		return e, nil
	case BookingCancelledName:
		var e BookingCancelled
		if err := json.Unmarshal(env.Payload, &e); err != nil {
			return nil, fmt.Errorf("%w: decode %s: %v", ErrInvalidEvent, env.Name, err)
		}
		if e.BookingID == "" {
			return nil, fmt.Errorf("%w: %s needs booking_id", ErrInvalidEvent, env.Name)
		}
		return e, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownEvent, env.Name)
	}
}
