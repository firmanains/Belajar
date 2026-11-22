package proyek_test

// File ini sudah jadi. Jangan diubah.

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"belajar/eventdriven/proyek/internal/event"
	"belajar/eventdriven/proyek/internal/inbox"
)

const (
	write = "/admin/availability" // write model: sumber kebenaran
	read  = "/tape-chart"         // read model: dibangun dari event
)

// Dunia ideal: setiap revision datang tepat sekali dan berurutan.
// Test ini sudah lulus di kode awal.
func TestStep0_WebhookAcceptsRevision(t *testing.T) {
	h := start(t)

	if code, _ := h.do(http.MethodPost, "/webhooks/booking-revisions", rev("B-1", 1, "new", 2, "2026-12-10", "2026-12-12"), nil); code != http.StatusUnauthorized {
		t.Fatalf("tanpa secret: status %d, seharusnya 401", code)
	}
	if code, _ := h.post(rev("B-1", 1, "new", 0, "2026-12-10", "2026-12-12")); code != http.StatusBadRequest {
		t.Fatalf("rooms = 0: status %d, seharusnya 400", code)
	}

	h.mustPost(rev("B-1", 1, "new", 2, "2026-12-10", "2026-12-12"))

	for date, want := range map[string]int{"2026-12-10": 2, "2026-12-11": 2, "2026-12-12": 0} {
		if got := h.booked(write, "deluxe", date); got != want {
			t.Fatalf("deluxe terpakai pada %s = %d, seharusnya %d", date, got, want)
		}
	}
	if got := h.booked(write, "suite", "2026-12-10"); got != 0 {
		t.Fatalf("suite terpakai = %d, seharusnya 0", got)
	}
	if code := h.get("/healthz", nil); code != http.StatusOK {
		t.Fatalf("GET /healthz: status %d", code)
	}
}

// Channel manager tidak menerima response (timeout), lalu mengirim revision
// yang sama sekali lagi.
func TestStep1_DuplicateRevisionIsHarmless(t *testing.T) {
	h := start(t)
	r := rev("B-1", 1, "new", 2, "2026-12-10", "2026-12-12")

	h.mustPost(r)
	result := h.mustPost(r)

	if got := h.booked(write, "deluxe", "2026-12-10"); got != 2 {
		t.Fatalf("kamar terpakai = %d, seharusnya 2: satu revision terhitung dua kali", got)
	}
	if result != string(inbox.Duplicate) {
		t.Fatalf("result kiriman kedua = %q, seharusnya %q", result, inbox.Duplicate)
	}
}

// Revision bisa mengubah booking, membatalkannya, dan datang tidak berurutan.
func TestStep2_StaleRevisionIsIgnored(t *testing.T) {
	h := start(t)

	t.Run("modified menggantikan state lama", func(t *testing.T) {
		h.mustPost(rev("B-1", 1, "new", 2, "2026-12-10", "2026-12-12"))
		h.mustPost(rev("B-1", 2, "modified", 3, "2026-12-11", "2026-12-13"))
		for date, want := range map[string]int{"2026-12-10": 0, "2026-12-11": 3, "2026-12-12": 3} {
			if got := h.booked(write, "deluxe", date); got != want {
				t.Fatalf("terpakai pada %s = %d, seharusnya %d: pemakaian dari revision 1 belum dilepas", date, got, want)
			}
		}
	})

	t.Run("cancelled melepas kamar", func(t *testing.T) {
		h.mustPost(rev("B-2", 1, "new", 2, "2027-01-10", "2027-01-12"))
		h.mustPost(rev("B-2", 2, "cancelled", 2, "2027-01-10", "2027-01-12"))
		if got := h.booked(write, "deluxe", "2027-01-10"); got != 0 {
			t.Fatalf("terpakai setelah cancel = %d, seharusnya 0", got)
		}
	})

	t.Run("revision lama yang datang terlambat diabaikan", func(t *testing.T) {
		h.mustPost(rev("B-3", 2, "modified", 3, "2027-02-10", "2027-02-12"))
		result := h.mustPost(rev("B-3", 1, "new", 1, "2027-02-10", "2027-02-12"))
		if got := h.booked(write, "deluxe", "2027-02-10"); got != 3 {
			t.Fatalf("terpakai = %d, seharusnya 3 (state revision 2): revision 1 yang terlambat menimpa state yang lebih baru", got)
		}
		if result != string(inbox.Stale) {
			t.Fatalf("result revision terlambat = %q, seharusnya %q", result, inbox.Stale)
		}
	})

	t.Run("revision yang datang bersamaan", func(t *testing.T) {
		// Dua belas revision dari booking yang sama tiba pada saat yang sama.
		// Apa pun urutan prosesnya, state akhir harus milik revision 12.
		const n = 12
		var wg sync.WaitGroup
		for i := 1; i <= n; i++ {
			wg.Go(func() { h.post(rev("B-4", i, "modified", 1+i%4, "2027-03-10", "2027-03-12")) })
		}
		wg.Wait()
		if got, want := h.booked(write, "deluxe", "2027-03-10"), 1+n%4; got != want {
			t.Fatalf("terpakai = %d, seharusnya %d (state revision %d): revision yang bersamaan saling menimpa", got, want, n)
		}
	})
}

// Booking dan event-nya harus tersimpan bersama atau gagal bersama.
func TestStep3_OutboxWrittenAtomically(t *testing.T) {
	h := start(t)

	h.mustPost(rev("B-1", 1, "new", 2, "2026-12-10", "2026-12-12"))
	if got := h.outbox().Total; got != 1 {
		t.Fatalf("baris outbox = %d, seharusnya 1: booking tersimpan tanpa event", got)
	}

	// Insert ke outbox gagal satu kali, tepat setelah booking ditulis.
	h.faults.OutboxInsert.Store(1)
	r := rev("B-2", 1, "new", 2, "2027-01-10", "2027-01-12")
	if code, _ := h.post(r); code != http.StatusInternalServerError {
		t.Fatalf("status = %d, seharusnya 500 supaya channel manager mengirim ulang", code)
	}
	if got := h.booked(write, "deluxe", "2027-01-10"); got != 0 {
		t.Fatalf("terpakai = %d, seharusnya 0: booking tersimpan padahal event-nya gagal ditulis", got)
	}
	if got := h.outbox().Total; got != 1 {
		t.Fatalf("baris outbox = %d, seharusnya tetap 1", got)
	}

	// Channel manager mengirim ulang revision yang sama.
	if result := h.mustPost(r); result != string(inbox.Applied) {
		t.Fatalf("result kiriman ulang = %q, seharusnya %q: catatan inbox dari percobaan yang gagal masih tertinggal", result, inbox.Applied)
	}
	if got := h.booked(write, "deluxe", "2027-01-10"); got != 2 {
		t.Fatalf("terpakai setelah kiriman ulang = %d, seharusnya 2", got)
	}
	if got := h.outbox().Total; got != 2 {
		t.Fatalf("baris outbox = %d, seharusnya 2", got)
	}
}

// Event di outbox harus sampai ke Kafka walau broker sempat tidak terjangkau.
func TestStep4_ForwarderDeliversAtLeastOnce(t *testing.T) {
	h := start(t)

	h.mustPost(rev("B-1", 1, "new", 2, "2026-12-10", "2026-12-12"))
	h.mustPost(rev("B-1", 2, "modified", 3, "2026-12-10", "2026-12-12"))
	h.mustPost(rev("B-2", 1, "new", 1, "2026-12-10", "2026-12-12"))

	eventually(t, "tiga event sampai di topic Kafka", func() bool { return len(h.events.records()) >= 3 })

	for _, r := range h.events.records() {
		env, err := event.Decode(r.Value)
		if err != nil {
			t.Fatalf("value record bukan envelope yang valid: %v", err)
		}
		if string(r.Key) != env.Booking.BookingID {
			t.Fatalf("key record = %q, seharusnya booking ID %q", r.Key, env.Booking.BookingID)
		}
		if header(r, event.HeaderEventName) != env.EventName || header(r, event.HeaderEventID) != env.EventID {
			t.Fatalf("header %s/%s tidak sama dengan isi envelope", event.HeaderEventName, event.HeaderEventID)
		}
		if header(r, event.HeaderCorrelationID) == "" {
			t.Fatalf("header %s kosong", event.HeaderCorrelationID)
		}
	}
	b1 := h.events.forBooking("B-1")
	if len(b1) < 2 || b1[0].Partition != b1[1].Partition {
		t.Fatalf("dua event B-1 harus berada di partition yang sama")
	}
	if first, second := header(b1[0], event.HeaderEventName), header(b1[1], event.HeaderEventName); first != event.BookingCreated || second != event.BookingModified {
		t.Fatalf("urutan event B-1 = %s lalu %s, seharusnya %s lalu %s", first, second, event.BookingCreated, event.BookingModified)
	}

	// Broker menolak tiga publish berikutnya.
	h.faults.Publish.Store(3)
	h.mustPost(rev("B-3", 1, "new", 1, "2026-12-10", "2026-12-12"))
	eventually(t, "event B-3 sampai setelah broker pulih", func() bool { return len(h.events.forBooking("B-3")) >= 1 })
	eventually(t, "outbox kosong (pending = 0)", func() bool { return h.outbox().Pending == 0 })

	ids := map[string]bool{}
	for _, r := range h.events.records() {
		ids[header(r, event.HeaderEventID)] = true
	}
	if len(ids) != 4 {
		t.Fatalf("event berbeda di Kafka = %d, seharusnya 4", len(ids))
	}
}

// Read model harus benar walau event datang dobel, terlambat, atau rusak.
func TestStep5_TapeChartSurvivesDuplicatesAndDisorder(t *testing.T) {
	h := start(t)

	// Jalur normal: webhook -> outbox -> Kafka -> projector -> HTTP.
	h.mustPost(rev("B-1", 1, "new", 2, "2026-12-10", "2026-12-12"))
	eventually(t, "B-1 muncul di read model", func() bool { _, code := h.booking("B-1"); return code == http.StatusOK })
	if got := h.booked(read, "deluxe", "2026-12-10"); got != 2 {
		t.Fatalf("tape chart 2026-12-10 = %d, seharusnya 2", got)
	}
	h.mustPost(rev("B-1", 2, "cancelled", 2, "2026-12-10", "2026-12-12"))
	eventually(t, "B-1 berstatus cancelled di read model", func() bool { b, _ := h.booking("B-1"); return b.Status == event.StatusCancelled })
	if got := h.booked(read, "deluxe", "2026-12-10"); got != 0 {
		t.Fatalf("tape chart setelah cancel = %d, seharusnya 0", got)
	}

	// Jalur buruk, ditulis langsung ke Kafka. Semua record memakai key yang
	// sama supaya masuk satu partition dan diproses sesuai urutan di bawah.
	booking := func(id string, revision, rooms int, checkIn, checkOut string) event.Booking {
		return event.Booking{BookingID: id, Revision: revision, Status: event.StatusConfirmed, RoomType: "suite",
			CheckIn: checkIn, CheckOut: checkOut, Rooms: rooms, GuestEmail: "tamu@example.com"}
	}
	created := event.New(event.BookingCreated, "c-1", booking("B-9", 1, 2, "2027-01-10", "2027-01-12"))
	rev3 := event.New(event.BookingModified, "c-3", booking("B-9", 3, 1, "2027-01-20", "2027-01-21"))
	rev2 := event.New(event.BookingModified, "c-2", booking("B-9", 2, 4, "2027-01-15", "2027-01-17"))

	h.produceEvent("B-9", created)
	h.produceEvent("B-9", created)                      // duplicate
	h.produceEvent("B-9", rev3)                         // revision 3 mendahului revision 2
	h.produceRaw("B-9", []byte("{ini bukan json"), nil) // message rusak
	h.produceEvent("B-9", rev2)                         // revision 2 datang terlambat
	h.produceEvent("B-9", rev3)                         // duplicate
	// Penanda: begitu booking ini terlihat, semua record di atas sudah diproses.
	h.produceEvent("B-9", event.New(event.BookingCreated, "c-end", booking("B-penanda", 1, 1, "2027-06-01", "2027-06-02")))

	eventually(t, "projector melewati message rusak dan sampai ke record penanda", func() bool {
		_, code := h.booking("B-penanda")
		return code == http.StatusOK
	})

	if b, _ := h.booking("B-9"); b.Revision != 3 {
		t.Fatalf("revision B-9 di read model = %d, seharusnya 3: revision 2 yang terlambat menimpa revision 3", b.Revision)
	}
	for date, want := range map[string]int{"2027-01-10": 0, "2027-01-11": 0, "2027-01-15": 0, "2027-01-16": 0, "2027-01-20": 1} {
		if got := h.booked(read, "suite", date); got != want {
			t.Fatalf("tape chart suite %s = %d, seharusnya %d", date, got, want)
		}
	}
	if _, code := h.booking("tidak-ada"); code != http.StatusNotFound {
		t.Fatalf("GET booking yang tidak ada: status %d, seharusnya 404", code)
	}
}

// Email yang gagal dicoba ulang. Message yang tidak tertolong disingkirkan ke
// dead letter topic supaya message di belakangnya tetap mengalir.
func TestStep6_EmailRetryThenDeadLetter(t *testing.T) {
	h := start(t)
	h.sender.set(func() {
		h.sender.failures["B-1"] = 2  // gagal dua kali, lalu berhasil
		h.sender.failures["B-2"] = -1 // selalu gagal
	})

	h.mustPost(rev("B-1", 1, "new", 1, "2026-12-10", "2026-12-12"))
	eventually(t, "email B-1 terkirim setelah retry", func() bool { return h.sender.sentFor("B-1") == 1 })
	if got := h.sender.attemptsFor("B-1"); got != 3 {
		t.Fatalf("percobaan kirim B-1 = %d, seharusnya 3 (dua gagal, satu berhasil)", got)
	}

	h.mustPost(rev("B-2", 1, "new", 1, "2026-12-10", "2026-12-12"))
	h.mustPost(rev("B-3", 1, "new", 1, "2026-12-10", "2026-12-12"))
	eventually(t, "message B-2 masuk dead letter topic", func() bool { return len(h.dlq.forBooking("B-2")) == 1 })
	if reason := header(h.dlq.forBooking("B-2")[0], event.HeaderDLQReason); reason == "" {
		t.Fatalf("header %s kosong: tanpa alasan, isi dead letter topic tidak bisa didiagnosis", event.HeaderDLQReason)
	}
	if got := h.sender.attemptsFor("B-2"); got != h.cfg.EmailMaxAttempts {
		t.Fatalf("percobaan kirim B-2 = %d, seharusnya %d", got, h.cfg.EmailMaxAttempts)
	}
	eventually(t, "email B-3 terkirim walau B-2 gagal terus", func() bool { return h.sender.sentFor("B-3") == 1 })

	// Message rusak: mencoba ulang tidak ada gunanya.
	garbage := []byte("{ini bukan json")
	h.produceRaw("rusak", garbage, nil)
	eventually(t, "message rusak masuk dead letter topic", func() bool { return len(h.dlq.forBooking("rusak")) == 1 })
	rusak := h.dlq.forBooking("rusak")[0]
	if !bytes.Equal(rusak.Value, garbage) || !strings.Contains(header(rusak, event.HeaderDLQReason), "malformed") {
		t.Fatalf("message rusak harus disimpan utuh dengan alasan yang memuat kata malformed, dapat alasan %q", header(rusak, event.HeaderDLQReason))
	}

	// Event yang sama dua kali, lalu event yang bukan BookingCreated.
	b4 := event.Booking{BookingID: "B-4", Revision: 1, Status: event.StatusConfirmed, RoomType: "deluxe",
		CheckIn: "2026-12-10", CheckOut: "2026-12-12", Rooms: 1, GuestEmail: "tamu@example.com"}
	created := event.New(event.BookingCreated, "c-4", b4)
	b4.Revision = 2
	h.produceEvent("B-4", created)
	h.produceEvent("B-4", created)
	h.produceEvent("B-4", event.New(event.BookingModified, "c-4", b4))
	b5 := b4
	b5.BookingID, b5.Revision = "B-5", 1
	h.produceEvent("B-4", event.New(event.BookingCreated, "c-5", b5)) // penanda, key sama supaya diproses paling akhir
	eventually(t, "email B-5 terkirim", func() bool { return h.sender.sentFor("B-5") == 1 })
	if got := h.sender.sentFor("B-4"); got != 1 {
		t.Fatalf("email B-4 terkirim %d kali, seharusnya 1: event dobel dan BookingModified tidak boleh mengirim email lagi", got)
	}
}

// Sistem asinkron gagal tanpa suara. Metrics dan correlation ID membuatnya terlihat.
func TestStep7_MetricsAndCorrelationID(t *testing.T) {
	h := start(t)
	const corr = "corr-dari-channel-manager"

	h.post(rev("B-1", 1, "new", 1, "2026-12-10", "2026-12-12"), inbox.HeaderCorrelationID, corr)
	eventually(t, "event B-1 sampai di Kafka", func() bool { return len(h.events.forBooking("B-1")) >= 1 })
	if got := header(h.events.forBooking("B-1")[0], event.HeaderCorrelationID); got != corr {
		t.Fatalf("header %s di Kafka = %q, seharusnya %q", event.HeaderCorrelationID, got, corr)
	}
	for _, consumer := range []string{"projector", "email"} {
		eventually(t, "log consumer "+consumer+" memuat correlation_id dari webhook", func() bool {
			return h.logs.has(map[string]string{"consumer": consumer, "correlation_id": corr})
		})
		series := `channelsync_messages_processed_total{consumer="` + consumer + `",outcome="ok"}`
		eventually(t, "metric "+series+" minimal 1", func() bool { v, ok := h.metric(series); return ok && v >= 1 })
	}

	// Broker tidak terjangkau: outbox menumpuk dan harus terlihat di /metrics.
	h.faults.Publish.Store(1 << 40)
	h.mustPost(rev("B-2", 1, "new", 1, "2026-12-10", "2026-12-12"))
	eventually(t, "channelsync_outbox_pending = 1 dan umur baris tertua > 0", func() bool {
		pending, ok1 := h.metric("channelsync_outbox_pending")
		age, ok2 := h.metric("channelsync_outbox_oldest_pending_age_seconds")
		return ok1 && ok2 && pending == 1 && age > 0
	})
	h.faults.Publish.Store(0)
	eventually(t, "channelsync_outbox_pending kembali 0", func() bool { v, ok := h.metric("channelsync_outbox_pending"); return ok && v == 0 })

	// Handler yang gagal harus terhitung. Email B-3 selalu gagal dan dead
	// letter topic tidak bisa ditulisi, jadi handler mengembalikan error.
	h.sender.set(func() { h.sender.failures["B-3"] = -1 })
	h.faults.Publish.Store(1 << 40)
	h.produceEvent("B-3", event.New(event.BookingCreated, "c-3", event.Booking{BookingID: "B-3", Revision: 1,
		Status: event.StatusConfirmed, RoomType: "deluxe", CheckIn: "2026-12-10", CheckOut: "2026-12-12", Rooms: 1, GuestEmail: "tamu@example.com"}))
	const failed = `channelsync_messages_processed_total{consumer="email",outcome="error"}`
	eventually(t, "metric "+failed+" minimal 1", func() bool { v, ok := h.metric(failed); return ok && v >= 1 })
	h.faults.Publish.Store(0)
}

// Saat service berhenti (deploy, restart), pekerjaan yang sedang berjalan
// tidak boleh hilang dan tidak boleh dianggap selesai padahal belum.
func TestStep8_GracefulShutdownAndCommitAfterSuccess(t *testing.T) {
	t.Run("pekerjaan yang sedang berjalan diselesaikan", func(t *testing.T) {
		h := start(t)
		h.sender.set(func() { h.sender.delay["B-1"] = 500 * time.Millisecond })
		h.mustPost(rev("B-1", 1, "new", 1, "2026-12-10", "2026-12-12"))
		eventually(t, "pengiriman email B-1 dimulai", func() bool { return h.sender.attemptsFor("B-1") == 1 })

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.stop(ctx); err != nil {
			t.Fatalf("shutdown dengan waktu cukup harus bersih, dapat: %v", err)
		}
		if got := h.sender.sentFor("B-1"); got != 1 {
			t.Fatalf("email B-1 terkirim %d kali, seharusnya 1: shutdown membatalkan pengiriman yang sedang berjalan", got)
		}
		if got := h.committed("email"); got != 1 {
			t.Fatalf("offset consumer email yang di-commit = %d, seharusnya 1", got)
		}
	})

	t.Run("pekerjaan yang belum selesai dikirim ulang setelah restart", func(t *testing.T) {
		h := start(t)
		h.sender.set(func() { h.sender.block["B-1"] = true }) // SMTP menggantung
		h.mustPost(rev("B-1", 1, "new", 1, "2026-12-10", "2026-12-12"))
		eventually(t, "pengiriman email B-1 dimulai", func() bool { return h.sender.attemptsFor("B-1") == 1 })

		// Waktu shutdown habis sebelum email terkirim.
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		_ = h.stop(ctx)
		if got := h.committed("email"); got != 0 {
			t.Fatalf("offset consumer email yang di-commit = %d, seharusnya 0: offset di-commit padahal email belum terkirim, jadi email ini hilang", got)
		}
		if got := len(h.dlq.forBooking("B-1")); got != 0 {
			t.Fatalf("B-1 masuk dead letter topic: shutdown bukan alasan untuk membuang message")
		}

		// Service dijalankan lagi dengan SMTP yang sehat.
		h2 := startWith(t, h.cfg, newFakeSender())
		eventually(t, "email B-1 terkirim oleh instance baru", func() bool { return h2.sender.sentFor("B-1") == 1 })
	})
}
