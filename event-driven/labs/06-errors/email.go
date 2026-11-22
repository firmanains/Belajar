package lab06

// File ini sudah jadi. Jangan diubah; baca saja.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"

	// Import kosong ini membuat dependency middleware bawaan Watermill sudah
	// tercatat di go.sum ketika kamu memakainya di Step 4.
	_ "github.com/ThreeDotsLabs/watermill/message/router/middleware"
)

const (
	TopicEmailRequested = "email.confirmation-requested"
	ConsumerGroup       = "email-service"
	HandlerName         = "send-confirmation-email"
)

// SendConfirmationEmail adalah command yang diterima layanan email.
type SendConfirmationEmail struct {
	BookingID string `json:"booking_id"`
	To        string `json:"to"`
	Guest     *Guest `json:"guest"`
}

type Guest struct {
	Name string `json:"name"`
}

// Emailer mengirim email lewat SMTP. Bisa gagal sementara.
type Emailer interface {
	Send(ctx context.Context, to, subject string) error
}

// ErrSMTPTimeout adalah kegagalan sementara: server SMTP sedang tidak menjawab.
var ErrSMTPTimeout = errors.New("smtp: connection timeout")

// buildSubject menyusun subject email. Fungsi ini punya bug yang disengaja:
// ia panic kalau Guest kosong. Jangan diperbaiki. Di Step 4 kamu membuat
// service selamat dari bug yang belum kamu ketahui.
func buildSubject(cmd SendConfirmationEmail) string {
	return "Konfirmasi booking " + cmd.BookingID + " untuk " + strings.ToUpper(cmd.Guest.Name)
}

// PermanentError membungkus error yang tidak akan pernah berhasil walau
// message diproses ulang, misalnya payload yang rusak.
type PermanentError struct {
	Err error
}

func (e PermanentError) Error() string { return "permanent: " + e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }

// Permanent menandai err sebagai error permanen.
func Permanent(err error) error { return PermanentError{Err: err} }

// IsPermanent melaporkan apakah err, atau error yang dibungkusnya, permanen.
func IsPermanent(err error) bool {
	var permanent PermanentError
	return errors.As(err, &permanent)
}

// ParkedMessage adalah message yang disingkirkan dari antrean beserta alasannya.
type ParkedMessage struct {
	UUID    string
	Payload string
	Reason  string
}

// ParkingLot menampung message yang gagal supaya antrean bisa jalan terus.
// Di sini ia hanya slice di memory. Bentuk production-nya adalah dead letter
// queue, yang dibahas di Lab 19.
type ParkingLot struct {
	mu     sync.Mutex
	parked []ParkedMessage
}

func (p *ParkingLot) Add(msg *message.Message, reason error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.parked = append(p.parked, ParkedMessage{UUID: msg.UUID, Payload: string(msg.Payload), Reason: reason.Error()})
}

func (p *ParkingLot) Messages() []ParkedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]ParkedMessage(nil), p.parked...)
}

// NewRouter merakit layanan email: satu handler yang membaca dari broker,
// dibungkus middleware dari Middlewares.
func NewRouter(
	broker *Broker,
	handler message.NoPublishHandlerFunc,
	retry RetryConfig,
	lot *ParkingLot,
	logger *slog.Logger,
) (*message.Router, error) {
	router, err := message.NewRouter(message.RouterConfig{}, watermill.NopLogger{})
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	router.AddMiddleware(Middlewares(retry, lot, logger)...)
	router.AddConsumerHandler(HandlerName, TopicEmailRequested, broker.Subscriber(ConsumerGroup), handler)

	return router, nil
}
