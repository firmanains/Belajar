package lab15

import "encoding/json"

// File ini sudah jadi. Jangan diubah.

// Nama event di broker. Nama adalah bagian dari kontrak.
const (
	NameBookingCreatedV1 = "BookingCreated"
	NameBookingCreatedV2 = "BookingCreated_v2"
)

// DefaultRatePlan dipakai untuk event yang dibuat sebelum rate plan ada.
const DefaultRatePlan = "standard"

// Message adalah event di perjalanan: nama dan payload JSON.
type Message struct {
	Name    string
	Payload json.RawMessage
}

// BookingCreatedV1 adalah kontrak pertama. Total berisi harga dalam rupiah;
// mata uangnya tidak tertulis karena dulu semua booking memakai IDR.
// RatePlan ditambahkan belakangan sebagai field opsional.
type BookingCreatedV1 struct {
	BookingID string `json:"booking_id"`
	RoomType  string `json:"room_type"`
	Rooms     int    `json:"rooms"`
	Total     int64  `json:"total"`
	RatePlan  string `json:"rate_plan,omitempty"`
}

// Money adalah jumlah uang beserta mata uangnya.
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// BookingCreatedV2 mengubah arti dan bentuk "total": dari angka rupiah
// menjadi Money. Perubahan itu breaking, karena itu namanya berbeda.
type BookingCreatedV2 struct {
	BookingID string `json:"booking_id"`
	RoomType  string `json:"room_type"`
	Rooms     int    `json:"rooms"`
	RatePlan  string `json:"rate_plan"`
	Total     Money  `json:"total"`
}

// Booking adalah data booking di dalam PMS pada saat event diterbitkan.
type Booking struct {
	ID       string
	RoomType string
	Rooms    int
	RatePlan string
	Total    Money
}

// InternalBookingConfirmed adalah event internal PMS. Hanya kode di dalam PMS
// yang membacanya, jadi bentuknya bebas berubah kapan saja. Ia memuat hal yang
// tidak boleh keluar dari service.
type InternalBookingConfirmed struct {
	RowID      int64  `json:"row_id"`
	BookingID  string `json:"booking_id"`
	RoomType   string `json:"room_type"`
	Rooms      int    `json:"rooms"`
	RatePlan   string `json:"rate_plan"`
	SellAmount int64  `json:"sell_amount"`
	CostAmount int64  `json:"cost_amount"`
	Currency   string `json:"currency"`
	StaffNote  string `json:"staff_note"`
}
