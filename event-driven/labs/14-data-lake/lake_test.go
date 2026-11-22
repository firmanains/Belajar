package lab14

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// File ini sudah jadi. Jangan diubah.

// newLake membuat schema Postgres dengan nama unik, menjalankan Migrate di
// dalamnya, dan menghapus schema itu saat test selesai.
func newLake(t *testing.T) (*Lake, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("ED_POSTGRES_URL")
	if url == "" {
		url = "postgres://lab:lab@localhost:55432/lab"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("Postgres tidak terjangkau (%v): jalankan `make up` di root kursus", err)
	}
	schema := fmt.Sprintf("lab14_%d", rand.Uint64())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("membuat schema %s: %v", schema, err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("ED_POSTGRES_URL tidak valid: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("membuka pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Logf("menghapus schema %s: %v", schema, err)
		}
		if err := admin.Close(ctx); err != nil {
			t.Logf("menutup koneksi admin: %v", err)
		}
	})

	lake := NewLake(pool)
	if err := lake.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return lake, pool
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

var day = time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC)

func at(hour int) time.Time { return day.Add(time.Duration(hour) * time.Hour) }

func event(id, name string, occurredAt time.Time, payload string) Envelope {
	return Envelope{ID: id, Name: name, OccurredAt: occurredAt, Payload: json.RawMessage(payload)}
}

func countRows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(testCtx(t), `SELECT count(*) FROM events`).Scan(&n); err != nil {
		t.Fatalf("menghitung baris events: %v", err)
	}
	return n
}

// sameJSON membandingkan dua dokumen JSON tanpa peduli spasi dan urutan key,
// karena kolom jsonb tidak mempertahankan keduanya.
func sameJSON(t *testing.T, got, want json.RawMessage) bool {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("payload tersimpan bukan JSON: %v", err)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("payload harapan bukan JSON: %v", err)
	}
	return reflect.DeepEqual(g, w)
}

func ids(events []Envelope) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.ID)
	}
	return out
}

// Dunia ideal: satu event disimpan sekali, lalu bisa dibaca kembali.
func TestStep0_AppendAndQuery(t *testing.T) {
	lake, _ := newLake(t)
	ctx := testCtx(t)

	in := event("evt-1", "BookingCreated", at(9), `{"booking_id":"B-1","room_type":"deluxe","rooms":2}`)
	if err := lake.Append(ctx, in); err != nil {
		t.Fatal(err)
	}

	got, err := lake.Query(ctx, "BookingCreated", at(0), at(24))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "evt-1" {
		t.Fatalf("Query mengembalikan %v, seharusnya [evt-1]", ids(got))
	}
	if !got[0].OccurredAt.Equal(at(9)) {
		t.Fatalf("occurred_at = %s, seharusnya %s", got[0].OccurredAt, at(9))
	}
	if !sameJSON(t, got[0].Payload, in.Payload) {
		t.Fatalf("payload berubah: %s", got[0].Payload)
	}
}

// Forwarder menerima event at-least-once, jadi event yang sama bisa datang
// dua kali. Data lake harus tetap berisi satu baris per event.
func TestStep1_DuplicateEventIsStoredOnce(t *testing.T) {
	lake, pool := newLake(t)
	ctx := testCtx(t)

	e := event("evt-1", "BookingCreated", at(9), `{"booking_id":"B-1","room_type":"deluxe","rooms":2}`)
	for i := 1; i <= 2; i++ {
		if err := lake.Append(ctx, e); err != nil {
			t.Fatalf("Append ke-%d untuk event yang sama harus berhasil tanpa error: %v", i, err)
		}
	}
	if n := countRows(t, pool); n != 1 {
		t.Fatalf("tabel events berisi %d baris untuk satu event, seharusnya 1: analisis di atas data ini akan menghitung booking dua kali", n)
	}
}

// Data lake menyimpan semua event apa adanya: event yang belum dikenal saat
// forwarder ditulis, dan field yang baru ditambahkan pengirim.
func TestStep2_ForwardKeepsEverythingRaw(t *testing.T) {
	lake, pool := newLake(t)
	ctx := testCtx(t)

	sent := []Envelope{
		event("evt-1", "BookingCreated", at(9), `{"booking_id":"B-1","room_type":"deluxe","rooms":2,"rate_plan":"nonrefundable"}`),
		event("evt-2", "PaymentCaptured", at(10), `{"booking_id":"B-1","amount":1500000,"currency":"IDR"}`),
		event("evt-3", "BookingCancelled", at(11), `{"booking_id":"B-1","reason":"guest request"}`),
	}
	in := make(chan Envelope, len(sent))
	for _, e := range sent {
		in <- e
	}
	close(in)

	if err := lake.Forward(ctx, in); err != nil {
		t.Fatalf("Forward: %v", err)
	}
	if n := countRows(t, pool); n != len(sent) {
		t.Fatalf("tabel events berisi %d baris, seharusnya %d: ada event yang tidak tersimpan dan tidak akan pernah bisa diputar ulang", n, len(sent))
	}
	for _, want := range sent {
		got, err := lake.Query(ctx, want.Name, at(0), at(24))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("Query(%s) mengembalikan %d event, seharusnya 1", want.Name, len(got))
		}
		if !sameJSON(t, got[0].Payload, want.Payload) {
			t.Fatalf("payload %s berubah di data lake.\n tersimpan: %s\n dikirim:   %s", want.ID, got[0].Payload, want.Payload)
		}
	}
}

// Query memakai rentang waktu setengah terbuka [from, to) dan hasilnya terurut.
func TestStep3_QueryByNameAndTimeRange(t *testing.T) {
	lake, _ := newLake(t)
	ctx := testCtx(t)

	// Sengaja disimpan tidak berurutan.
	for _, e := range []Envelope{
		event("evt-5", "BookingCreated", at(15), `{"booking_id":"B-5"}`),
		event("evt-2", "BookingCreated", at(10), `{"booking_id":"B-2"}`),
		event("evt-4", "BookingCreated", at(12), `{"booking_id":"B-4"}`),
		event("evt-1", "BookingCreated", at(9), `{"booking_id":"B-1"}`),
		event("evt-3", "BookingCancelled", at(11), `{"booking_id":"B-2"}`),
	} {
		if err := lake.Append(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := lake.Query(ctx, "BookingCreated", at(10), at(15))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"evt-2", "evt-4"}
	if !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("Query(BookingCreated, 10:00, 15:00) = %v, seharusnya %v: from termasuk, to tidak, urut dari yang paling lama", ids(got), want)
	}
}

// Sejarah tidak boleh diubah. Database sendiri yang harus menolaknya, bukan
// sekadar kesepakatan antar-developer.
func TestStep4_HistoryCannotBeRewritten(t *testing.T) {
	lake, pool := newLake(t)
	ctx := testCtx(t)

	if err := lake.Append(ctx, event("evt-1", "BookingCreated", at(9), `{"booking_id":"B-1","rooms":2}`)); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `UPDATE events SET payload = '{"booking_id":"B-1","rooms":99}' WHERE event_id = 'evt-1'`); err == nil {
		t.Fatalf("UPDATE pada tabel events berhasil, seharusnya ditolak database")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM events WHERE event_id = 'evt-1'`); err == nil {
		t.Fatalf("DELETE pada tabel events berhasil, seharusnya ditolak database")
	}

	got, err := lake.Query(ctx, "BookingCreated", at(0), at(24))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !sameJSON(t, got[0].Payload, json.RawMessage(`{"booking_id":"B-1","rooms":2}`)) {
		t.Fatalf("isi data lake berubah: %v", got)
	}
}
