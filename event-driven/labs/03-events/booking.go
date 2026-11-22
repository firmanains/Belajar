package lab03

// File ini sudah jadi. Jangan diubah.

import "time"

// Booking adalah model internal PMS: isi tabel bookings apa adanya. Ia berisi
// hal yang tidak boleh keluar dari service ini.
type Booking struct {
	ID         string
	RoomType   string
	CheckIn    time.Time
	CheckOut   time.Time
	Rooms      int
	GuestEmail string
	CardNumber string
	InternalID int64
}

// Nama event adalah bagian dari kontrak. Consumer memakainya untuk memutuskan
// cara membaca payload.
const (
	BookingCreatedName   = "BookingCreated"
	BookingCancelledName = "BookingCancelled"
)

// BookingCancelled adalah contoh event yang sudah dirancang dengan benar:
// nama field di JSON ditentukan oleh tag, bukan oleh nama field Go.
type BookingCancelled struct {
	BookingID string `json:"booking_id"`
	Reason    string `json:"reason"`
}
