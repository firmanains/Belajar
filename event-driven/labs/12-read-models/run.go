package lab12

// File ini sudah jadi. Jangan diubah; baca saja.

import (
	"context"
	"fmt"
)

// Run adalah consumer: mengambil event dari channel satu per satu dan
// menyerahkannya ke handle. Ia berjalan di goroutine sendiri, terpisah dari
// HTTP handler yang membaca read model. Jarak waktu di antara keduanya adalah
// sumber eventual consistency.
func Run(ctx context.Context, events <-chan Envelope, handle func(context.Context, Envelope) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case env, ok := <-events:
			if !ok {
				return nil
			}
			if err := handle(ctx, env); err != nil {
				return fmt.Errorf("proyeksikan event %s: %w", env.EventID, err)
			}
		}
	}
}
