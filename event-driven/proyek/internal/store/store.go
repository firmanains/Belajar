// Package store berisi SQL untuk booking dan pemakaian kamar per tanggal.
// Write model dan read model punya bentuk tabel yang sama di proyek ini, jadi
// satu set helper melayani keduanya.
//
// File ini sudah jadi. Jangan diubah.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/platform"
)

// Tables menunjuk sepasang tabel: daftar booking dan kamar terpakai per tanggal.
type Tables struct {
	Bookings  string
	Inventory string
}

var (
	WriteModel = Tables{Bookings: "bookings", Inventory: "inventory"}
	ReadModel  = Tables{Bookings: "rm_bookings", Inventory: "rm_tape_chart"}
)

// LockAndGetBooking mengunci satu booking sampai transaction selesai, lalu
// mengembalikan state terakhirnya (nil kalau belum ada). Lock-nya advisory lock,
// bukan SELECT FOR UPDATE, karena FOR UPDATE tidak mengunci apa pun saat
// barisnya belum ada: dua revision pertama dari booking yang sama bisa lolos
// bersamaan. Di luar transaction, lock ini langsung lepas dan tidak melindungi.
func (t Tables) LockAndGetBooking(ctx context.Context, q platform.DBTX, bookingID string) (*event.Booking, error) {
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, t.Bookings+":"+bookingID); err != nil {
		return nil, fmt.Errorf("lock booking %s: %w", bookingID, err)
	}
	return t.GetBooking(ctx, q, bookingID)
}

// GetBooking mengembalikan state terakhir sebuah booking, atau nil kalau belum ada.
func (t Tables) GetBooking(ctx context.Context, q platform.DBTX, bookingID string) (*event.Booking, error) {
	var b event.Booking
	err := q.QueryRow(ctx, `
		SELECT booking_id, revision, status, room_type, check_in::text, check_out::text, rooms, guest_email
		FROM `+t.Bookings+` WHERE booking_id = $1`, bookingID).
		Scan(&b.BookingID, &b.Revision, &b.Status, &b.RoomType, &b.CheckIn, &b.CheckOut, &b.Rooms, &b.GuestEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get booking %s: %w", bookingID, err)
	}
	return &b, nil
}

// SaveBooking menyimpan state booking, menimpa state sebelumnya.
func (t Tables) SaveBooking(ctx context.Context, q platform.DBTX, b event.Booking) error {
	_, err := q.Exec(ctx, `
		INSERT INTO `+t.Bookings+` (booking_id, revision, status, room_type, check_in, check_out, rooms, guest_email)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8)
		ON CONFLICT (booking_id) DO UPDATE SET
			revision = EXCLUDED.revision, status = EXCLUDED.status, room_type = EXCLUDED.room_type,
			check_in = EXCLUDED.check_in, check_out = EXCLUDED.check_out, rooms = EXCLUDED.rooms,
			guest_email = EXCLUDED.guest_email`,
		b.BookingID, b.Revision, b.Status, b.RoomType, b.CheckIn, b.CheckOut, b.Rooms, b.GuestEmail)
	if err != nil {
		return fmt.Errorf("save booking %s: %w", b.BookingID, err)
	}
	return nil
}

// Occupy menambah kamar terpakai untuk setiap malam menginap booking b.
// Release mengembalikannya. Keduanya menumpuk: memanggil Occupy dua kali untuk
// booking yang sama menghitung booking itu dua kali.
func (t Tables) Occupy(ctx context.Context, q platform.DBTX, b event.Booking) error {
	return t.adjust(ctx, q, b, b.Rooms)
}

func (t Tables) Release(ctx context.Context, q platform.DBTX, b event.Booking) error {
	return t.adjust(ctx, q, b, -b.Rooms)
}

func (t Tables) adjust(ctx context.Context, q platform.DBTX, b event.Booking, delta int) error {
	_, err := q.Exec(ctx, `
		INSERT INTO `+t.Inventory+` AS inv (room_type, day, booked)
		SELECT $1::text, d::date, $4::int
		FROM generate_series($2::date, $3::date - 1, interval '1 day') AS d
		ON CONFLICT (room_type, day) DO UPDATE SET booked = inv.booked + EXCLUDED.booked`,
		b.RoomType, b.CheckIn, b.CheckOut, delta)
	if err != nil {
		return fmt.Errorf("adjust %s untuk booking %s: %w", t.Inventory, b.BookingID, err)
	}
	return nil
}

// Day adalah satu sel tape chart: satu room type pada satu tanggal.
type Day struct {
	RoomType  string `json:"room_type"`
	Date      string `json:"date"`
	Total     int    `json:"total"`
	Booked    int    `json:"booked"`
	Available int    `json:"available"`
}

// Availability mengembalikan semua room type untuk tanggal from sampai sebelum to.
func (t Tables) Availability(ctx context.Context, q platform.DBTX, from, to string) ([]Day, error) {
	rows, err := q.Query(ctx, `
		SELECT rt.room_type, d::date::text, rt.total, COALESCE(inv.booked, 0)
		FROM room_types rt
		CROSS JOIN generate_series($1::date, $2::date - 1, interval '1 day') AS d
		LEFT JOIN `+t.Inventory+` inv ON inv.room_type = rt.room_type AND inv.day = d::date
		ORDER BY rt.room_type, d`, from, to)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", t.Inventory, err)
	}
	defer rows.Close()
	days := []Day{}
	for rows.Next() {
		var d Day
		if err := rows.Scan(&d.RoomType, &d.Date, &d.Total, &d.Booked); err != nil {
			return nil, fmt.Errorf("scan %s: %w", t.Inventory, err)
		}
		d.Available = d.Total - d.Booked
		days = append(days, d)
	}
	return days, rows.Err()
}
