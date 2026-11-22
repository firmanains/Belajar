package lab11

// File ini sudah jadi. Jangan diubah; baca saja.
//
// Isinya sisi event (ringkasan lab 08) dan siklus hidup router.
// Sisi command kamu tulis di commands.go.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/components/cqrs"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

// marshaler mengubah struct menjadi message dan sebaliknya. Nama command dan
// event diambil dari nama struct-nya saja, misalnya "CapturePayment".
var marshaler = cqrs.JSONMarshaler{GenerateName: cqrs.StructName}

// Deps adalah semua hal di luar messaging yang dibutuhkan handler.
type Deps struct {
	Gateway  PaymentGateway
	Bookings *Bookings
	Receipts *Receipts
}

// App merangkai pub/sub, router, event bus, dan (setelah kamu kerjakan) command bus.
type App struct {
	deps Deps

	// pubSub adalah broker di dalam memori. Pakai sebagai Subscriber.
	pubSub *gochannel.GoChannel
	// publisher membungkus pubSub dan mencatat topic. Pakai ini, bukan pubSub,
	// setiap kali butuh message.Publisher.
	publisher *recordingPublisher
	router    *message.Router
	eventBus  *cqrs.EventBus

	// commandBus kosong di kode awal. Isi di setupCommands.
	commandBus *cqrs.CommandBus

	runErr chan error
}

// NewApp menyiapkan sisi event, lalu memanggil setupCommands.
func NewApp(deps Deps) (*App, error) {
	logger := watermill.NopLogger{}
	router, err := message.NewRouter(message.RouterConfig{CloseTimeout: 2 * time.Second}, logger)
	if err != nil {
		return nil, fmt.Errorf("buat router: %w", err)
	}
	pubSub := gochannel.NewGoChannel(gochannel.Config{}, logger)
	a := &App{
		deps:      deps,
		pubSub:    pubSub,
		publisher: &recordingPublisher{Publisher: pubSub},
		router:    router,
		runErr:    make(chan error, 1),
	}

	// Event bus: satu topic per event, dengan awalan "events.".
	a.eventBus, err = cqrs.NewEventBusWithConfig(a.publisher, cqrs.EventBusConfig{
		GeneratePublishTopic: func(p cqrs.GenerateEventPublishTopicParams) (string, error) {
			return "events." + p.EventName, nil
		},
		Marshaler: marshaler,
	})
	if err != nil {
		return nil, fmt.Errorf("buat event bus: %w", err)
	}

	// Event processor: setiap handler punya subscription sendiri, jadi satu
	// event sampai ke semua handler yang mendengarkannya.
	eventProcessor, err := cqrs.NewEventProcessorWithConfig(router, cqrs.EventProcessorConfig{
		GenerateSubscribeTopic: func(p cqrs.EventProcessorGenerateSubscribeTopicParams) (string, error) {
			return "events." + p.EventName, nil
		},
		SubscriberConstructor: func(cqrs.EventProcessorSubscriberConstructorParams) (message.Subscriber, error) {
			return pubSub, nil
		},
		Marshaler: marshaler,
	})
	if err != nil {
		return nil, fmt.Errorf("buat event processor: %w", err)
	}
	err = eventProcessor.AddHandlers(
		cqrs.NewEventHandler("MarkBookingPaid", func(_ context.Context, e *PaymentCaptured) error {
			deps.Bookings.SetStatus(e.BookingID, StatusPaid)
			return nil
		}),
		cqrs.NewEventHandler("QueueReceipt", func(_ context.Context, e *PaymentCaptured) error {
			deps.Receipts.Queue(e.BookingID)
			return nil
		}),
		cqrs.NewEventHandler("MarkBookingPaymentFailed", func(_ context.Context, e *PaymentRejected) error {
			deps.Bookings.SetStatus(e.BookingID, StatusPaymentFailed)
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("pasang event handler: %w", err)
	}

	if err := a.setupCommands(); err != nil {
		return nil, fmt.Errorf("pasang command: %w", err)
	}
	return a, nil
}

// Publish mengabarkan sebuah event ke semua handler yang mendengarkannya.
func (a *App) Publish(ctx context.Context, event any) error {
	return a.eventBus.Publish(ctx, event)
}

// Start menjalankan router di background dan menunggu sampai semua handler
// siap menerima message.
func (a *App) Start(ctx context.Context) error {
	go func() { a.runErr <- a.router.Run(ctx) }()
	select {
	case <-a.router.Running():
		return nil
	case err := <-a.runErr:
		return fmt.Errorf("router berhenti sebelum siap: %w", err)
	case <-time.After(5 * time.Second):
		return errors.New("router tidak siap dalam 5 detik")
	}
}

// Close menghentikan router dan menutup pub/sub.
func (a *App) Close() error {
	return errors.Join(a.router.Close(), a.pubSub.Close())
}

// PublishedTopics mengembalikan topic dari semua message yang pernah di-publish.
func (a *App) PublishedTopics() []string { return a.publisher.Topics() }

// recordingPublisher meneruskan Publish dan mencatat topic-nya untuk test.
type recordingPublisher struct {
	message.Publisher

	mu     sync.Mutex
	topics []string
}

func (r *recordingPublisher) Publish(topic string, messages ...*message.Message) error {
	r.mu.Lock()
	r.topics = append(r.topics, topic)
	r.mu.Unlock()
	return r.Publisher.Publish(topic, messages...)
}

func (r *recordingPublisher) Topics() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.topics...)
}
