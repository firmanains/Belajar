package lab09

// File ini kamu ubah di Step 1 sampai Step 3.

// Emailer mengirim confirmation email. Bisa gagal, misalnya SMTP timeout.
type Emailer interface {
	Send(bookingID string) error
}

// Inventory menyimpan sisa kamar per room type sebagai angka.
type Inventory struct {
	available map[string]int
	email     Emailer
}

func NewInventory(available map[string]int, email Emailer) *Inventory {
	return &Inventory{available: available, email: email}
}

func (inv *Inventory) Available(roomType string) int { return inv.available[roomType] }

// Handle memproses satu BookingCreated: kurangi sisa kamar, lalu kirim
// confirmation email. Versi ini benar hanya kalau setiap message datang tepat sekali.
func (inv *Inventory) Handle(m Message) error {
	inv.available[m.Event.RoomType] -= m.Event.Rooms
	return inv.email.Send(m.Event.BookingID)
}
