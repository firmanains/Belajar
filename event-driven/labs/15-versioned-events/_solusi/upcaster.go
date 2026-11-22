package lab15

import (
	"encoding/json"
	"fmt"
)

// Upcast mengubah message BookingCreated versi apa pun menjadi bentuk
// terbaru, supaya handler di belakangnya hanya perlu mengenal satu bentuk.
func Upcast(m Message) (BookingCreatedV2, error) {
	switch m.Name {
	case NameBookingCreatedV2:
		var ev BookingCreatedV2
		if err := json.Unmarshal(m.Payload, &ev); err != nil {
			return BookingCreatedV2{}, fmt.Errorf("decode %s: %w", m.Name, err)
		}
		return ev, nil

	case NameBookingCreatedV1:
		old, err := DecodeV1(m.Payload)
		if err != nil {
			return BookingCreatedV2{}, err
		}
		return BookingCreatedV2{
			BookingID: old.BookingID,
			RoomType:  old.RoomType,
			Rooms:     old.Rooms,
			RatePlan:  old.RatePlan, // DecodeV1 sudah mengisi default
			// Pengetahuan yang dulu tersirat di v1 ditulis di sini, satu kali.
			Total: Money{Amount: old.Total, Currency: "IDR"},
		}, nil

	default:
		return BookingCreatedV2{}, fmt.Errorf("event %q tidak didukung", m.Name)
	}
}
