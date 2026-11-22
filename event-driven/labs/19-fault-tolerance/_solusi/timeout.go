package lab19

import (
	"context"
	"fmt"
	"time"
)

// SendWithTimeout mengirim email dengan batas waktu. Batas itu berlaku di
// atas deadline yang mungkin sudah ada di ctx: yang lebih dekat yang menang.
func SendWithTimeout(ctx context.Context, sender Sender, timeout time.Duration, email Email) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := sender.Send(ctx, email); err != nil {
		return fmt.Errorf("send email booking %s: %w", email.BookingID, err)
	}
	return nil
}
