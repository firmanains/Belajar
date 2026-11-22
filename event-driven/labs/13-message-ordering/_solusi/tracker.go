package lab13

import "fmt"

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

// Handle menerapkan satu event hanya kalau Revision-nya tepat satu di atas
// revisi yang sudah diterapkan. Booking yang belum dikenal punya revisi 0.
func (t *Tracker) Handle(ev BookingEvent) error {
	current := t.bookings[ev.BookingID].Revision

	if ev.Revision <= current {
		return nil // stale atau duplikat: sudah tercakup, cukup ack
	}
	if ev.Revision > current+1 {
		// Keadaan tidak disentuh. Broker akan mengirim ulang event ini.
		return fmt.Errorf("booking %s: punya revision %d, menerima %d: %w",
			ev.BookingID, current, ev.Revision, ErrGap)
	}

	switch ev.Name {
	case BookingCreated, BookingModified:
		t.bookings[ev.BookingID] = Booking{Revision: ev.Revision, Rooms: ev.Rooms}
	case BookingCancelled:
		t.bookings[ev.BookingID] = Booking{Revision: ev.Revision, Cancelled: true}
	default:
		return fmt.Errorf("event tidak dikenal: %q", ev.Name)
	}
	return nil
}
