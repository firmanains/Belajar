package lab19

import (
	"context"
	"time"
)

// File ini yang kamu ubah di Step 1.

// SendWithTimeout mengirim email dengan batas waktu. Versi ini meneruskan
// ctx apa adanya, jadi ia menunggu selama server SMTP mau membuatnya menunggu.
func SendWithTimeout(ctx context.Context, sender Sender, timeout time.Duration, email Email) error {
	return sender.Send(ctx, email)
}
