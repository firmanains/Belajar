package lab09

// File ini kamu ubah di Step 4.

// Ledger menyimpan booking itu sendiri, bukan angka sisa kamar. Sisa kamar
// dihitung dari capacity dikurangi semua booking yang tersimpan.
type Ledger struct {
	capacity map[string]int
	bookings []BookingCreated
}

func NewLedger(capacity map[string]int) *Ledger {
	return &Ledger{capacity: capacity}
}

// Handle menyimpan booking dari message. Versi ini menambah baris baru setiap
// kali dipanggil, jadi booking yang sama bisa tersimpan dua kali.
func (l *Ledger) Handle(m Message) error {
	l.bookings = append(l.bookings, m.Event)
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
