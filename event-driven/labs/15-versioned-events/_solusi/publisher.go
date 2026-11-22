package lab15

import (
	"encoding/json"
	"fmt"
)

// MessagesFor mengembalikan message yang diterbitkan PMS saat booking dibuat.
// Selama masa transisi, v2 terbit berdampingan dengan v1 supaya consumer lama
// tetap berjalan sampai mereka pindah.
func MessagesFor(b Booking) ([]Message, error) {
	v2, err := json.Marshal(BookingCreatedV2{
		BookingID: b.ID,
		RoomType:  b.RoomType,
		Rooms:     b.Rooms,
		RatePlan:  b.RatePlan,
		Total:     b.Total,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal v2 booking %s: %w", b.ID, err)
	}
	msgs := []Message{{Name: NameBookingCreatedV2, Payload: v2}}

	// v1 tidak punya tempat untuk mata uang. Booking non-IDR tidak bisa
	// dinyatakan di v1 tanpa berbohong, jadi tidak diterbitkan sebagai v1.
	if b.Total.Currency != "IDR" {
		return msgs, nil
	}

	v1, err := json.Marshal(BookingCreatedV1{
		BookingID: b.ID,
		RoomType:  b.RoomType,
		Rooms:     b.Rooms,
		Total:     b.Total.Amount,
		RatePlan:  b.RatePlan,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal v1 booking %s: %w", b.ID, err)
	}
	return append(msgs, Message{Name: NameBookingCreatedV1, Payload: v1}), nil
}
