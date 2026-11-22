package lab11

// File ini kamu ubah di Step 1 sampai Step 4.

import (
	"context"
	"errors"
)

// setupCommands dipanggil NewApp setelah sisi event siap, sebelum router jalan.
// Di sinilah command bus dan command processor dibuat dan handler dipasang.
func (a *App) setupCommands() error {
	// Belum ada command bus, command processor, maupun handler.
	return nil
}

// Send mengirim sebuah command ke handler-nya.
func (a *App) Send(ctx context.Context, cmd any) error {
	return errors.New("command bus belum dipasang")
}
