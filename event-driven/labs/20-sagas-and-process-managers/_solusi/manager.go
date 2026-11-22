package lab20

import (
	"context"
	"fmt"
	"time"
)

// Manager adalah process manager untuk saga booking. Ia tidak mengerjakan
// apa pun sendiri: ia menerima event, memperbarui state, dan mengirim command.
type Manager struct {
	store       Store
	bus         CommandBus
	now         func() time.Time
	stepTimeout time.Duration
}

func NewManager(store Store, bus CommandBus, now func() time.Time, stepTimeout time.Duration) *Manager {
	return &Manager{store: store, bus: bus, now: now, stepTimeout: stepTimeout}
}

// Handle memproses satu event. Mengembalikan error berarti event belum
// selesai diproses dan broker akan mengirimnya lagi.
func (m *Manager) Handle(ctx context.Context, evt Event) error {
	saga, found, err := m.store.Load(ctx, evt.BookingID)
	if err != nil {
		return fmt.Errorf("load saga %s: %w", evt.BookingID, err)
	}

	if evt.Name == BookingRequested {
		if found {
			return nil // duplikat: saga ini sudah berjalan
		}
		saga = Saga{
			BookingID:     evt.BookingID,
			Status:        StatusReservingRoom,
			RoomType:      evt.RoomType,
			Rooms:         evt.Rooms,
			AmountIDR:     evt.AmountIDR,
			StepStartedAt: m.now(),
		}
		return m.commit(ctx, saga, []Command{
			{Name: ReserveRoom, BookingID: saga.BookingID, RoomType: saga.RoomType, Rooms: saga.Rooms},
		})
	}
	if !found {
		return nil // event untuk saga yang tidak dikenal
	}

	before := saga
	commands := m.transition(&saga, evt)
	if saga == before {
		return nil // duplikat atau event yang tidak cocok dengan posisi saga
	}
	return m.commit(ctx, saga, commands)
}

// transition adalah state machine-nya: untuk posisi saga sekarang dan event
// yang datang, apa state berikutnya dan command apa yang harus dikirim.
// Event yang tidak cocok dengan posisi saga tidak mengubah apa pun.
func (m *Manager) transition(saga *Saga, evt Event) []Command {
	stopped := saga.Status == StatusCompensating || saga.Status == StatusFailed

	switch evt.Name {
	case RoomReserved:
		switch {
		case saga.Status == StatusReservingRoom:
			saga.RoomReserved = true
			saga.Status = StatusCapturingPayment
			saga.StepStartedAt = m.now()
			return []Command{{Name: CapturePayment, BookingID: saga.BookingID, AmountIDR: saga.AmountIDR}}
		case stopped && !saga.RoomReserved:
			// Jawaban datang setelah saga menyerah: kamar terlanjur ditahan.
			saga.RoomReserved = true
			saga.Status = StatusCompensating
			return []Command{releaseRoom(*saga)}
		}

	case RoomReservationFailed:
		if saga.Status == StatusReservingRoom {
			return compensate(saga, evt.Reason)
		}

	case PaymentCaptured:
		switch {
		case saga.Status == StatusCapturingPayment:
			saga.PaymentCaptured = true
			saga.Status = StatusConfirmingChannel
			saga.StepStartedAt = m.now()
			return []Command{{Name: ConfirmWithChannel, BookingID: saga.BookingID, RoomType: saga.RoomType, Rooms: saga.Rooms}}
		case stopped && !saga.PaymentCaptured:
			// Jawaban datang setelah saga menyerah: uang terlanjur diambil.
			saga.PaymentCaptured = true
			saga.Status = StatusCompensating
			return []Command{refundPayment(*saga)}
		}

	case PaymentFailed:
		if saga.Status == StatusCapturingPayment {
			return compensate(saga, evt.Reason)
		}

	case ChannelConfirmed:
		if saga.Status == StatusConfirmingChannel {
			saga.Status = StatusCompleted
		}

	case ChannelConfirmationFailed:
		if saga.Status == StatusConfirmingChannel {
			return compensate(saga, evt.Reason)
		}

	case RoomReleased:
		if saga.Status == StatusCompensating && saga.RoomReserved {
			saga.RoomReleased = true
			settle(saga)
		}

	case PaymentRefunded:
		if saga.Status == StatusCompensating && saga.PaymentCaptured {
			saga.PaymentRefunded = true
			settle(saga)
		}
	}
	return nil
}

// compensate menghentikan saga dan mengembalikan command untuk membatalkan
// setiap step yang sudah berhasil.
func compensate(saga *Saga, reason string) []Command {
	saga.Status = StatusCompensating
	saga.FailureReason = reason

	var commands []Command
	if saga.PaymentCaptured && !saga.PaymentRefunded {
		commands = append(commands, refundPayment(*saga))
	}
	if saga.RoomReserved && !saga.RoomReleased {
		commands = append(commands, releaseRoom(*saga))
	}
	settle(saga)
	return commands
}

// settle menandai saga failed begitu tidak ada lagi yang perlu dibatalkan.
func settle(saga *Saga) {
	roomDone := !saga.RoomReserved || saga.RoomReleased
	paymentDone := !saga.PaymentCaptured || saga.PaymentRefunded
	if saga.Status == StatusCompensating && roomDone && paymentDone {
		saga.Status = StatusFailed
	}
}

func releaseRoom(saga Saga) Command {
	return Command{Name: ReleaseRoom, BookingID: saga.BookingID, RoomType: saga.RoomType, Rooms: saga.Rooms}
}

func refundPayment(saga Saga) Command {
	return Command{Name: RefundPayment, BookingID: saga.BookingID, AmountIDR: saga.AmountIDR}
}

// commit mengirim command dulu, baru menyimpan state. Kalau pengiriman gagal,
// state belum berubah, sehingga event yang dikirim ulang akan diproses lagi
// dari posisi yang sama. Urutan sebaliknya membuat command hilang: state
// sudah maju, dan event yang dikirim ulang dianggap duplikat.
func (m *Manager) commit(ctx context.Context, saga Saga, commands []Command) error {
	for _, cmd := range commands {
		if err := m.bus.Send(ctx, cmd); err != nil {
			return fmt.Errorf("send %s for booking %s: %w", cmd.Name, saga.BookingID, err)
		}
	}
	if err := m.store.Save(ctx, saga); err != nil {
		return fmt.Errorf("save saga %s: %w", saga.BookingID, err)
	}
	return nil
}

// Tick mencari saga yang menunggu jawaban lebih lama dari stepTimeout, lalu
// menghentikannya dengan compensation. Saga yang sudah compensating tidak
// disentuh, jadi Tick aman dipanggil berulang.
func (m *Manager) Tick(ctx context.Context) error {
	active, err := m.store.Active(ctx)
	if err != nil {
		return fmt.Errorf("list active sagas: %w", err)
	}
	for _, saga := range active {
		waiting := saga.Status == StatusReservingRoom ||
			saga.Status == StatusCapturingPayment ||
			saga.Status == StatusConfirmingChannel
		if !waiting || m.now().Sub(saga.StepStartedAt) < m.stepTimeout {
			continue
		}
		commands := compensate(&saga, fmt.Sprintf("timeout di step %s", saga.Status))
		if err := m.commit(ctx, saga, commands); err != nil {
			return err
		}
	}
	return nil
}
