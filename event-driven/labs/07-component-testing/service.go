package lab07

// File ini yang kamu ubah (Step 3 dan Step 4).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
)

// PubSub adalah gabungan Publisher dan Subscriber; gochannel memenuhi keduanya.
type PubSub interface {
	message.Publisher
	message.Subscriber
}

// Config berisi semua dependency keluar service. Test mengganti isinya dengan
// tiruan; kode service sendiri tidak berubah.
type Config struct {
	PubSub            PubSub
	ChannelManagerURL string
	Emailer           Emailer
}

// Service adalah PMS bagian sinkronisasi: ia mendengarkan event booking dan
// meneruskannya ke channel manager dan ke tamu.
type Service struct {
	router *message.Router
}

func New(cfg Config) (*Service, error) {
	router, err := message.NewRouter(message.RouterConfig{}, watermill.NopLogger{})
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	sync := channelSync{baseURL: cfg.ChannelManagerURL, client: http.DefaultClient}
	email := confirmationEmail{emailer: cfg.Emailer}

	router.AddConsumerHandler("sync-channel-manager", TopicBookingCreated, cfg.PubSub, sync.handleBookingCreated)
	router.AddConsumerHandler("send-confirmation-email", "bookings.created", cfg.PubSub, email.handleBookingCreated)
	router.AddConsumerHandler("restore-channel-availability", TopicBookingCancelled, cfg.PubSub, sync.handleBookingCancelled)

	return &Service{router: router}, nil
}

// Run memblokir sampai ctx dibatalkan atau Close dipanggil.
func (s *Service) Run(ctx context.Context) error { return s.router.Run(ctx) }

// Running tertutup ketika semua handler sudah subscribe dan siap menerima message.
func (s *Service) Running() <-chan struct{} { return s.router.Running() }

func (s *Service) Close() error { return s.router.Close() }

// channelSync meneruskan perubahan availability ke channel manager lewat HTTP.
type channelSync struct {
	baseURL string
	client  *http.Client
}

func (c channelSync) handleBookingCreated(msg *message.Message) error {
	var event BookingCreated
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("decode BookingCreated: %w", err)
	}
	return c.push(msg.Context(), AvailabilityUpdate{
		BookingID: event.BookingID, RoomType: event.RoomType, Date: event.Date, Delta: -event.Rooms,
	})
}

func (c channelSync) handleBookingCancelled(msg *message.Message) error {
	var event BookingCancelled
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("decode BookingCancelled: %w", err)
	}
	return c.push(msg.Context(), AvailabilityUpdate{
		BookingID: event.BookingID, RoomType: event.RoomType, Date: event.Date, Delta: event.Rooms,
	})
}

func (c channelSync) push(ctx context.Context, update AvailabilityUpdate) error {
	body, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("encode availability update: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/availability", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build availability request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("push availability for booking %s: %w", update.BookingID, err)
	}
	defer resp.Body.Close()

	return nil
}

// confirmationEmail mengirim email konfirmasi untuk setiap booking baru.
type confirmationEmail struct {
	emailer Emailer
}

func (c confirmationEmail) handleBookingCreated(msg *message.Message) error {
	var event BookingCreated
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return fmt.Errorf("decode BookingCreated: %w", err)
	}
	if err := c.emailer.SendConfirmation(msg.Context(), event.BookingID, event.GuestEmail); err != nil {
		return fmt.Errorf("send confirmation for booking %s: %w", event.BookingID, err)
	}
	return nil
}
