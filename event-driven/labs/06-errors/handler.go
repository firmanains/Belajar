package lab06

// File ini yang kamu ubah (Step 3).

import (
	"encoding/json"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
)

// EmailHandler mengirim email konfirmasi untuk setiap SendConfirmationEmail.
type EmailHandler struct {
	Emailer Emailer
}

func (h EmailHandler) Handle(msg *message.Message) error {
	var cmd SendConfirmationEmail
	if err := json.Unmarshal(msg.Payload, &cmd); err != nil {
		return fmt.Errorf("decode SendConfirmationEmail: %w", err)
	}

	if err := h.Emailer.Send(msg.Context(), cmd.To, buildSubject(cmd)); err != nil {
		return fmt.Errorf("send confirmation email for booking %s: %w", cmd.BookingID, err)
	}
	return nil
}
