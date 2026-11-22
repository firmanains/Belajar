package lab20

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// File ini sudah jadi. Jangan diubah.

const stepTimeout = 15 * time.Minute

// fakeClock adalah jam yang hanya maju kalau test memajukannya.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

// fakeBus mencatat command yang dikirim dan bisa disuruh gagal.
type fakeBus struct {
	sent     []Command
	failNext int
}

var errBusDown = errors.New("command bus tidak terjangkau")

func (b *fakeBus) Send(ctx context.Context, cmd Command) error {
	if b.failNext > 0 {
		b.failNext--
		return errBusDown
	}
	b.sent = append(b.sent, cmd)
	return nil
}

func (b *fakeBus) names() []CommandName {
	var names []CommandName
	for _, cmd := range b.sent {
		names = append(names, cmd.Name)
	}
	return names
}

func (b *fakeBus) count(name CommandName) int {
	n := 0
	for _, cmd := range b.sent {
		if cmd.Name == name {
			n++
		}
	}
	return n
}

type harness struct {
	t       *testing.T
	clock   *fakeClock
	store   *MemoryStore
	bus     *fakeBus
	manager *Manager
}

func newHarness(t *testing.T) *harness {
	clock := &fakeClock{t: time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)}
	store := NewMemoryStore()
	bus := &fakeBus{}
	return &harness{t: t, clock: clock, store: store, bus: bus, manager: NewManager(store, bus, clock.Now, stepTimeout)}
}

// handle mengirim event satu per satu dan menuntut semuanya diterima.
func (h *harness) handle(bookingID string, names ...EventName) {
	h.t.Helper()
	for _, name := range names {
		evt := Event{Name: name, BookingID: bookingID}
		switch name {
		case BookingRequested:
			evt.RoomType, evt.Rooms, evt.AmountIDR = "deluxe", 2, 1_500_000
		case RoomReservationFailed:
			evt.Reason = "kamar habis"
		case PaymentFailed:
			evt.Reason = "kartu ditolak"
		case ChannelConfirmationFailed:
			evt.Reason = "channel manager menolak"
		}
		if err := h.manager.Handle(context.Background(), evt); err != nil {
			h.t.Fatalf("Handle(%s) gagal: %v", name, err)
		}
	}
}

func (h *harness) saga(bookingID string) Saga {
	h.t.Helper()
	saga, found, err := h.store.Load(context.Background(), bookingID)
	if err != nil || !found {
		h.t.Fatalf("saga %s tidak ditemukan di store (err = %v)", bookingID, err)
	}
	return saga
}

func (h *harness) wantStatus(bookingID string, want Status) {
	h.t.Helper()
	if got := h.saga(bookingID).Status; got != want {
		h.t.Fatalf("status saga %s = %q, seharusnya %q", bookingID, got, want)
	}
}

func (h *harness) wantCommands(want ...CommandName) {
	h.t.Helper()
	if got := h.bus.names(); !slices.Equal(got, want) {
		h.t.Fatalf("command terkirim = %v, seharusnya %v", got, want)
	}
}

func (h *harness) tick() {
	h.t.Helper()
	if err := h.manager.Tick(context.Background()); err != nil {
		h.t.Fatalf("Tick gagal: %v", err)
	}
}

// Saga dimulai: state dibuat dan command pertama dikirim.
func TestStep0_BookingRequestedStartsSaga(t *testing.T) {
	h := newHarness(t)
	h.handle("B-1", BookingRequested)

	h.wantStatus("B-1", StatusReservingRoom)
	h.wantCommands(ReserveRoom)
	if cmd := h.bus.sent[0]; cmd.BookingID != "B-1" || cmd.RoomType != "deluxe" || cmd.Rooms != 2 {
		t.Fatalf("ReserveRoom = %+v, seharusnya untuk B-1, 2 kamar deluxe", cmd)
	}
}

// Jalur bahagia: setiap jawaban dari satu service memicu command ke service
// berikutnya.
func TestStep1_HappyPath(t *testing.T) {
	h := newHarness(t)

	h.handle("B-1", BookingRequested, RoomReserved)
	h.wantStatus("B-1", StatusCapturingPayment)
	h.wantCommands(ReserveRoom, CapturePayment)
	if cmd := h.bus.sent[1]; cmd.BookingID != "B-1" || cmd.AmountIDR != 1_500_000 {
		t.Fatalf("CapturePayment = %+v, seharusnya untuk B-1 sebesar 1500000: jumlahnya diambil dari state saga", cmd)
	}

	h.handle("B-1", PaymentCaptured)
	h.wantStatus("B-1", StatusConfirmingChannel)
	h.wantCommands(ReserveRoom, CapturePayment, ConfirmWithChannel)

	h.handle("B-1", ChannelConfirmed)
	h.wantStatus("B-1", StatusCompleted)
	h.wantCommands(ReserveRoom, CapturePayment, ConfirmWithChannel)
}

// Tidak ada transaction bersama yang bisa di-rollback. Step yang sudah
// berhasil harus dibatalkan dengan command baru.
func TestStep2_FailureTriggersCompensation(t *testing.T) {
	t.Run("kamar habis: tidak ada yang perlu dibatalkan", func(t *testing.T) {
		h := newHarness(t)
		h.handle("B-1", BookingRequested, RoomReservationFailed)

		h.wantStatus("B-1", StatusFailed)
		h.wantCommands(ReserveRoom)
		if got := h.saga("B-1").FailureReason; got != "kamar habis" {
			t.Fatalf("FailureReason = %q, seharusnya %q", got, "kamar habis")
		}
	})

	t.Run("pembayaran gagal: kamar dilepas", func(t *testing.T) {
		h := newHarness(t)
		h.handle("B-1", BookingRequested, RoomReserved, PaymentFailed)

		h.wantCommands(ReserveRoom, CapturePayment, ReleaseRoom)
		// Di antara dua event ini kamar masih tertahan. Compensation bukan
		// rollback: keadaan antara ini terlihat dari luar.
		h.wantStatus("B-1", StatusCompensating)

		h.handle("B-1", RoomReleased)
		h.wantStatus("B-1", StatusFailed)
		if got := h.saga("B-1").FailureReason; got != "kartu ditolak" {
			t.Fatalf("FailureReason = %q, seharusnya %q", got, "kartu ditolak")
		}
	})

	t.Run("channel menolak: uang dikembalikan dan kamar dilepas", func(t *testing.T) {
		h := newHarness(t)
		h.handle("B-1", BookingRequested, RoomReserved, PaymentCaptured, ChannelConfirmationFailed)

		if len(h.bus.sent) != 5 || h.bus.count(RefundPayment) != 1 || h.bus.count(ReleaseRoom) != 1 {
			t.Fatalf("command terkirim = %v, seharusnya tiga command maju lalu satu RefundPayment dan satu ReleaseRoom", h.bus.names())
		}
		for _, cmd := range h.bus.sent {
			if cmd.Name == RefundPayment && cmd.AmountIDR != 1_500_000 {
				t.Fatalf("RefundPayment = %+v, seharusnya sebesar 1500000", cmd)
			}
		}
		h.wantStatus("B-1", StatusCompensating)

		h.handle("B-1", PaymentRefunded)
		h.wantStatus("B-1", StatusCompensating) // kamar belum dilepas

		h.handle("B-1", RoomReleased)
		h.wantStatus("B-1", StatusFailed)
	})
}

// Broker at-least-once: event bisa datang dua kali, dan event yang gagal
// diproses akan dikirim lagi.
func TestStep3_DuplicatesAndRedelivery(t *testing.T) {
	t.Run("event duplikat tidak mengirim command dua kali", func(t *testing.T) {
		h := newHarness(t)
		h.handle("B-1", BookingRequested, BookingRequested)
		h.handle("B-1", RoomReserved, RoomReserved)
		h.wantCommands(ReserveRoom, CapturePayment)

		h.handle("B-1", PaymentFailed, PaymentFailed)
		h.wantCommands(ReserveRoom, CapturePayment, ReleaseRoom)

		h.handle("B-1", RoomReleased, RoomReleased)
		h.wantStatus("B-1", StatusFailed)
		h.wantCommands(ReserveRoom, CapturePayment, ReleaseRoom)
	})

	t.Run("event untuk saga yang tidak dikenal diabaikan", func(t *testing.T) {
		h := newHarness(t)
		h.handle("B-404", PaymentCaptured)
		h.wantCommands()
	})

	t.Run("command gagal terkirim: event yang dikirim ulang harus mengulangnya", func(t *testing.T) {
		h := newHarness(t)
		h.handle("B-1", BookingRequested)

		h.bus.failNext = 1
		err := h.manager.Handle(context.Background(), Event{Name: RoomReserved, BookingID: "B-1"})
		if !errors.Is(err, errBusDown) {
			t.Fatalf("Handle mengembalikan %v, seharusnya error dari command bus supaya broker mengirim ulang event", err)
		}

		// Broker mengirim ulang event yang sama.
		h.handle("B-1", RoomReserved)
		h.wantCommands(ReserveRoom, CapturePayment)
		h.wantStatus("B-1", StatusCapturingPayment)
	})
}

// Payment service tidak pernah menjawab. Saga tidak boleh menahan kamar
// selamanya.
func TestStep4_TimeoutAndLateReply(t *testing.T) {
	h := newHarness(t)
	h.handle("B-done", BookingRequested, RoomReserved, PaymentCaptured, ChannelConfirmed)
	h.handle("B-1", BookingRequested, RoomReserved)
	before := len(h.bus.sent)

	h.clock.Advance(stepTimeout - time.Minute)
	h.handle("B-fresh", BookingRequested)
	before++
	h.tick()
	if len(h.bus.sent) != before {
		t.Fatalf("Tick mengirim %v sebelum timeout tercapai", h.bus.names()[before:])
	}

	h.clock.Advance(time.Minute)
	h.tick()
	h.wantStatus("B-1", StatusCompensating)
	h.wantStatus("B-done", StatusCompleted)
	h.wantStatus("B-fresh", StatusReservingRoom)
	if got := h.bus.sent[before:]; len(got) != 1 || got[0].Name != ReleaseRoom || got[0].BookingID != "B-1" {
		t.Fatalf("command setelah timeout = %+v, seharusnya satu ReleaseRoom untuk B-1", got)
	}
	if reason := h.saga("B-1").FailureReason; !strings.Contains(reason, "timeout") {
		t.Fatalf("FailureReason = %q, seharusnya memuat \"timeout\"", reason)
	}

	h.tick()
	if got := h.bus.count(ReleaseRoom); got != 1 {
		t.Fatalf("ReleaseRoom terkirim %d kali setelah Tick kedua, seharusnya 1", got)
	}

	// Payment service ternyata hanya lambat. Jawabannya datang setelah saga
	// menyerah, dan uang tamu sudah terlanjur diambil.
	h.handle("B-1", PaymentCaptured)
	if got := h.bus.count(RefundPayment); got != 1 {
		t.Fatalf("RefundPayment terkirim %d kali setelah PaymentCaptured yang terlambat, seharusnya 1: uang sudah diambil untuk booking yang batal", got)
	}
	if got := h.bus.count(ConfirmWithChannel); got != 1 {
		t.Fatalf("ConfirmWithChannel terkirim %d kali, seharusnya 1 (milik B-done): saga yang sudah menyerah tidak boleh lanjut", got)
	}

	h.handle("B-1", PaymentCaptured) // duplikat dari jawaban yang terlambat
	if got := h.bus.count(RefundPayment); got != 1 {
		t.Fatalf("RefundPayment terkirim %d kali, seharusnya 1: refund ganda berarti hotel rugi", got)
	}

	h.handle("B-1", RoomReleased)
	h.wantStatus("B-1", StatusCompensating) // refund belum selesai
	h.handle("B-1", PaymentRefunded)
	h.wantStatus("B-1", StatusFailed)
}
