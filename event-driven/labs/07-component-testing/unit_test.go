package lab07

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

type recordingEmailer struct {
	bookingID, to string
}

func (r *recordingEmailer) SendConfirmation(_ context.Context, bookingID, to string) error {
	r.bookingID, r.to = bookingID, to
	return nil
}

func bookingCreatedMessage(t *testing.T) *message.Message {
	t.Helper()
	payload, err := json.Marshal(BookingCreated{BookingID: "B-1", RoomType: "deluxe", Date: "2026-12-24", Rooms: 2, GuestEmail: "tamu@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return message.NewMessage(watermill.NewUUID(), payload)
}

// Unit test setiap handler, satu per satu, tanpa router. Semuanya lulus dari
// awal, padahal service ini punya dua bug. Itulah alasan lab ini ada.
func TestStep0_UnitTestsPassOnABrokenService(t *testing.T) {
	t.Run("handler email memanggil Emailer", func(t *testing.T) {
		emailer := &recordingEmailer{}
		handler := confirmationEmail{emailer: emailer}

		if err := handler.handleBookingCreated(bookingCreatedMessage(t)); err != nil {
			t.Fatal(err)
		}
		if emailer.bookingID != "B-1" || emailer.to != "tamu@example.com" {
			t.Fatalf("Emailer menerima (%q, %q)", emailer.bookingID, emailer.to)
		}
	})

	t.Run("handler sync mengirim delta negatif", func(t *testing.T) {
		var got AvailabilityUpdate
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("decode request: %v", err)
			}
		}))
		defer server.Close()
		handler := channelSync{baseURL: server.URL, client: server.Client()}

		if err := handler.handleBookingCreated(bookingCreatedMessage(t)); err != nil {
			t.Fatal(err)
		}
		want := AvailabilityUpdate{BookingID: "B-1", RoomType: "deluxe", Date: "2026-12-24", Delta: -2}
		if got != want {
			t.Fatalf("channel manager menerima %+v, seharusnya %+v", got, want)
		}
	})
}
