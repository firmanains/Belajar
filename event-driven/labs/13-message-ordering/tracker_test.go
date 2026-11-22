package lab13

import (
	"errors"
	"fmt"
	"testing"
)

// File ini sudah jadi. Jangan diubah.

func created(id string, rooms int) BookingEvent {
	return BookingEvent{EventID: id + "-r1", Name: BookingCreated, BookingID: id, Revision: 1, Rooms: rooms}
}

func modified(id string, rev, rooms int) BookingEvent {
	return BookingEvent{EventID: fmt.Sprintf("%s-r%d", id, rev), Name: BookingModified, BookingID: id, Revision: rev, Rooms: rooms}
}

func cancelled(id string, rev int) BookingEvent {
	return BookingEvent{EventID: fmt.Sprintf("%s-r%d", id, rev), Name: BookingCancelled, BookingID: id, Revision: rev}
}

func mustHandle(t *testing.T, tr *Tracker, ev BookingEvent) {
	t.Helper()
	if err := tr.Handle(ev); err != nil {
		t.Fatalf("Handle(%s revision %d) mengembalikan error: %v", ev.Name, ev.Revision, err)
	}
}

func assertBooking(t *testing.T, tr *Tracker, id string, want Booking) {
	t.Helper()
	got, ok := tr.Booking(id)
	if !ok {
		t.Fatalf("booking %s tidak dikenal Tracker, seharusnya %+v", id, want)
	}
	if got != want {
		t.Fatalf("booking %s = %+v, seharusnya %+v", id, got, want)
	}
}

// deliverWithRedelivery meniru broker at-least-once: event yang handler-nya
// mengembalikan error dikirim ulang setelah event lain, paling banyak
// maxRounds putaran. Return-nya adalah event yang tetap gagal sampai akhir.
func deliverWithRedelivery(tr *Tracker, events []BookingEvent, maxRounds int) []BookingEvent {
	pending := events
	for round := 0; round < maxRounds && len(pending) > 0; round++ {
		var failed []BookingEvent
		for _, ev := range pending {
			if err := tr.Handle(ev); err != nil {
				failed = append(failed, ev)
			}
		}
		pending = failed
	}
	return pending
}

// Dunia ideal: event datang berurutan dan tepat sekali.
func TestStep0_InOrder(t *testing.T) {
	tr := NewTracker()
	mustHandle(t, tr, created("B-1", 2))
	mustHandle(t, tr, modified("B-1", 2, 3))
	assertBooking(t, tr, "B-1", Booking{Revision: 2, Rooms: 3})

	mustHandle(t, tr, cancelled("B-1", 3))
	assertBooking(t, tr, "B-1", Booking{Revision: 3, Rooms: 0, Cancelled: true})
}

// Revision 2 dikirim ulang setelah revision 3 sudah diterapkan
// (misalnya retry dari consumer yang lambat).
func TestStep2_StaleRevisionIsIgnored(t *testing.T) {
	tr := NewTracker()
	mustHandle(t, tr, created("B-1", 2))
	mustHandle(t, tr, modified("B-1", 2, 3))
	mustHandle(t, tr, modified("B-1", 3, 5))

	if err := tr.Handle(modified("B-1", 2, 3)); err != nil {
		t.Fatalf("event stale harus di-ack (return nil), bukan error: %v", err)
	}
	assertBooking(t, tr, "B-1", Booking{Revision: 3, Rooms: 5})

	// Duplikat persis dari revisi terakhir juga tidak mengubah apa pun.
	mustHandle(t, tr, modified("B-1", 3, 5))
	assertBooking(t, tr, "B-1", Booking{Revision: 3, Rooms: 5})
}

// BookingCancelled (revision 2) tiba sebelum BookingCreated (revision 1).
func TestStep3_GapIsDetected(t *testing.T) {
	tr := NewTracker()

	err := tr.Handle(cancelled("B-1", 2))
	if !errors.Is(err, ErrGap) {
		t.Fatalf("Handle(BookingCancelled revision 2) pada booking yang belum dikenal = %v, seharusnya error yang membungkus ErrGap", err)
	}
	if _, ok := tr.Booking("B-1"); ok {
		t.Fatalf("event yang ditolak karena gap tidak boleh mengubah keadaan")
	}

	mustHandle(t, tr, created("B-1", 2))
	mustHandle(t, tr, cancelled("B-1", 2)) // dikirim ulang oleh broker
	assertBooking(t, tr, "B-1", Booking{Revision: 2, Rooms: 0, Cancelled: true})

	// Gap di tengah: revision 4 tiba, revision 3 belum.
	tr2 := NewTracker()
	mustHandle(t, tr2, created("B-2", 1))
	mustHandle(t, tr2, modified("B-2", 2, 2))
	if err := tr2.Handle(modified("B-2", 4, 4)); !errors.Is(err, ErrGap) {
		t.Fatalf("Handle(revision 4) saat baru punya revision 2 = %v, seharusnya ErrGap", err)
	}
	assertBooking(t, tr2, "B-2", Booking{Revision: 2, Rooms: 2})
}

// Urutan terbalik dan ada duplikat, untuk dua booking sekaligus.
// Dengan redelivery, keadaan akhir harus tetap benar.
func TestStep4_ConvergesUnderRedelivery(t *testing.T) {
	tr := NewTracker()
	events := []BookingEvent{
		cancelled("B-1", 3),
		modified("B-2", 2, 4),
		modified("B-1", 2, 3),
		modified("B-2", 2, 4), // duplikat
		created("B-1", 2),
		created("B-2", 1),
		modified("B-1", 2, 3), // duplikat
	}

	if left := deliverWithRedelivery(tr, events, 10); len(left) > 0 {
		t.Fatalf("%d event tidak pernah berhasil diproses, contoh: %+v", len(left), left[0])
	}
	assertBooking(t, tr, "B-1", Booking{Revision: 3, Rooms: 0, Cancelled: true})
	assertBooking(t, tr, "B-2", Booking{Revision: 2, Rooms: 4})
}
