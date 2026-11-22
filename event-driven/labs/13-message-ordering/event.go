package lab13

import "errors"

// File ini sudah jadi. Jangan diubah.

// Nama event yang dikenal Tracker.
const (
	BookingCreated   = "BookingCreated"
	BookingModified  = "BookingModified"
	BookingCancelled = "BookingCancelled"
)

// BookingEvent adalah satu revisi dari sebuah booking.
//
// Revision naik satu setiap kali booking berubah: BookingCreated selalu
// membawa Revision 1, perubahan berikutnya 2, lalu 3, dan seterusnya.
// Rooms berisi jumlah kamar setelah revisi itu (nilai mutlak, bukan selisih).
type BookingEvent struct {
	EventID   string `json:"event_id"`
	Name      string `json:"name"`
	BookingID string `json:"booking_id"`
	Revision  int    `json:"revision"`
	Rooms     int    `json:"rooms"`
}

// ErrGap berarti event datang terlalu cepat: ada revisi sebelum dia yang belum
// diterima. Handler yang mengembalikan error ini meminta broker mengirim ulang
// event tersebut nanti.
var ErrGap = errors.New("revision gap")

// Booking adalah keadaan booking menurut Tracker.
type Booking struct {
	Revision  int
	Rooms     int
	Cancelled bool
}
