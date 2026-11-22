package lab10

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
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
	schema := fmt.Sprintf("lab10_%d_%d", time.Now().UnixNano(), rand.IntN(1_000_000))
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
	return ctx, pool
}

func count(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

type outboxState struct {
	status    string
	attempts  int
	lastError *string
	sent      bool
}

// outboxFor mengambil baris outbox milik satu booking.
func outboxFor(t *testing.T, ctx context.Context, pool *pgxpool.Pool, bookingID string) outboxState {
	t.Helper()
	var s outboxState
	err := pool.QueryRow(ctx, `
		SELECT status, attempts, last_error, sent_at IS NOT NULL
		FROM outbox WHERE payload->>'booking_id' = $1`, bookingID).
		Scan(&s.status, &s.attempts, &s.lastError, &s.sent)
	if err != nil {
		t.Fatalf("baris outbox untuk %s tidak bisa dibaca: %v", bookingID, err)
	}
	return s
}

func bookingIDOf(t *testing.T, m Message) string {
	t.Helper()
	var e BookingCreated
	if err := json.Unmarshal(m.Payload, &e); err != nil {
		t.Fatalf("payload message %q bukan BookingCreated: %v", m.ID, err)
	}
	return e.BookingID
}

func mustCreate(t *testing.T, ctx context.Context, svc *BookingService, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if err := svc.CreateBooking(ctx, Booking{ID: id, RoomType: "deluxe", Rooms: 2}); err != nil {
			t.Fatalf("CreateBooking(%s): %v", id, err)
		}
	}
}

func mustRunOnce(t *testing.T, ctx context.Context, f *Forwarder) int {
	t.Helper()
	n, err := f.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	return n
}

// Dunia ideal: broker sehat. Booking tersimpan dan satu event sampai di broker.
func TestStep0_HappyPath(t *testing.T) {
	ctx, pool := setupDB(t)
	pub := &FakePublisher{}
	svc := NewBookingService(pool, pub)
	fwd := NewForwarder(pool, pub)

	mustCreate(t, ctx, svc, "B-1")
	mustRunOnce(t, ctx, fwd)

	if n := count(t, ctx, pool, `SELECT count(*) FROM bookings WHERE id = 'B-1'`); n != 1 {
		t.Fatalf("booking B-1 di database = %d baris, seharusnya 1", n)
	}
	got := pub.Published()
	if len(got) != 1 {
		t.Fatalf("message di broker = %d, seharusnya 1", len(got))
	}
	if got[0].Topic != TopicBookingCreated || bookingIDOf(t, got[0]) != "B-1" {
		t.Fatalf("message salah: topic %q, booking %q", got[0].Topic, bookingIDOf(t, got[0]))
	}
}

// Broker mati tepat saat booking dibuat. Booking tidak boleh ikut gagal, dan
// event-nya tidak boleh hilang: ia harus menunggu di outbox.
func TestStep1_BrokerDownDuringBooking(t *testing.T) {
	ctx, pool := setupDB(t)
	pub := &FakePublisher{}
	svc := NewBookingService(pool, pub)
	pub.FailNext(1)

	if err := svc.CreateBooking(ctx, Booking{ID: "B-1", RoomType: "deluxe", Rooms: 2}); err != nil {
		t.Fatalf("CreateBooking gagal karena broker mati (%v): membuat booking tidak boleh bergantung pada broker", err)
	}
	if n := count(t, ctx, pool, `SELECT count(*) FROM bookings WHERE id = 'B-1'`); n != 1 {
		t.Fatalf("booking B-1 di database = %d baris, seharusnya 1", n)
	}
	if n := count(t, ctx, pool, `SELECT count(*) FROM outbox WHERE status = 'PENDING' AND topic = $1 AND payload->>'booking_id' = 'B-1'`, TopicBookingCreated); n != 1 {
		t.Fatalf("baris outbox PENDING untuk B-1 = %d, seharusnya 1: event-nya hilang", n)
	}
	if n := pub.Calls(); n != 0 {
		t.Fatalf("CreateBooking memanggil Publish %d kali, seharusnya 0: yang berbicara dengan broker hanya forwarder", n)
	}
}

// Booking dan baris outbox harus tersimpan bersama atau tidak sama sekali.
func TestStep2_AllOrNothing(t *testing.T) {
	ctx, pool := setupDB(t)
	svc := NewBookingService(pool, &FakePublisher{})
	mustCreate(t, ctx, svc, "B-1")

	// Kasus A: booking kedua dengan ID yang sama ditolak primary key.
	// Tidak boleh ada event untuk booking yang tidak pernah tersimpan.
	if err := svc.CreateBooking(ctx, Booking{ID: "B-1", RoomType: "deluxe", Rooms: 5}); err == nil {
		t.Fatal("CreateBooking dengan ID duplikat seharusnya error")
	}
	if n := count(t, ctx, pool, `SELECT count(*) FROM outbox`); n != 1 {
		t.Fatalf("baris outbox = %d, seharusnya 1 (hanya milik B-1 yang berhasil): booking yang gagal disimpan tidak boleh punya event, dan yang berhasil harus punya", n)
	}

	// Kasus B: insert ke outbox gagal (trigger meniru kerusakan).
	// Tidak boleh ada booking tanpa event.
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION outbox_broken() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'outbox rusak (simulasi)'; END $$;
		CREATE TRIGGER outbox_broken BEFORE INSERT ON outbox
		FOR EACH ROW EXECUTE FUNCTION outbox_broken();`); err != nil {
		t.Fatalf("pasang trigger: %v", err)
	}
	if err := svc.CreateBooking(ctx, Booking{ID: "B-2", RoomType: "deluxe", Rooms: 2}); err == nil {
		t.Fatal("CreateBooking seharusnya error ketika baris outbox tidak bisa ditulis")
	}
	if n := count(t, ctx, pool, `SELECT count(*) FROM bookings WHERE id = 'B-2'`); n != 0 {
		t.Fatalf("booking B-2 tersimpan (%d baris) padahal event-nya gagal ditulis: keduanya harus dalam satu transaction", n)
	}
}

// Forwarder memindahkan baris PENDING ke broker, urut, lalu menandainya SENT.
func TestStep3_ForwarderPublishesAndMarksSent(t *testing.T) {
	ctx, pool := setupDB(t)
	pub := &FakePublisher{}
	svc := NewBookingService(pool, pub)
	fwd := NewForwarder(pool, pub)
	mustCreate(t, ctx, svc, "B-1", "B-2", "B-3")

	if n := mustRunOnce(t, ctx, fwd); n != 3 {
		t.Fatalf("RunOnce melaporkan %d message, seharusnya 3", n)
	}
	got := pub.Published()
	if len(got) != 3 {
		t.Fatalf("message di broker = %d, seharusnya 3", len(got))
	}
	for i, want := range []string{"B-1", "B-2", "B-3"} {
		if id := bookingIDOf(t, got[i]); id != want {
			t.Fatalf("message ke-%d membawa %s, seharusnya %s: urutkan berdasarkan id outbox", i+1, id, want)
		}
		if s := outboxFor(t, ctx, pool, want); s.status != StatusSent || !s.sent {
			t.Fatalf("outbox %s: status %s, sent_at terisi = %v; seharusnya SENT dan terisi", want, s.status, s.sent)
		}
	}

	if n := mustRunOnce(t, ctx, fwd); n != 0 {
		t.Fatalf("RunOnce kedua melaporkan %d message, seharusnya 0", n)
	}
	if n := len(pub.Published()); n != 3 {
		t.Fatalf("message di broker = %d setelah RunOnce kedua, seharusnya tetap 3: baris SENT terkirim lagi", n)
	}
}

// Satu baris selalu gagal di-publish. Ia tidak boleh menahan baris lain, dan
// setelah MaxAttempts ia harus berhenti dicoba.
func TestStep4_FailedPublishCountsAttemptsThenDead(t *testing.T) {
	ctx, pool := setupDB(t)
	pub := &FakePublisher{}
	svc := NewBookingService(pool, pub)
	fwd := NewForwarder(pool, pub)
	fwd.MaxAttempts = 3
	mustCreate(t, ctx, svc, "B-1", "B-poison", "B-3")
	pub.FailWhen(func(m Message) bool { return strings.Contains(string(m.Payload), "B-poison") })

	if n := mustRunOnce(t, ctx, fwd); n != 2 {
		t.Fatalf("RunOnce pertama melaporkan %d message, seharusnya 2: baris yang gagal menahan baris lain", n)
	}
	s := outboxFor(t, ctx, pool, "B-poison")
	if s.status != StatusPending || s.attempts != 1 {
		t.Fatalf("B-poison setelah 1 kegagalan: status %s, attempts %d; seharusnya PENDING dan 1", s.status, s.attempts)
	}
	if s.lastError == nil || *s.lastError == "" {
		t.Fatal("last_error B-poison kosong: simpan pesan error supaya bisa diselidiki")
	}

	mustRunOnce(t, ctx, fwd)
	mustRunOnce(t, ctx, fwd)
	s = outboxFor(t, ctx, pool, "B-poison")
	if s.status != StatusDead || s.attempts != 3 {
		t.Fatalf("B-poison setelah 3 kegagalan: status %s, attempts %d; seharusnya DEAD dan 3", s.status, s.attempts)
	}

	calls := pub.Calls()
	if n := mustRunOnce(t, ctx, fwd); n != 0 {
		t.Fatalf("RunOnce setelah DEAD melaporkan %d message, seharusnya 0", n)
	}
	if pub.Calls() != calls {
		t.Fatal("baris DEAD masih dicoba: forwarder hanya boleh mengambil baris PENDING")
	}
}

// Message sampai di broker, tetapi forwarder menerima error. Ia tidak punya
// pilihan selain mengirim lagi. Kedua kiriman harus membawa ID yang sama supaya
// consumer bisa mengenalinya sebagai duplikat.
func TestStep4_LostAckCausesDuplicateWithSameID(t *testing.T) {
	ctx, pool := setupDB(t)
	pub := &FakePublisher{}
	svc := NewBookingService(pool, pub)
	fwd := NewForwarder(pool, pub)
	mustCreate(t, ctx, svc, "B-1")
	pub.LoseAckNext(1)

	if n := mustRunOnce(t, ctx, fwd); n != 0 {
		t.Fatalf("RunOnce melaporkan %d message padahal Publish return error, seharusnya 0", n)
	}
	if s := outboxFor(t, ctx, pool, "B-1"); s.status != StatusPending {
		t.Fatalf("status %s setelah Publish return error, seharusnya PENDING", s.status)
	}
	if n := mustRunOnce(t, ctx, fwd); n != 1 {
		t.Fatalf("RunOnce kedua melaporkan %d message, seharusnya 1", n)
	}

	got := pub.Published()
	if len(got) != 2 {
		t.Fatalf("message di broker = %d, seharusnya 2 (duplikat memang terjadi di sini)", len(got))
	}
	if got[0].ID == "" || got[0].ID != got[1].ID {
		t.Fatalf("ID kedua kiriman: %q dan %q; seharusnya sama dan tidak kosong (pakai id baris outbox)", got[0].ID, got[1].ID)
	}
	if s := outboxFor(t, ctx, pool, "B-1"); s.status != StatusSent {
		t.Fatalf("status akhir %s, seharusnya SENT", s.status)
	}
}

// Dua forwarder berjalan bersamaan, seperti dua replica service. Tanpa
// kegagalan, setiap baris harus sampai di broker tepat sekali.
func TestStep5_TwoForwardersDoNotDoublePublish(t *testing.T) {
	ctx, pool := setupDB(t)
	pub := &FakePublisher{Delay: 5 * time.Millisecond}
	svc := NewBookingService(pool, pub)
	const total = 20
	for i := range total {
		mustCreate(t, ctx, svc, fmt.Sprintf("B-%02d", i))
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	sent := make([]int, 2)
	errs := make([]error, 2)
	for i := range 2 {
		fwd := NewForwarder(pool, pub)
		fwd.BatchSize = 10
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				n, err := fwd.RunOnce(ctx)
				if err != nil {
					errs[i] = err
					return
				}
				if n == 0 {
					return
				}
				sent[i] += n
			}
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("forwarder %d: %v", i+1, err)
		}
	}
	t.Logf("forwarder 1 mengirim %d, forwarder 2 mengirim %d", sent[0], sent[1])
	if sent[0]+sent[1] != total {
		t.Fatalf("kedua forwarder melaporkan %d message, seharusnya %d", sent[0]+sent[1], total)
	}

	seen := map[string]int{}
	for _, m := range pub.Published() {
		seen[bookingIDOf(t, m)]++
	}
	if len(seen) != total {
		t.Fatalf("booking yang sampai di broker = %d, seharusnya %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("%s sampai di broker %d kali: dua forwarder mengambil baris yang sama", id, n)
		}
	}
	if n := count(t, ctx, pool, `SELECT count(*) FROM outbox WHERE status <> 'SENT'`); n != 0 {
		t.Fatalf("masih ada %d baris outbox yang belum SENT", n)
	}
}
