package lab16

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

// newPool membuat schema Postgres dengan nama unik, menjalankan Migrate di
// dalamnya, dan menghapus schema itu saat test selesai.
func newPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("lab16_%d", rand.Uint64())
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

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return pool
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

var start = time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)

// booking membuat event ke-seq. Waktu kejadiannya naik satu menit per seq.
func booking(seq int, name, bookingID, roomType, ratePlan, stayDate string, rooms int) Envelope {
	payload, err := json.Marshal(BookingPayload{
		BookingID: bookingID, RoomType: roomType, RatePlan: ratePlan, StayDate: stayDate, Rooms: rooms,
	})
	if err != nil {
		panic(err)
	}
	return Envelope{
		ID:         fmt.Sprintf("evt-%02d", seq),
		Name:       name,
		OccurredAt: start.Add(time.Duration(seq) * time.Minute),
		Payload:    payload,
	}
}

// appendToLake meniru lake forwarder dari lab 14: setiap event yang lewat
// broker tersimpan di tabel events, apa adanya dan tanpa duplikat.
func appendToLake(t *testing.T, pool *pgxpool.Pool, ev Envelope) {
	t.Helper()
	_, err := pool.Exec(testCtx(t),
		`INSERT INTO events (event_id, name, occurred_at, payload) VALUES ($1, $2, $3, $4::jsonb) ON CONFLICT DO NOTHING`,
		ev.ID, ev.Name, ev.OccurredAt, string(ev.Payload))
	if err != nil {
		t.Fatalf("menyimpan %s ke data lake: %v", ev.ID, err)
	}
}

// seedHistory mengisi keadaan sebelum migrasi dimulai: lima event sudah ada
// di data lake dan sudah diterapkan ke v1. Saat itu v2 belum ada.
func seedHistory(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := testCtx(t)
	history := []Envelope{
		booking(1, BookingCreated, "B-1", "deluxe", "flexible", "2026-12-24", 2),
		booking(2, BookingCreated, "B-2", "deluxe", "nonrefundable", "2026-12-24", 1),
		booking(3, BookingCreated, "B-3", "suite", "flexible", "2026-12-25", 1),
		booking(4, BookingCancelled, "B-2", "deluxe", "nonrefundable", "2026-12-24", 1),
		booking(5, BookingCreated, "B-4", "deluxe", "nonrefundable", "2026-12-24", 3),
	}
	for _, ev := range history {
		appendToLake(t, pool, ev)
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return ApplyV1(ctx, tx, ev) })
		if err != nil {
			t.Fatalf("menerapkan %s ke v1: %v", ev.ID, err)
		}
	}
}

// live meniru satu event baru yang datang dari broker: lake forwarder
// menyimpannya, dan handler read model menerapkannya.
func live(t *testing.T, pool *pgxpool.Pool, ev Envelope) {
	t.Helper()
	appendToLake(t, pool, ev)
	if err := ApplyLive(testCtx(t), pool, ev); err != nil {
		t.Fatalf("ApplyLive(%s): %v", ev.ID, err)
	}
}

func assertV1(t *testing.T, pool *pgxpool.Pool, want map[KeyV1]int) {
	t.Helper()
	got, err := AvailabilityV1(testCtx(t), pool)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read model v1:\n   isi: %v\n harus: %v", got, want)
	}
}

func assertV2(t *testing.T, pool *pgxpool.Pool, want map[KeyV2]int) {
	t.Helper()
	got, err := AvailabilityV2(testCtx(t), pool)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("read model v2:\n   isi: %v\n harus: %v", got, want)
	}
}

// Hasil yang benar setelah lima event sejarah.
var (
	historyV1 = map[KeyV1]int{
		{"deluxe", "2026-12-24"}: 5,
		{"suite", "2026-12-25"}:  1,
	}
	historyV2 = map[KeyV2]int{
		{"deluxe", "flexible", "2026-12-24"}:      2,
		{"deluxe", "nonrefundable", "2026-12-24"}: 3,
		{"suite", "flexible", "2026-12-25"}:       1,
	}
)

// Dunia sebelum migrasi: hanya v1 yang ada, dan event live memperbaruinya.
func TestStep0_V1ServesReads(t *testing.T) {
	pool := newPool(t)
	seedHistory(t, pool)
	assertV1(t, pool, historyV1)

	live(t, pool, booking(6, BookingCreated, "B-5", "suite", "nonrefundable", "2026-12-25", 2))
	assertV1(t, pool, map[KeyV1]int{
		{"deluxe", "2026-12-24"}: 5,
		{"suite", "2026-12-25"}:  3,
	})

	v, err := ActiveVersion(testCtx(t), pool)
	if err != nil {
		t.Fatal(err)
	}
	if v != 1 {
		t.Fatalf("versi aktif = %d, seharusnya 1", v)
	}
}

// v2 butuh angka per rate plan. v1 tidak pernah menyimpannya.
func TestStep1_RebuildFromHistory(t *testing.T) {
	pool := newPool(t)
	seedHistory(t, pool)

	if err := Rebuild(testCtx(t), pool); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	assertV2(t, pool, historyV2)
}

// Rebuild yang mati di tengah dijalankan lagi dari awal. Hasilnya harus sama
// dengan satu kali jalan.
func TestStep2_RebuildIsIdempotent(t *testing.T) {
	pool := newPool(t)
	seedHistory(t, pool)

	for i := 1; i <= 2; i++ {
		if err := Rebuild(testCtx(t), pool); err != nil {
			t.Fatalf("Rebuild ke-%d: %v", i, err)
		}
	}
	assertV2(t, pool, historyV2)
}

// Hotel tidak berhenti menerima booking selama migrasi. Satu event datang
// sebelum Rebuild, satu lagi sesudahnya. Keduanya harus terhitung tepat sekali
// di v2.
func TestStep3_LiveEventsDuringMigration(t *testing.T) {
	pool := newPool(t)
	seedHistory(t, pool)

	live(t, pool, booking(6, BookingCreated, "B-5", "suite", "nonrefundable", "2026-12-25", 2))
	if err := Rebuild(testCtx(t), pool); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	live(t, pool, booking(7, BookingCancelled, "B-1", "deluxe", "flexible", "2026-12-24", 2))

	wantV1 := map[KeyV1]int{
		{"deluxe", "2026-12-24"}: 3,
		{"suite", "2026-12-25"}:  3,
	}
	wantV2 := map[KeyV2]int{
		{"deluxe", "nonrefundable", "2026-12-24"}: 3,
		{"suite", "flexible", "2026-12-25"}:       1,
		{"suite", "nonrefundable", "2026-12-25"}:  2,
	}
	assertV1(t, pool, wantV1)
	assertV2(t, pool, wantV2)

	// Rebuild sekali lagi setelah event live masuk tidak mengubah apa pun.
	if err := Rebuild(testCtx(t), pool); err != nil {
		t.Fatalf("Rebuild ulang: %v", err)
	}
	assertV2(t, pool, wantV2)
}

// Sebelum pembacaan dipindah, v2 harus terbukti cocok dengan v1 per tipe
// kamar dan tanggal, bukan hanya pada total keseluruhan.
func TestStep4_CompareThenSwitch(t *testing.T) {
	pool := newPool(t)
	ctx := testCtx(t)
	seedHistory(t, pool)
	if err := Rebuild(ctx, pool); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// Rusak v2 tanpa mengubah totalnya: kamar suite pindah ke tanggal lain.
	if _, err := pool.Exec(ctx, `UPDATE availability_v2 SET stay_date = '2026-12-31' WHERE room_type = 'suite'`); err != nil {
		t.Fatal(err)
	}

	diffs, err := Compare(ctx, pool)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	want := []Diff{
		{RoomType: "suite", StayDate: "2026-12-25", V1: 1, V2: 0},
		{RoomType: "suite", StayDate: "2026-12-31", V1: 0, V2: 1},
	}
	if !reflect.DeepEqual(diffs, want) {
		t.Fatalf("Compare pada v2 yang rusak:\n   isi: %+v\n harus: %+v (urut tipe kamar lalu tanggal)", diffs, want)
	}
	if err := SwitchToV2(ctx, pool); err == nil {
		t.Fatalf("SwitchToV2 berhasil padahal v2 tidak cocok dengan v1")
	}
	if v, err := ActiveVersion(ctx, pool); err != nil || v != 1 {
		t.Fatalf("versi aktif = %d (err %v), seharusnya tetap 1 setelah switch ditolak", v, err)
	}

	// Read model yang salah tidak diperbaiki baris demi baris: dikosongkan,
	// lalu dibangun ulang.
	if err := ResetV2(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := Rebuild(ctx, pool); err != nil {
		t.Fatalf("Rebuild setelah reset: %v", err)
	}
	diffs, err = Compare(ctx, pool)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(diffs) != 0 {
		t.Fatalf("Compare setelah rebuild bersih menemukan %d perbedaan: %+v", len(diffs), diffs)
	}
	if err := SwitchToV2(ctx, pool); err != nil {
		t.Fatalf("SwitchToV2 ditolak padahal v2 cocok: %v", err)
	}
	if v, err := ActiveVersion(ctx, pool); err != nil || v != 2 {
		t.Fatalf("versi aktif = %d (err %v), seharusnya 2", v, err)
	}
}
