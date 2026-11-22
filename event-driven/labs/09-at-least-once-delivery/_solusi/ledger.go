package lab09

// Ledger menyimpan booking itu sendiri, bukan angka sisa kamar. Sisa kamar
// dihitung dari capacity dikurangi semua booking yang tersimpan.
type Ledger struct {
	capacity map[string]int
	bookings map[string]BookingCreated // kunci: BookingID
}

func NewLedger(capacity map[string]int) *Ledger {
	return &Ledger{capacity: capacity, bookings: map[string]BookingCreated{}}
}

// Handle melakukan upsert: booking dengan ID yang sama menimpa isi lama.
// Menyimpan dua kali menghasilkan keadaan yang sama dengan menyimpan sekali.
func (l *Ledger) Handle(m Message) error {
	l.bookings[m.Event.BookingID] = m.Event
	return nil
}

// Count mengembalikan jumlah booking yang tersimpan.
func (l *Ledger) Count() int { return len(l.bookings) }

// Available menghitung sisa kamar dari booking yang tersimpan.
func (l *Ledger) Available(roomType string) int {
	left := l.capacity[roomType]
	for _, b := range l.bookings {
		if b.RoomType == roomType {
			left -= b.Rooms
		}
	}
	return left
}
