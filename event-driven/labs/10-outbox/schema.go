package lab10

// File ini sudah jadi. Jangan diubah; baca saja.

// Schema dibuat ulang oleh test di schema Postgres tersendiri.
//
// Kolom outbox yang penting:
//   - status: PENDING (belum terkirim), SENT (sudah), DEAD (menyerah)
//   - attempts: berapa kali forwarder gagal melakukan publish baris ini
//   - last_error: pesan error dari percobaan terakhir, untuk manusia yang menyelidiki
const Schema = `
CREATE TABLE bookings (
	id        text PRIMARY KEY,
	room_type text NOT NULL,
	rooms     int  NOT NULL CHECK (rooms > 0)
);

CREATE TABLE outbox (
	id         bigserial   PRIMARY KEY,
	topic      text        NOT NULL,
	payload    jsonb       NOT NULL,
	status     text        NOT NULL DEFAULT 'PENDING',
	attempts   int         NOT NULL DEFAULT 0,
	last_error text,
	created_at timestamptz NOT NULL DEFAULT now(),
	sent_at    timestamptz
);

-- Forwarder hanya mencari baris PENDING, jadi index-nya cukup untuk baris itu.
CREATE INDEX outbox_pending ON outbox (id) WHERE status = 'PENDING';
`

const (
	StatusPending = "PENDING"
	StatusSent    = "SENT"
	StatusDead    = "DEAD"
)

const TopicBookingCreated = "booking.created"

// Booking adalah data yang disimpan service.
type Booking struct {
	ID       string
	RoomType string
	Rooms    int
}

// BookingCreated adalah event yang dikabarkan ke sistem lain.
type BookingCreated struct {
	BookingID string `json:"booking_id"`
	RoomType  string `json:"room_type"`
	Rooms     int    `json:"rooms"`
}
