package lab12

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultPostgresURL = "postgres://lab:lab@localhost:55432/lab"

// setupDB membuat schema Postgres baru untuk satu test dan menghapusnya di akhir.
func setupDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("ED_POSTGRES_URL")
	if url == "" {
		url = defaultPostgresURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("Postgres tidak terjangkau (%v): jalankan `make up` di root kursus", err)
	}
	schema := fmt.Sprintf("lab12_%d_%d", time.Now().UnixNano(), rand.IntN(1_000_000))
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("buat schema: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse ED_POSTGRES_URL: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("buat pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Logf("hapus schema %s: %v", schema, err)
		}
		if err := admin.Close(cleanupCtx); err != nil {
			t.Logf("tutup koneksi admin: %v", err)
		}
	})

	if _, err := pool.Exec(ctx, Schema); err != nil {
		t.Fatalf("buat tabel: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO room_types VALUES ('deluxe', 10), ('suite', 4)`); err != nil {
		t.Fatalf("isi room_types: %v", err)
	}
	return ctx, pool
}

// Tiga booking yang dipakai hampir semua test.
//
//	            1 Des  2 Des  3 Des  4 Des
//	B-1 deluxe    2      2
//	B-2 deluxe           1      1
//	B-3 suite     1
var (
	created1 = Envelope{EventID: "e-1", Event: BookingCreated{BookingID: "B-1", GuestName: "Sari", RoomType: "deluxe", CheckIn: "2026-12-01", CheckOut: "2026-12-03", Rooms: 2}}
	created2 = Envelope{EventID: "e-2", Event: BookingCreated{BookingID: "B-2", GuestName: "Budi", RoomType: "deluxe", CheckIn: "2026-12-02", CheckOut: "2026-12-04", Rooms: 1}}
	created3 = Envelope{EventID: "e-3", Event: BookingCreated{BookingID: "B-3", GuestName: "Wayan", RoomType: "suite", CheckIn: "2026-12-01", CheckOut: "2026-12-02", Rooms: 1}}
)

func project(t *testing.T, ctx context.Context, p *Projector, envs ...Envelope) {
	t.Helper()
	for _, env := range envs {
		if err := p.Handle(ctx, env); err != nil {
			t.Fatalf("Handle(%s): %v", env.EventID, err)
		}
	}
}

// sold membaca rooms_sold dari availability_view; 0 kalau barisnya tidak ada.
func sold(t *testing.T, ctx context.Context, pool *pgxpool.Pool, roomType, day string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(ctx,
		`SELECT rooms_sold FROM availability_view WHERE room_type = $1 AND day = $2::date`,
		roomType, day).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0
	}
	if err != nil {
		t.Fatalf("baca availability_view: %v", err)
	}
	return n
}

func wantSold(t *testing.T, ctx context.Context, pool *pgxpool.Pool, roomType string, want map[string]int, why string) {
	t.Helper()
	for day, n := range want {
		if got := sold(t, ctx, pool, roomType, day); got != n {
			t.Fatalf("availability_view %s %s: rooms_sold = %d, seharusnya %d%s", roomType, day, got, n, why)
		}
	}
}

func status(t *testing.T, ctx context.Context, pool *pgxpool.Pool, bookingID string) string {
	t.Helper()
	var s string
	err := pool.QueryRow(ctx, `SELECT status FROM booking_list_view WHERE booking_id = $1`, bookingID).Scan(&s)
	if err != nil {
		t.Fatalf("booking %s tidak ada di booking_list_view: %v", bookingID, err)
	}
	return s
}

// Read model pertama sudah jadi: satu baris per booking untuk layar resepsionis.
func TestStep0_BookingListView(t *testing.T) {
	ctx, pool := setupDB(t)
	p := NewProjector(pool)
	project(t, ctx, p, created1, created2)

	if s := status(t, ctx, pool, "B-1"); s != StatusConfirmed {
		t.Fatalf("status B-1 = %s, seharusnya confirmed", s)
	}
	var guests []string
	rows, err := pool.Query(ctx, `SELECT guest_name FROM booking_list_view WHERE check_in <= '2026-12-02' AND check_out > '2026-12-02' ORDER BY guest_name`)
	if err != nil {
		t.Fatal(err)
	}
	guests, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(guests, []string{"Budi", "Sari"}) {
		t.Fatalf("tamu yang menginap malam 2 Des = %v, seharusnya [Budi Sari]", guests)
	}
}

// Read model kedua: kamar terjual per malam, sudah dijumlahkan saat event datang.
func TestStep1_AvailabilityViewPerNight(t *testing.T) {
	ctx, pool := setupDB(t)
	p := NewProjector(pool)
	project(t, ctx, p, created1, created2, created3)

	wantSold(t, ctx, pool, "deluxe", map[string]int{
		"2026-12-01": 2,
		"2026-12-02": 3,
		"2026-12-03": 1,
	}, "")
	wantSold(t, ctx, pool, "deluxe", map[string]int{"2026-12-04": 0}, ": tanggal check-out bukan malam menginap")
	wantSold(t, ctx, pool, "suite", map[string]int{"2026-12-01": 1, "2026-12-02": 0}, "")
}

// Event yang sama datang dua kali (redelivery). Angka tidak boleh berlipat.
func TestStep2_DuplicateEventsDoNotDoubleCount(t *testing.T) {
	ctx, pool := setupDB(t)
	p := NewProjector(pool)
	project(t, ctx, p, created1, created1, created2, created1)

	wantSold(t, ctx, pool, "deluxe", map[string]int{
		"2026-12-01": 2,
		"2026-12-02": 3,
		"2026-12-03": 1,
	}, ": angkanya harus tepat walau e-1 datang tiga kali")

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM processed_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("processed_events berisi %d baris, seharusnya 2 (e-1 dan e-2)", n)
	}
}

// Pembatalan mengubah status di daftar booking dan melepas kamarnya.
func TestStep3_CancellationReleasesRooms(t *testing.T) {
	ctx, pool := setupDB(t)
	p := NewProjector(pool)
	project(t, ctx, p, created1, created2)

	cancel := Envelope{EventID: "e-10", Event: BookingCancelled{BookingID: "B-1"}}
	project(t, ctx, p, cancel)
	if s := status(t, ctx, pool, "B-1"); s != StatusCancelled {
		t.Fatalf("status B-1 = %s, seharusnya cancelled", s)
	}
	if s := status(t, ctx, pool, "B-2"); s != StatusConfirmed {
		t.Fatalf("status B-2 = %s, seharusnya tetap confirmed", s)
	}
	after := map[string]int{"2026-12-01": 0, "2026-12-02": 1, "2026-12-03": 1}
	wantSold(t, ctx, pool, "deluxe", after, ": kamar B-1 belum dilepas dengan benar")

	// Redelivery event yang sama.
	project(t, ctx, p, cancel)
	wantSold(t, ctx, pool, "deluxe", after, ": pembatalan yang sama melepas kamar dua kali")

	// Pengirim mengirim ulang pembatalan dengan event ID baru. processed_events
	// tidak mengenalinya, jadi penjaganya harus status booking itu sendiri.
	project(t, ctx, p, Envelope{EventID: "e-11", Event: BookingCancelled{BookingID: "B-1"}})
	wantSold(t, ctx, pool, "deluxe", after, ": pembatalan dengan event ID baru melepas kamar lagi")
}

func getAvailability(t *testing.T, baseURL, query string) (int, AvailabilityResponse) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/availability?"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /availability?%s: %v", query, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body AvailabilityResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("body bukan AvailabilityResponse: %v", err)
		}
	}
	return resp.StatusCode, body
}

// Read model dibaca lewat HTTP, sementara event diproyeksikan di goroutine
// lain. Di antara "event dikirim" dan "terlihat di API" ada jeda.
func TestStep4_HTTPEndpointServesTheView(t *testing.T) {
	ctx, pool := setupDB(t)
	p := NewProjector(pool)
	srv := httptest.NewServer(NewAPI(pool))
	t.Cleanup(srv.Close)
	const query = "room_type=deluxe&from=2026-12-01&to=2026-12-05"

	// Sebelum ada event: jawaban tetap valid, hanya isinya kosong.
	code, body := getAvailability(t, srv.URL, query)
	if code != http.StatusOK {
		t.Fatalf("GET /availability = %d, seharusnya 200", code)
	}
	empty := []AvailabilityDay{
		{Date: "2026-12-01", RoomsSold: 0, Available: 10},
		{Date: "2026-12-02", RoomsSold: 0, Available: 10},
		{Date: "2026-12-03", RoomsSold: 0, Available: 10},
		{Date: "2026-12-04", RoomsSold: 0, Available: 10},
	}
	if body.RoomType != "deluxe" || !slices.Equal(body.Days, empty) {
		t.Fatalf("sebelum ada event: %+v, seharusnya room_type deluxe dan 4 malam dengan 10 kamar tersedia", body)
	}

	// Event mengalir lewat channel ke consumer di goroutine lain.
	events := make(chan Envelope)
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	// Consumer sengaja diperlambat 20 ms per event, meniru consumer yang
	// tertinggal dari producer (consumer lag).
	lagging := func(ctx context.Context, env Envelope) error {
		time.Sleep(20 * time.Millisecond)
		return p.Handle(ctx, env)
	}
	go func() { done <- Run(runCtx, events, lagging) }()
	t.Cleanup(func() {
		stop()
		if err := <-done; err != nil {
			t.Errorf("consumer berhenti dengan error: %v", err)
		}
	})
	for _, env := range []Envelope{created3, created1, created1, created2} {
		select {
		case events <- env:
		case err := <-done:
			done <- err
			t.Fatalf("consumer berhenti sebelum menerima %s: %v", env.EventID, err)
		case <-ctx.Done():
			t.Fatal("timeout mengirim event ke consumer")
		}
	}

	want := []AvailabilityDay{
		{Date: "2026-12-01", RoomsSold: 2, Available: 8},
		{Date: "2026-12-02", RoomsSold: 3, Available: 7},
		{Date: "2026-12-03", RoomsSold: 1, Available: 9},
		{Date: "2026-12-04", RoomsSold: 0, Available: 10},
	}
	deadline := time.Now().Add(3 * time.Second)
	polls := 0
	for {
		polls++
		code, body = getAvailability(t, srv.URL, query)
		if code == http.StatusOK && slices.Equal(body.Days, want) {
			break
		}
		if polls == 1 {
			t.Logf("GET pertama setelah event dikirim masih melihat data lama: %+v", body.Days)
		}
		if time.Now().After(deadline) {
			t.Fatalf("setelah 3 detik API masih menjawab %d %+v, seharusnya %+v", code, body.Days, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Logf("read model menyusul setelah %d kali GET", polls)

	// Input dari luar tidak dipercaya.
	for query, wantCode := range map[string]int{
		"from=2026-12-01&to=2026-12-05":                     http.StatusBadRequest, // tanpa room_type
		"room_type=deluxe&from=kemarin&to=2026-12-05":       http.StatusBadRequest, // tanggal tidak valid
		"room_type=deluxe&from=2026-12-05&to=2026-12-01":    http.StatusBadRequest, // to sebelum from
		"room_type=deluxe&from=2026-12-01&to=2036-12-01":    http.StatusBadRequest, // rentang terlalu panjang
		"room_type=penthouse&from=2026-12-01&to=2026-12-05": http.StatusNotFound,
	} {
		if code, _ := getAvailability(t, srv.URL, query); code != wantCode {
			t.Fatalf("GET /availability?%s = %d, seharusnya %d", query, code, wantCode)
		}
	}
}
