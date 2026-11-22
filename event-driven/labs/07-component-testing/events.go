package lab07

// File ini sudah jadi. Jangan diubah; baca saja.

import "context"

const (
	TopicBookingCreated   = "booking.created"
	TopicBookingCancelled = "booking.cancelled"
)

// BookingCreated terbit ketika sebuah booking tersimpan di PMS.
type BookingCreated struct {
	BookingID  string `json:"booking_id"`
	RoomType   string `json:"room_type"`
	Date       string `json:"date"`
	Rooms      int    `json:"rooms"`
	GuestEmail string `json:"guest_email"`
}

// BookingCancelled terbit ketika booking dibatalkan.
type BookingCancelled struct {
	BookingID string `json:"booking_id"`
	RoomType  string `json:"room_type"`
	Date      string `json:"date"`
	Rooms     int    `json:"rooms"`
}

// AvailabilityUpdate adalah body `POST /availability` ke channel manager.
// Delta negatif berarti kamar berkurang.
type AvailabilityUpdate struct {
	BookingID string `json:"booking_id"`
	RoomType  string `json:"room_type"`
	Date      string `json:"date"`
	Delta     int    `json:"delta"`
}

// Emailer mengirim email konfirmasi ke tamu.
type Emailer interface {
	SendConfirmation(ctx context.Context, bookingID, to string) error
}
