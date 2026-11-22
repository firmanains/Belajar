package lab09

// Emailer mengirim confirmation email. Bisa gagal, misalnya SMTP timeout.
type Emailer interface {
	Send(bookingID string) error
}

// Inventory menyimpan sisa kamar per room type sebagai angka.
type Inventory struct {
	available map[string]int
	email     Emailer
	processed map[string]bool // idempotency key: BookingID
}

func NewInventory(available map[string]int, email Emailer) *Inventory {
	return &Inventory{available: available, email: email, processed: map[string]bool{}}
}

func (inv *Inventory) Available(roomType string) int { return inv.available[roomType] }

// Handle idempotent untuk pengurangan kamar: tanda "sudah diproses" dipasang
// bersamaan dengan pengurangan, dengan business key sebagai kuncinya.
// Email tidak ikut dijaga, jadi redelivery tetap mengirimnya.
func (inv *Inventory) Handle(m Message) error {
	if !inv.processed[m.Event.BookingID] {
		inv.available[m.Event.RoomType] -= m.Event.Rooms
		inv.processed[m.Event.BookingID] = true
	}
	return inv.email.Send(m.Event.BookingID)
}
