package lab20

import (
	"context"
	"sort"
	"sync"
	"time"
)

// File ini sudah jadi. Jangan diubah.

// EventName adalah nama event: fakta yang sudah terjadi di salah satu service.
type EventName string

const (
	BookingRequested          EventName = "BookingRequested"
	RoomReserved              EventName = "RoomReserved"
	RoomReservationFailed     EventName = "RoomReservationFailed"
	PaymentCaptured           EventName = "PaymentCaptured"
	PaymentFailed             EventName = "PaymentFailed"
	ChannelConfirmed          EventName = "ChannelConfirmed"
	ChannelConfirmationFailed EventName = "ChannelConfirmationFailed"
	RoomReleased              EventName = "RoomReleased"
	PaymentRefunded           EventName = "PaymentRefunded"
)

// Event adalah satu event yang diterima process manager. RoomType, Rooms,
// dan AmountIDR hanya terisi pada BookingRequested. Reason hanya terisi pada
// event kegagalan.
type Event struct {
	Name      EventName
	BookingID string
	RoomType  string
	Rooms     int
	AmountIDR int64
	Reason    string
}

// CommandName adalah nama command: perintah untuk tepat satu service.
type CommandName string

const (
	ReserveRoom        CommandName = "ReserveRoom"        // ke inventory service
	CapturePayment     CommandName = "CapturePayment"     // ke payment service
	ConfirmWithChannel CommandName = "ConfirmWithChannel" // ke channel service
	ReleaseRoom        CommandName = "ReleaseRoom"        // compensation untuk ReserveRoom
	RefundPayment      CommandName = "RefundPayment"      // compensation untuk CapturePayment
)

// Command adalah satu command yang dikirim process manager.
type Command struct {
	Name      CommandName
	BookingID string
	RoomType  string
	Rooms     int
	AmountIDR int64
}

// Status adalah posisi saga saat ini.
type Status string

const (
	StatusReservingRoom     Status = "reserving_room"
	StatusCapturingPayment  Status = "capturing_payment"
	StatusConfirmingChannel Status = "confirming_channel"
	StatusCompleted         Status = "completed"
	StatusCompensating      Status = "compensating"
	StatusFailed            Status = "failed"
)

// Saga adalah state satu proses booking. Inilah yang disimpan process
// manager di antara dua event.
type Saga struct {
	BookingID string
	Status    Status
	RoomType  string
	Rooms     int
	AmountIDR int64

	// Apa saja yang sudah terjadi di service lain, dan mana yang sudah
	// dibatalkan. Dari empat field ini bisa dihitung compensation yang tersisa.
	RoomReserved    bool
	RoomReleased    bool
	PaymentCaptured bool
	PaymentRefunded bool

	FailureReason string
	StepStartedAt time.Time // kapan step yang sedang ditunggu dimulai
}

// Store menyimpan state saga.
type Store interface {
	Load(ctx context.Context, bookingID string) (saga Saga, found bool, err error)
	Save(ctx context.Context, saga Saga) error
	// Active mengembalikan saga yang belum completed dan belum failed.
	Active(ctx context.Context) ([]Saga, error)
}

// CommandBus mengirim command ke service tujuan.
type CommandBus interface {
	Send(ctx context.Context, cmd Command) error
}

// MemoryStore adalah Store di memory. Di production ini tabel database.
type MemoryStore struct {
	mu    sync.Mutex
	sagas map[string]Saga
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sagas: map[string]Saga{}}
}

func (s *MemoryStore) Load(ctx context.Context, bookingID string) (Saga, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saga, found := s.sagas[bookingID]
	return saga, found, nil
}

func (s *MemoryStore) Save(ctx context.Context, saga Saga) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sagas[saga.BookingID] = saga
	return nil
}

func (s *MemoryStore) Active(ctx context.Context) ([]Saga, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var active []Saga
	for _, saga := range s.sagas {
		if saga.Status != StatusCompleted && saga.Status != StatusFailed {
			active = append(active, saga)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].BookingID < active[j].BookingID })
	return active, nil
}
