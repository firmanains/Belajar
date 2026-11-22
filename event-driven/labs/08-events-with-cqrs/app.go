package lab08

// File ini yang kamu ubah.

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/components/cqrs"
	"github.com/ThreeDotsLabs/watermill/message"
)

// App adalah satu instance service PMS: EventBus untuk menerbitkan event dan
// router yang menjalankan event handler.
type App struct {
	EventBus *cqrs.EventBus
	router   *message.Router
}

func NewApp(broker *Broker, deps Deps) (*App, error) {
	router, err := message.NewRouter(message.RouterConfig{}, watermill.NopLogger{})
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	// Nama event diambil dari nama struct: BookingCreated -> "BookingCreated".
	marshaler := cqrs.JSONMarshaler{GenerateName: cqrs.StructName}
	// Satu topic per jenis event.
	topicFor := func(eventName string) string { return "events." + eventName }

	eventBus, err := cqrs.NewEventBusWithConfig(broker, cqrs.EventBusConfig{
		GeneratePublishTopic: func(params cqrs.GenerateEventPublishTopicParams) (string, error) {
			return topicFor(params.EventName), nil
		},
		Marshaler: marshaler,
	})
	if err != nil {
		return nil, fmt.Errorf("create event bus: %w", err)
	}

	instanceID := watermill.NewShortUUID()

	processor, err := cqrs.NewEventProcessorWithConfig(router, cqrs.EventProcessorConfig{
		GenerateSubscribeTopic: func(params cqrs.EventProcessorGenerateSubscribeTopicParams) (string, error) {
			return topicFor(params.EventName), nil
		},
		// Dipanggil sekali per handler. Nama yang diberikan ke broker.Subscriber
		// menjadi nama consumer group handler itu.
		SubscriberConstructor: func(params cqrs.EventProcessorSubscriberConstructorParams) (message.Subscriber, error) {
			return broker.Subscriber(params.HandlerName + "-" + instanceID), nil
		},
		Marshaler: marshaler,
	})
	if err != nil {
		return nil, fmt.Errorf("create event processor: %w", err)
	}

	err = processor.AddHandlers(
		cqrs.NewEventHandler("on-booking-created", func(ctx context.Context, event *BookingCreated) error {
			if err := deps.Availability.Decrease(ctx, event.RoomType, event.Rooms); err != nil {
				return fmt.Errorf("decrease availability for booking %s: %w", event.BookingID, err)
			}
			if err := deps.Emailer.SendConfirmation(ctx, event.BookingID, event.GuestEmail); err != nil {
				return fmt.Errorf("send confirmation for booking %s: %w", event.BookingID, err)
			}
			if err := deps.ChannelManager.PushAvailability(ctx, event.RoomType); err != nil {
				return fmt.Errorf("push availability for booking %s: %w", event.BookingID, err)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("add event handlers: %w", err)
	}

	return &App{EventBus: eventBus, router: router}, nil
}

// Run memblokir sampai ctx dibatalkan atau Close dipanggil.
func (a *App) Run(ctx context.Context) error { return a.router.Run(ctx) }

// Running tertutup ketika semua handler sudah subscribe.
func (a *App) Running() <-chan struct{} { return a.router.Running() }

func (a *App) Close() error { return a.router.Close() }
