package lab13

import "fmt"

// File ini yang kamu ubah di Step 2 sampai Step 4.

// Tracker menyimpan keadaan terakhir setiap booking berdasarkan event yang
// diterimanya.
type Tracker struct {
	bookings map[string]Booking
}

func NewTracker() *Tracker {
	return &Tracker{bookings: map[string]Booking{}}
}

// Booking mengembalikan keadaan booking dan apakah booking itu dikenal.
func (t *Tracker) Booking(id string) (Booking, bool) {
	b, ok := t.bookings[id]
	return b, ok
}

// Handle menerapkan satu event. Return nil berarti event selesai diurus (ack).
// Versi ini benar hanya kalau event setiap booking datang berurutan dan
// tepat sekali.
func (t *Tracker) Handle(ev BookingEvent) error {
	switch ev.Name {
	case BookingCreated, BookingModified:
		t.bookings[ev.BookingID] = Booking{Revision: ev.Revision, Rooms: ev.Rooms}
	case BookingCancelled:
		b, ok := t.bookings[ev.BookingID]
		if !ok {
			return nil // booking tidak dikenal: tidak ada yang bisa dibatalkan
		}
		b.Revision = ev.Revision
		b.Rooms = 0
		b.Cancelled = true
		t.bookings[ev.BookingID] = b
	default:
		return fmt.Errorf("event tidak dikenal: %q", ev.Name)
	}
	return nil
}
