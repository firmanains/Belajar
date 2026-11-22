package lab11

// File ini sudah jadi. Jangan diubah.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/components/cqrs"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/pubsub/gochannel"
)

type fixture struct {
	app      *App
	gateway  *FakeGateway
	bookings *Bookings
	receipts *Receipts
}

// startApp membuat App dengan dependency tiruan dan menjalankannya.
func startApp(t *testing.T) (context.Context, fixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	f := fixture{gateway: NewFakeGateway(), bookings: NewBookings(), receipts: NewReceipts()}
	app, err := NewApp(Deps{Gateway: f.gateway, Bookings: f.bookings, Receipts: f.receipts})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	f.app = app
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Logf("Close: %v", err)
		}
	})
	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return ctx, f
}

// eventually menunggu sampai cond benar, paling lama 3 detik. Pemrosesan
// message terjadi di goroutine lain, jadi hasilnya tidak langsung terlihat.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("setelah 3 detik masih belum: %s", what)
}

// Ringkasan lab 08: satu event sampai ke semua handler yang mendengarkannya.
// Sisi event sudah terpasang di app.go, jadi test ini lulus sejak kode awal.
func TestStep0_EventsFanOutToAllHandlers(t *testing.T) {
	ctx, f := startApp(t)
	if err := f.app.Publish(ctx, PaymentCaptured{BookingID: "B-1", AmountIDR: 1_500_000}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	eventually(t, "booking B-1 berstatus paid", func() bool { return f.bookings.Status("B-1") == StatusPaid })
	eventually(t, "kuitansi B-1 dijadwalkan", func() bool { return f.receipts.Queued("B-1") })
}

// Di luar step: command processor Watermill menolak dua handler untuk command
// yang sama. Event boleh punya banyak pendengar; command hanya punya satu pelaksana.
func TestDemo_TwoHandlersForOneCommandAreRejected(t *testing.T) {
	logger := watermill.NopLogger{}
	router, err := message.NewRouter(message.RouterConfig{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	pubSub := gochannel.NewGoChannel(gochannel.Config{}, logger)
	processor, err := cqrs.NewCommandProcessorWithConfig(router, cqrs.CommandProcessorConfig{
		GenerateSubscribeTopic: func(p cqrs.CommandProcessorGenerateSubscribeTopicParams) (string, error) {
			return "commands." + p.CommandName, nil
		},
		SubscriberConstructor: func(cqrs.CommandProcessorSubscriberConstructorParams) (message.Subscriber, error) {
			return pubSub, nil
		},
		Marshaler: marshaler,
	})
	if err != nil {
		t.Fatal(err)
	}
	noop := func(context.Context, *CapturePayment) error { return nil }
	err = processor.AddHandlers(
		cqrs.NewCommandHandler("CaptureWithGatewayA", noop),
		cqrs.NewCommandHandler("CaptureWithGatewayB", noop),
	)
	var dup cqrs.DuplicateCommandHandlerError
	if !errors.As(err, &dup) {
		t.Fatalf("AddHandlers return %v, seharusnya DuplicateCommandHandlerError", err)
	}
	t.Logf("Watermill menolak: %v", err)
}

// Command dikirim lewat bus dan sampai ke handler yang memanggil gateway.
func TestStep1_CommandReachesItsHandler(t *testing.T) {
	ctx, f := startApp(t)
	if err := f.app.Send(ctx, &CapturePayment{BookingID: "B-1", AmountIDR: 1_500_000}); err != nil {
		t.Fatalf("Send(CapturePayment) gagal: %v", err)
	}
	eventually(t, "gateway menarik pembayaran B-1", func() bool { return len(f.gateway.Captures()) == 1 })
	if got := f.gateway.Captures()[0]; got != (Capture{BookingID: "B-1", AmountIDR: 1_500_000}) {
		t.Fatalf("gateway menerima %+v, seharusnya B-1 sebesar 1500000", got)
	}
}

// Command dan event tidak boleh bercampur di topic yang sama.
func TestStep2_CommandsAndEventsUseSeparateTopics(t *testing.T) {
	ctx, f := startApp(t)
	if err := f.app.Send(ctx, &CapturePayment{BookingID: "B-1", AmountIDR: 1_500_000}); err != nil {
		t.Fatalf("Send(CapturePayment) gagal: %v", err)
	}
	eventually(t, "gateway menarik pembayaran B-1", func() bool { return len(f.gateway.Captures()) == 1 })

	topics := f.app.PublishedTopics()
	if !slices.Contains(topics, "commands.CapturePayment") {
		t.Fatalf("topic yang dipakai: %v; command CapturePayment seharusnya di topic \"commands.CapturePayment\"", topics)
	}
	for _, topic := range topics {
		if topic == "events.CapturePayment" {
			t.Fatal("command CapturePayment di-publish ke topic event")
		}
	}
}

// Setelah bekerja, handler mengabarkan hasilnya sebagai event. Handler lain
// bereaksi terhadap event itu tanpa diketahui oleh command handler.
func TestStep3_HandlerPublishesEventAfterWork(t *testing.T) {
	ctx, f := startApp(t)
	if err := f.app.Send(ctx, &CapturePayment{BookingID: "B-1", AmountIDR: 1_500_000}); err != nil {
		t.Fatalf("Send(CapturePayment) gagal: %v", err)
	}
	eventually(t, "booking B-1 berstatus paid (lewat event PaymentCaptured)", func() bool {
		return f.bookings.Status("B-1") == StatusPaid
	})
	eventually(t, "kuitansi B-1 dijadwalkan (lewat event PaymentCaptured)", func() bool {
		return f.receipts.Queued("B-1")
	})
	if !slices.Contains(f.app.PublishedTopics(), "events.PaymentCaptured") {
		t.Fatalf("topic yang dipakai: %v; tidak ada events.PaymentCaptured", f.app.PublishedTopics())
	}
}

// Kartu ditolak bank. Itu jawaban, bukan kegagalan: handler mengabarkan
// PaymentRejected dan selesai. Kalau handler return error, command dikirim
// ulang terus dan gateway dipanggil berkali-kali untuk kartu yang sama.
func TestStep4_BusinessRejectionIsAnEventNotAnError(t *testing.T) {
	ctx, f := startApp(t)
	f.gateway.Decline("B-2")
	if err := f.app.Send(ctx, &CapturePayment{BookingID: "B-2", AmountIDR: 900_000}); err != nil {
		t.Fatalf("Send(CapturePayment) gagal: %v", err)
	}
	eventually(t, "booking B-2 berstatus payment_failed (lewat event PaymentRejected)", func() bool {
		return f.bookings.Status("B-2") == StatusPaymentFailed
	})
	time.Sleep(100 * time.Millisecond) // beri waktu kalau-kalau command dikirim ulang
	if n := f.gateway.Calls("B-2"); n != 1 {
		t.Fatalf("gateway dipanggil %d kali untuk kartu yang ditolak, seharusnya 1: penolakan bisnis tidak boleh memicu retry", n)
	}
}

// Gateway timeout sekali. Itu kegagalan teknis: handler return error supaya
// command dikirim ulang, dan percobaan kedua berhasil.
func TestStep4_TechnicalFailureIsRetried(t *testing.T) {
	ctx, f := startApp(t)
	f.gateway.TimeoutNext(1)
	if err := f.app.Send(ctx, &CapturePayment{BookingID: "B-3", AmountIDR: 700_000}); err != nil {
		t.Fatalf("Send(CapturePayment) gagal: %v", err)
	}
	eventually(t, "booking B-3 berstatus paid setelah gateway pulih", func() bool {
		return f.bookings.Status("B-3") == StatusPaid
	})
	if n := f.gateway.Calls("B-3"); n != 2 {
		t.Fatalf("gateway dipanggil %d kali, seharusnya 2 (timeout, lalu berhasil)", n)
	}
}
