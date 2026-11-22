package lab07

import "testing"

// Booking yang dibatalkan harus mengembalikan availability di channel manager.
func TestStep5_CancelledBookingRestoresAvailability(t *testing.T) {
	cm := newFakeChannelManager(t, 0)
	pubSub := startService(t, cm, &fakeEmailer{})

	publish(t, pubSub, TopicBookingCancelled, BookingCancelled{BookingID: "B-1", RoomType: "deluxe", Date: "2026-12-24", Rooms: 2})

	if !eventually(waitTimeout, func() bool { return len(cm.accepted()) == 1 }) {
		t.Fatalf("channel manager tiruan merekam %d update dalam %s, seharusnya 1", len(cm.accepted()), waitTimeout)
	}
	want := AvailabilityUpdate{BookingID: "B-1", RoomType: "deluxe", Date: "2026-12-24", Delta: 2}
	if got := cm.accepted()[0]; got != want {
		t.Fatalf("channel manager menerima %+v, seharusnya %+v", got, want)
	}
}
