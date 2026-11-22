package lab09

// File ini sudah jadi. Jangan diubah.

import (
	"errors"
	"testing"
)

type fakeEmail struct {
	failFirst bool
	calls     int
	sent      map[string]int
}

func (f *fakeEmail) Send(bookingID string) error {
	f.calls++
	if f.failFirst && f.calls == 1 {
		return errors.New("SMTP timeout")
	}
	f.sent[bookingID]++
	return nil
}

func setup(failFirstEmail bool) (*Inventory, *fakeEmail) {
	email := &fakeEmail{failFirst: failFirstEmail, sent: map[string]int{}}
	return NewInventory(map[string]int{"deluxe": 10}, email), email
}

func booking(msgID, bookingID string, rooms int) Message {
	return Message{ID: msgID, Event: BookingCreated{BookingID: bookingID, RoomType: "deluxe", Rooms: rooms}}
}

// Dunia ideal: setiap message datang tepat sekali. Lulus sejak kode awal.
func TestStep0_NoDuplicates(t *testing.T) {
	inv, _ := setup(false)
	b := &Broker{}
	b.Publish(booking("m-1", "B-1", 2))
	if err := b.Run(inv.Handle); err != nil {
		t.Fatal(err)
	}
	if got := inv.Available("deluxe"); got != 8 {
		t.Fatalf("sisa kamar = %d, seharusnya 8", got)
	}
}

// Handler berhasil, tapi ack hilang, jadi broker mengirim message yang sama lagi.
func TestStep1_LostAck(t *testing.T) {
	inv, _ := setup(false)
	b := &Broker{LoseFirstAck: true}
	b.Publish(booking("m-1", "B-1", 2))
	if err := b.Run(inv.Handle); err != nil {
		t.Fatal(err)
	}
	if got := inv.Available("deluxe"); got != 8 {
		t.Fatalf("sisa kamar = %d, seharusnya 8: satu booking terhitung dua kali", got)
	}
}

// Kamar sudah dikurangi, lalu email gagal. Handler return error dan terjadi redelivery.
func TestStep2_FailureAfterRoomsDecremented(t *testing.T) {
	inv, email := setup(true)
	b := &Broker{}
	b.Publish(booking("m-1", "B-1", 2))
	if err := b.Run(inv.Handle); err != nil {
		t.Fatal(err)
	}
	if got := inv.Available("deluxe"); got != 8 {
		t.Fatalf("sisa kamar = %d, seharusnya 8: redelivery mengurangi kamar lagi", got)
	}
	if email.sent["B-1"] != 1 {
		t.Fatalf("email terkirim %d kali, seharusnya 1: redelivery harus tetap mengirim email", email.sent["B-1"])
	}
}

// Pengirim mengirim booking yang sama dua kali dengan message ID berbeda,
// misalnya karena ia timeout lalu mencoba lagi.
func TestStep3_SenderResends(t *testing.T) {
	inv, _ := setup(false)
	b := &Broker{}
	b.Publish(booking("m-1", "B-1", 2))
	b.Publish(booking("m-2", "B-1", 2))
	if err := b.Run(inv.Handle); err != nil {
		t.Fatal(err)
	}
	if got := inv.Available("deluxe"); got != 8 {
		t.Fatalf("sisa kamar = %d, seharusnya 8: message ID beda, tapi booking-nya sama", got)
	}
}

// Ledger tidak punya daftar "sudah diproses". Ia harus aman terhadap duplikat
// hanya dari cara ia menyimpan booking.
func TestStep4_IdempotentRepository(t *testing.T) {
	l := NewLedger(map[string]int{"deluxe": 10})
	b := &Broker{LoseFirstAck: true}
	b.Publish(booking("m-1", "B-1", 2))
	b.Publish(booking("m-2", "B-1", 2)) // pengirim mengirim ulang
	b.Publish(booking("m-3", "B-2", 1))
	if err := b.Run(l.Handle); err != nil {
		t.Fatal(err)
	}
	if got := l.Count(); got != 2 {
		t.Fatalf("booking tersimpan = %d, seharusnya 2 (B-1 dan B-2)", got)
	}
	if got := l.Available("deluxe"); got != 7 {
		t.Fatalf("sisa kamar = %d, seharusnya 7", got)
	}

	// Revisi booking B-1 dari 2 kamar menjadi 3. Menyimpan ulang booking yang
	// sama harus mengganti isinya, bukan menambah baris.
	b.Publish(booking("m-4", "B-1", 3))
	if err := b.Run(l.Handle); err != nil {
		t.Fatal(err)
	}
	if got := l.Count(); got != 2 {
		t.Fatalf("booking tersimpan = %d setelah revisi, seharusnya tetap 2", got)
	}
	if got := l.Available("deluxe"); got != 6 {
		t.Fatalf("sisa kamar = %d setelah revisi, seharusnya 6", got)
	}
}
