package lab03

// File ini yang kamu ubah.

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrUnknownEvent berarti nama event tidak dikenal consumer ini. Consumer
	// boleh melewatinya; itu bukan kerusakan.
	ErrUnknownEvent = errors.New("unknown event")
	// ErrInvalidEvent berarti event dikenal, tapi isinya tidak bisa dipakai.
	ErrInvalidEvent = errors.New("invalid event")
)

// Envelope adalah amplop yang membungkus setiap event. Field-nya sudah
// lengkap; jangan diubah.
type Envelope struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}

// BookingCreated adalah fakta bahwa sebuah booking sudah dibuat.
//
// Versi ini menyalin model internal apa adanya. Kamu boleh mengubah tag,
// menghapus field, dan mengganti tipe CheckIn dan CheckOut. Nama dan tipe
// field BookingID, RoomType, dan Rooms jangan diubah, karena test memakainya.
type BookingCreated struct {
	BookingID  string
	RoomType   string
	CheckIn    time.Time
	CheckOut   time.Time
	Rooms      int
	GuestEmail string
	CardNumber string
	InternalID int64
}

// NewBookingCreated membuat event dari model internal.
func NewBookingCreated(b Booking) BookingCreated {
	return BookingCreated{
		BookingID:  b.ID,
		RoomType:   b.RoomType,
		CheckIn:    b.CheckIn,
		CheckOut:   b.CheckOut,
		Rooms:      b.Rooms,
		GuestEmail: b.GuestEmail,
		CardNumber: b.CardNumber,
		InternalID: b.InternalID,
	}
}

// NewEnvelope membungkus payload. Versi ini baru mengisi payload-nya saja.
func NewEnvelope(name, correlationID string, payload any) (Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Payload: data}, nil
}

// DecodePayload membaca payload sesuai nama event, lalu mengembalikan
// BookingCreated atau BookingCancelled (sebagai value, bukan pointer).
func DecodePayload(env Envelope) (any, error) {
	return nil, ErrUnknownEvent
}
