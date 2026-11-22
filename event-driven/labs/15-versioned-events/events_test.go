package lab15

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// File ini sudah jadi. Jangan diubah.

func find(msgs []Message, name string) (Message, bool) {
	for _, m := range msgs {
		if m.Name == name {
			return m, true
		}
	}
	return Message{}, false
}

func names(msgs []Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Name)
	}
	sort.Strings(out)
	return out
}

// legacyConsumer adalah consumer lama milik tim lain yang belum di-deploy
// ulang: ia hanya tahu "total" sebagai angka rupiah.
func legacyConsumer(payload []byte) (total int64, err error) {
	var ev struct {
		BookingID string `json:"booking_id"`
		Total     int64  `json:"total"`
	}
	if err := json.Unmarshal(payload, &ev); err != nil {
		return 0, err
	}
	return ev.Total, nil
}

// Dunia ideal: producer dan consumer sepakat pada satu bentuk.
func TestStep0_V1RoundTrip(t *testing.T) {
	got, err := DecodeV1([]byte(`{"booking_id":"B-1","room_type":"deluxe","rooms":2,"total":1500000,"rate_plan":"flexible"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := BookingCreatedV1{BookingID: "B-1", RoomType: "deluxe", Rooms: 2, Total: 1500000, RatePlan: "flexible"}
	if got != want {
		t.Fatalf("DecodeV1 = %+v, seharusnya %+v", got, want)
	}
}

// Producer menambah field opsional "guest_note". Consumer yang belum tahu
// field itu harus tetap berjalan.
func TestStep1_UnknownFieldIsTolerated(t *testing.T) {
	got, err := DecodeV1([]byte(`{"booking_id":"B-1","room_type":"deluxe","rooms":2,"total":1500000,"rate_plan":"flexible","guest_note":"late check-in"}`))
	if err != nil {
		t.Fatalf("consumer berhenti karena field baru yang tidak ia butuhkan: %v", err)
	}
	if got.BookingID != "B-1" || got.Total != 1500000 {
		t.Fatalf("DecodeV1 = %+v, field yang dikenal harus tetap terbaca", got)
	}
}

// Event lama, dibuat sebelum rate plan ada, tidak punya "rate_plan".
// Consumer baru harus memberinya arti yang jelas, bukan string kosong.
func TestStep2_MissingOptionalFieldGetsDefault(t *testing.T) {
	got, err := DecodeV1([]byte(`{"booking_id":"B-0","room_type":"deluxe","rooms":1,"total":750000}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.RatePlan != DefaultRatePlan {
		t.Fatalf("RatePlan = %q untuk event tanpa rate_plan, seharusnya %q", got.RatePlan, DefaultRatePlan)
	}
}

// "total" berubah dari angka menjadi {amount, currency}. Itu breaking change:
// v2 harus terbit dengan nama baru, berdampingan dengan v1.
func TestStep3_V2IsPublishedAlongsideV1(t *testing.T) {
	msgs, err := MessagesFor(Booking{
		ID: "B-1", RoomType: "deluxe", Rooms: 2, RatePlan: "flexible",
		Total: Money{Amount: 1500000, Currency: "IDR"},
	})
	if err != nil {
		t.Fatal(err)
	}

	v1, ok := find(msgs, NameBookingCreatedV1)
	if !ok {
		t.Fatalf("message yang terbit: %v, seharusnya masih ada %q untuk consumer lama", names(msgs), NameBookingCreatedV1)
	}
	total, err := legacyConsumer(v1.Payload)
	if err != nil {
		t.Fatalf("consumer lama tidak bisa membaca %q lagi: %v\n payload: %s", NameBookingCreatedV1, err, v1.Payload)
	}
	if total != 1500000 {
		t.Fatalf("consumer lama membaca total = %d, seharusnya 1500000", total)
	}

	v2, ok := find(msgs, NameBookingCreatedV2)
	if !ok {
		t.Fatalf("message yang terbit: %v, seharusnya ada %q", names(msgs), NameBookingCreatedV2)
	}
	var got BookingCreatedV2
	if err := json.Unmarshal(v2.Payload, &got); err != nil {
		t.Fatalf("payload %q bukan BookingCreatedV2: %v", NameBookingCreatedV2, err)
	}
	if want := (Money{Amount: 1500000, Currency: "IDR"}); got.Total != want {
		t.Fatalf("v2 total = %+v, seharusnya %+v", got.Total, want)
	}

	// Booking dalam USD tidak bisa dinyatakan di v1, yang mengandaikan rupiah.
	usd, err := MessagesFor(Booking{
		ID: "B-2", RoomType: "deluxe", Rooms: 1, RatePlan: "flexible",
		Total: Money{Amount: 120, Currency: "USD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := find(usd, NameBookingCreatedV1); ok {
		t.Fatalf("booking USD terbit sebagai %q: consumer lama akan membacanya sebagai 120 rupiah", NameBookingCreatedV1)
	}
	if _, ok := find(usd, NameBookingCreatedV2); !ok {
		t.Fatalf("booking USD harus tetap terbit sebagai %q", NameBookingCreatedV2)
	}
}

// Consumer baru membaca sejarah (v1) dan event baru (v2) lewat satu pintu.
func TestStep4_UpcasterTurnsV1IntoV2(t *testing.T) {
	old := Message{Name: NameBookingCreatedV1, Payload: []byte(`{"booking_id":"B-0","room_type":"deluxe","rooms":1,"total":750000}`)}
	got, err := Upcast(old)
	if err != nil {
		t.Fatalf("Upcast(%s): %v", old.Name, err)
	}
	want := BookingCreatedV2{
		BookingID: "B-0", RoomType: "deluxe", Rooms: 1, RatePlan: DefaultRatePlan,
		Total: Money{Amount: 750000, Currency: "IDR"},
	}
	if got != want {
		t.Fatalf("Upcast(v1) = %+v, seharusnya %+v", got, want)
	}

	cur := Message{Name: NameBookingCreatedV2, Payload: []byte(`{"booking_id":"B-2","room_type":"suite","rooms":1,"rate_plan":"flexible","total":{"amount":120,"currency":"USD"}}`)}
	got, err = Upcast(cur)
	if err != nil {
		t.Fatalf("Upcast(%s): %v", cur.Name, err)
	}
	want = BookingCreatedV2{
		BookingID: "B-2", RoomType: "suite", Rooms: 1, RatePlan: "flexible",
		Total: Money{Amount: 120, Currency: "USD"},
	}
	if got != want {
		t.Fatalf("Upcast(v2) = %+v, seharusnya %+v", got, want)
	}

	if _, err := Upcast(Message{Name: "PaymentCaptured", Payload: []byte(`{}`)}); err == nil {
		t.Fatalf("Upcast harus menolak event yang bukan BookingCreated")
	}
}

// Event internal tidak boleh keluar apa adanya: isinya harga pokok, catatan
// staf, dan ID baris database.
func TestStep5_InternalEventDoesNotLeak(t *testing.T) {
	msg, err := ToPublic(InternalBookingConfirmed{
		RowID: 48213, BookingID: "B-1", RoomType: "deluxe", Rooms: 2, RatePlan: "flexible",
		SellAmount: 1500000, CostAmount: 900000, Currency: "IDR",
		StaffNote: "tamu sering komplain",
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Name != NameBookingCreatedV2 {
		t.Fatalf("nama message = %q, seharusnya %q", msg.Name, NameBookingCreatedV2)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(msg.Payload, &fields); err != nil {
		t.Fatalf("payload bukan JSON object: %v", err)
	}
	var got []string
	for k := range fields {
		got = append(got, k)
	}
	sort.Strings(got)
	want := []string{"booking_id", "rate_plan", "room_type", "rooms", "total"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("field yang keluar dari service: %v\n seharusnya hanya:           %v", got, want)
	}

	var pub BookingCreatedV2
	if err := json.Unmarshal(msg.Payload, &pub); err != nil {
		t.Fatalf("payload bukan BookingCreatedV2: %v", err)
	}
	if wantTotal := (Money{Amount: 1500000, Currency: "IDR"}); pub.Total != wantTotal {
		t.Fatalf("total publik = %+v, seharusnya harga jual %+v", pub.Total, wantTotal)
	}
}
