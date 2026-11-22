package lab20

import (
	"context"
	"fmt"
	"time"
)

// File ini yang kamu ubah.

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
//
// Versi ini baru mengenal BookingRequested. Event lain diabaikan.
func (m *Manager) Handle(ctx context.Context, evt Event) error {
	if evt.Name != BookingRequested {
		return nil
	}

	saga := Saga{
		BookingID:     evt.BookingID,
		Status:        StatusReservingRoom,
		RoomType:      evt.RoomType,
		Rooms:         evt.Rooms,
		AmountIDR:     evt.AmountIDR,
		StepStartedAt: m.now(),
	}
	cmd := Command{Name: ReserveRoom, BookingID: saga.BookingID, RoomType: saga.RoomType, Rooms: saga.Rooms}
	if err := m.bus.Send(ctx, cmd); err != nil {
		return fmt.Errorf("send %s for booking %s: %w", cmd.Name, saga.BookingID, err)
	}
	if err := m.store.Save(ctx, saga); err != nil {
		return fmt.Errorf("save saga %s: %w", saga.BookingID, err)
	}
	return nil
}

// Tick dipanggil berkala (misalnya tiap menit) untuk mencari saga yang
// menunggu terlalu lama. Versi ini belum melakukan apa pun.
func (m *Manager) Tick(ctx context.Context) error {
	return nil
}
