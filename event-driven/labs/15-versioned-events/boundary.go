package lab15

import (
	"encoding/json"
	"fmt"
)

// File ini yang kamu ubah di Step 5.

// ToPublic mengubah event internal menjadi message yang diterbitkan ke luar
// service. Versi ini mengirim struct internal apa adanya.
func ToPublic(e InternalBookingConfirmed) (Message, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return Message{}, fmt.Errorf("marshal booking %s: %w", e.BookingID, err)
	}
	return Message{Name: NameBookingCreatedV2, Payload: payload}, nil
}
