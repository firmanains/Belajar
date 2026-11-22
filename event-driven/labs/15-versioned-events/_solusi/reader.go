package lab15

import (
	"encoding/json"
	"fmt"
)

// DecodeV1 adalah kode consumer milik tim channel sync: ia membaca payload
// BookingCreated versi 1 sebagai tolerant reader. Field yang tidak dikenal
// diabaikan, dan field opsional yang tidak ada diisi default.
func DecodeV1(payload []byte) (BookingCreatedV1, error) {
	var ev BookingCreatedV1
	if err := json.Unmarshal(payload, &ev); err != nil {
		return BookingCreatedV1{}, fmt.Errorf("decode BookingCreated v1: %w", err)
	}
	if ev.RatePlan == "" {
		ev.RatePlan = DefaultRatePlan
	}
	return ev, nil
}
