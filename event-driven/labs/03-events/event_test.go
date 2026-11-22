package lab03

// File ini sudah jadi. Jangan diubah.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleBooking() Booking {
	return Booking{
		ID:         "B-1",
		RoomType:   "deluxe",
		CheckIn:    time.Date(2026, 12, 24, 14, 0, 0, 0, time.FixedZone("WIB", 7*3600)),
		CheckOut:   time.Date(2026, 12, 26, 12, 0, 0, 0, time.FixedZone("WIB", 7*3600)),
		Rooms:      2,
		GuestEmail: "tamu@example.com",
		CardNumber: "4111111111111111",
		InternalID: 90817,
	}
}

// asMap mengubah value apa pun menjadi bentuk JSON-nya, seperti yang dilihat
// consumer di service lain.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal gagal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal gagal: %v", err)
	}
	return m
}

func envelopeWith(name, payload string) Envelope {
	return Envelope{ID: "e-1", Name: name, OccurredAt: time.Now().UTC(), Payload: json.RawMessage(payload)}
}

// Event bisa diubah menjadi JSON dan dibaca kembali.
func TestStep0_EventSurvivesJSONRoundTrip(t *testing.T) {
	data, err := json.Marshal(NewBookingCreated(sampleBooking()))
	if err != nil {
		t.Fatalf("marshal gagal: %v", err)
	}
	var got BookingCreated
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal gagal: %v", err)
	}
	if got.BookingID != "B-1" || got.RoomType != "deluxe" || got.Rooms != 2 {
		t.Fatalf("event berubah setelah bolak-balik JSON: %+v", got)
	}
}

// Payload adalah kontrak dengan service lain, bukan salinan tabel.
func TestStep1_PayloadIsADeliberateContract(t *testing.T) {
	m := asMap(t, NewBookingCreated(sampleBooking()))

	want := map[string]any{
		"booking_id": "B-1",
		"room_type":  "deluxe",
		"check_in":   "2026-12-24",
		"check_out":  "2026-12-26",
		"rooms":      float64(2),
	}
	for key, value := range want {
		if got, ok := m[key]; !ok {
			t.Errorf("payload tidak punya field %q; field yang ada: %v", key, keysOf(m))
		} else if got != value {
			t.Errorf("payload[%q] = %v, seharusnya %v", key, got, value)
		}
	}
	for key, value := range m {
		if strings.Contains(strings.ToLower(key), "card") || value == "4111111111111111" {
			t.Errorf("payload membawa nomor kartu di field %q: semua consumer dan log broker ikut menyimpannya", key)
		}
		if strings.Contains(strings.ToLower(key), "internal") {
			t.Errorf("payload membawa detail internal di field %q", key)
		}
		if key != strings.ToLower(key) {
			t.Errorf("field %q mengikuti nama field Go; kontrak JSON memakai snake_case yang ditentukan lewat tag", key)
		}
	}
}

// Envelope membawa metadata yang dibutuhkan semua consumer.
func TestStep2_EnvelopeCarriesMetadata(t *testing.T) {
	before := time.Now().Add(-time.Second)
	env, err := NewEnvelope(BookingCreatedName, "req-42", NewBookingCreated(sampleBooking()))
	if err != nil {
		t.Fatalf("NewEnvelope gagal: %v", err)
	}
	other, err := NewEnvelope(BookingCreatedName, "req-42", NewBookingCreated(sampleBooking()))
	if err != nil {
		t.Fatalf("NewEnvelope gagal: %v", err)
	}

	if env.ID == "" {
		t.Error("envelope tidak punya ID: consumer tidak bisa mengenali pesan yang sama datang dua kali")
	} else if env.ID == other.ID {
		t.Errorf("dua envelope punya ID yang sama (%q)", env.ID)
	}
	if env.Name != BookingCreatedName {
		t.Errorf("Name = %q, seharusnya %q: consumer tidak tahu cara membaca payload", env.Name, BookingCreatedName)
	}
	if env.CorrelationID != "req-42" {
		t.Errorf("CorrelationID = %q, seharusnya %q: jejak request terputus di sini", env.CorrelationID, "req-42")
	}
	if env.OccurredAt.Before(before) || env.OccurredAt.After(time.Now().Add(time.Second)) {
		t.Errorf("OccurredAt = %v, seharusnya waktu sekarang", env.OccurredAt)
	}
	if env.OccurredAt.Location() != time.UTC {
		t.Errorf("OccurredAt memakai timezone %v, seharusnya UTC", env.OccurredAt.Location())
	}

	m := asMap(t, env)
	for _, key := range []string{"id", "name", "occurred_at", "correlation_id", "payload"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON envelope tidak punya field %q", key)
		}
	}
}

// Consumer memilih cara membaca payload dari nama event.
func TestStep3_DecodeByEventName(t *testing.T) {
	created, err := DecodePayload(envelopeWith(BookingCreatedName,
		`{"booking_id":"B-1","room_type":"deluxe","check_in":"2026-12-24","check_out":"2026-12-26","rooms":2}`))
	if err != nil {
		t.Fatalf("DecodePayload(BookingCreated) gagal: %v", err)
	}
	bc, ok := created.(BookingCreated)
	if !ok {
		t.Fatalf("DecodePayload(BookingCreated) mengembalikan %T, seharusnya BookingCreated", created)
	}
	if bc.BookingID != "B-1" || bc.RoomType != "deluxe" || bc.Rooms != 2 {
		t.Fatalf("BookingCreated terbaca %+v", bc)
	}

	cancelled, err := DecodePayload(envelopeWith(BookingCancelledName, `{"booking_id":"B-1","reason":"guest request"}`))
	if err != nil {
		t.Fatalf("DecodePayload(BookingCancelled) gagal: %v", err)
	}
	if c, ok := cancelled.(BookingCancelled); !ok || c.BookingID != "B-1" || c.Reason != "guest request" {
		t.Fatalf("DecodePayload(BookingCancelled) mengembalikan %#v", cancelled)
	}

	_, err = DecodePayload(envelopeWith("LoyaltyPointsEarned", `{"points":10}`))
	if !errors.Is(err, ErrUnknownEvent) {
		t.Fatalf("event yang tidak dikenal menghasilkan error %v, seharusnya ErrUnknownEvent", err)
	}
}

// Consumer yang baik menoleransi hal yang tidak ia kenal, tapi menolak event
// yang tidak bisa ia pakai.
func TestStep4_TolerantReader(t *testing.T) {
	// Producer versi baru menambah field. Consumer lama tidak boleh ikut rusak.
	v, err := DecodePayload(envelopeWith(BookingCreatedName,
		`{"booking_id":"B-1","room_type":"deluxe","check_in":"2026-12-24","check_out":"2026-12-26","rooms":2,"loyalty_tier":"gold"}`))
	if err != nil {
		t.Fatalf("field baru yang tidak dikenal membuat consumer gagal: %v", err)
	}
	if bc, ok := v.(BookingCreated); !ok || bc.BookingID != "B-1" {
		t.Fatalf("event dengan field baru terbaca %#v", v)
	}

	// Field wajib hilang. Zero value yang lolos diam-diam lebih berbahaya
	// daripada error.
	_, err = DecodePayload(envelopeWith(BookingCreatedName, `{"room_type":"deluxe","rooms":2}`))
	if !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("event tanpa booking_id menghasilkan error %v, seharusnya ErrInvalidEvent", err)
	}
	_, err = DecodePayload(envelopeWith(BookingCreatedName, `{"booking_id":"B-1","room_type":"deluxe","rooms":0}`))
	if !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("event dengan rooms = 0 menghasilkan error %v, seharusnya ErrInvalidEvent", err)
	}

	// Payload bukan JSON yang sah.
	_, err = DecodePayload(envelopeWith(BookingCreatedName, `{"booking_id":`))
	if !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("payload yang rusak menghasilkan error %v, seharusnya ErrInvalidEvent", err)
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
