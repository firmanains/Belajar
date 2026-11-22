package lab15

import (
	"encoding/json"
	"fmt"
)

// ToPublic mengubah event internal menjadi message yang diterbitkan ke luar
// service. Setiap field publik dipetakan satu per satu: field internal baru
// tidak ikut keluar kecuali seseorang sengaja menambahkannya di sini.
func ToPublic(e InternalBookingConfirmed) (Message, error) {
	payload, err := json.Marshal(BookingCreatedV2{
		BookingID: e.BookingID,
		RoomType:  e.RoomType,
		Rooms:     e.Rooms,
		RatePlan:  e.RatePlan,
		Total:     Money{Amount: e.SellAmount, Currency: e.Currency},
	})
	if err != nil {
		return Message{}, fmt.Errorf("marshal booking %s: %w", e.BookingID, err)
	}
	return Message{Name: NameBookingCreatedV2, Payload: payload}, nil
}
