package lab11

import (
	"context"
	"errors"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/components/cqrs"
	"github.com/ThreeDotsLabs/watermill/message"
)

// commandTopic memberi setiap command topic sendiri, terpisah dari topic event.
func commandTopic(commandName string) string { return "commands." + commandName }

// setupCommands dipanggil NewApp setelah sisi event siap, sebelum router jalan.
func (a *App) setupCommands() error {
	bus, err := cqrs.NewCommandBusWithConfig(a.publisher, cqrs.CommandBusConfig{
		GeneratePublishTopic: func(p cqrs.CommandBusGeneratePublishTopicParams) (string, error) {
			return commandTopic(p.CommandName), nil
		},
		Marshaler: marshaler,
	})
	if err != nil {
		return fmt.Errorf("buat command bus: %w", err)
	}
	a.commandBus = bus

	processor, err := cqrs.NewCommandProcessorWithConfig(a.router, cqrs.CommandProcessorConfig{
		GenerateSubscribeTopic: func(p cqrs.CommandProcessorGenerateSubscribeTopicParams) (string, error) {
			return commandTopic(p.CommandName), nil
		},
		SubscriberConstructor: func(cqrs.CommandProcessorSubscriberConstructorParams) (message.Subscriber, error) {
			return a.pubSub, nil
		},
		Marshaler: marshaler,
	})
	if err != nil {
		return fmt.Errorf("buat command processor: %w", err)
	}
	if err := processor.AddHandlers(
		cqrs.NewCommandHandler("CapturePayment", a.handleCapturePayment),
	); err != nil {
		return fmt.Errorf("pasang command handler: %w", err)
	}
	return nil
}

// Send mengirim sebuah command ke handler-nya.
func (a *App) Send(ctx context.Context, cmd any) error {
	return a.commandBus.Send(ctx, cmd)
}

// handleCapturePayment mengerjakan command, lalu mengabarkan hasilnya sebagai event.
//
// Tiga hasil, tiga perlakuan:
//   - berhasil: publish PaymentCaptured, return nil (ack)
//   - ditolak bank: publish PaymentRejected, return nil (ack). Mencoba lagi sia-sia.
//   - kegagalan teknis: return error (nack), supaya command dikirim ulang.
func (a *App) handleCapturePayment(ctx context.Context, cmd *CapturePayment) error {
	err := a.deps.Gateway.Capture(ctx, cmd.BookingID, cmd.AmountIDR)
	if errors.Is(err, ErrCardDeclined) {
		return a.Publish(ctx, PaymentRejected{BookingID: cmd.BookingID, Reason: err.Error()})
	}
	if err != nil {
		return fmt.Errorf("capture payment %s: %w", cmd.BookingID, err)
	}
	return a.Publish(ctx, PaymentCaptured{BookingID: cmd.BookingID, AmountIDR: cmd.AmountIDR})
}
