package lab12

// File ini sudah jadi. Jangan diubah; baca saja.

// Envelope membungkus satu event dengan ID uniknya.
type Envelope struct {
	EventID string
	Event   any // BookingCreated atau BookingCancelled
}

// BookingCreated: sebuah booking sudah dibuat. Tanggal berformat YYYY-MM-DD.
// CheckOut tidak dihitung sebagai malam menginap: 1 Des sampai 3 Des berarti
// malam 1 dan malam 2.
type BookingCreated struct {
	BookingID string
	GuestName string
	RoomType  string
	CheckIn   string
	CheckOut  string
	Rooms     int
}

// BookingCancelled: sebuah booking sudah dibatalkan.
type BookingCancelled struct {
	BookingID string
}

const (
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
)
