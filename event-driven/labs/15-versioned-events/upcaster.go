package lab15

import (
	"encoding/json"
	"fmt"
)

// File ini yang kamu ubah di Step 4.

// Upcast mengubah message BookingCreated versi apa pun menjadi bentuk
// terbaru, supaya handler di belakangnya hanya perlu mengenal satu bentuk.
func Upcast(m Message) (BookingCreatedV2, error) {
	if m.Name != NameBookingCreatedV2 {
		return BookingCreatedV2{}, fmt.Errorf("event %q tidak didukung", m.Name)
	}
	var ev BookingCreatedV2
	if err := json.Unmarshal(m.Payload, &ev); err != nil {
		return BookingCreatedV2{}, fmt.Errorf("decode %s: %w", m.Name, err)
	}
	return ev, nil
}
