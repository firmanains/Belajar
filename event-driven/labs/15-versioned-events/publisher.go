package lab15

import (
	"encoding/json"
	"fmt"
)

// File ini yang kamu ubah di Step 3.

// MessagesFor mengembalikan message yang diterbitkan PMS saat booking dibuat.
// Versi ini "meng-upgrade di tempat": nama lama, bentuk baru.
func MessagesFor(b Booking) ([]Message, error) {
	payload, err := json.Marshal(BookingCreatedV2{
		BookingID: b.ID,
		RoomType:  b.RoomType,
		Rooms:     b.Rooms,
		RatePlan:  b.RatePlan,
		Total:     b.Total,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal booking %s: %w", b.ID, err)
	}
	return []Message{{Name: NameBookingCreatedV1, Payload: payload}}, nil
}
