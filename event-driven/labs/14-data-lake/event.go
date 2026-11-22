package lab14

import (
	"encoding/json"
	"time"
)

// File ini sudah jadi. Jangan diubah.

// Envelope adalah event apa adanya, seperti yang diterima dari broker.
// Payload sengaja dibiarkan mentah: data lake tidak perlu tahu isinya.
type Envelope struct {
	ID         string
	Name       string
	OccurredAt time.Time
	Payload    json.RawMessage
}

// Dua bentuk payload yang dikenal service ini saat data lake pertama dibuat.
type BookingCreated struct {
	BookingID string `json:"booking_id"`
	RoomType  string `json:"room_type"`
	Rooms     int    `json:"rooms"`
}

type BookingCancelled struct {
	BookingID string `json:"booking_id"`
}
