package lab08

// File ini sudah jadi. Jangan diubah; baca saja.

import "context"

// BookingCreated terbit ketika sebuah booking tersimpan di PMS.
type BookingCreated struct {
	BookingID  string `json:"booking_id"`
	RoomType   string `json:"room_type"`
	Rooms      int    `json:"rooms"`
	GuestEmail string `json:"guest_email"`
}

// BookingCancelled terbit ketika booking dibatalkan.
type BookingCancelled struct {
	BookingID string `json:"booking_id"`
	RoomType  string `json:"room_type"`
	Rooms     int    `json:"rooms"`
}

// Availability menyimpan sisa kamar per room type.
type Availability interface {
	Decrease(ctx context.Context, roomType string, rooms int) error
	Increase(ctx context.Context, roomType string, rooms int) error
}

// Emailer mengirim email konfirmasi ke tamu.
type Emailer interface {
	SendConfirmation(ctx context.Context, bookingID, to string) error
}

// ChannelManager menerima kabar bahwa availability sebuah room type berubah.
type ChannelManager interface {
	PushAvailability(ctx context.Context, roomType string) error
}

// Deps adalah semua dependency keluar yang dipakai event handler.
type Deps struct {
	Availability   Availability
	Emailer        Emailer
	ChannelManager ChannelManager
}
