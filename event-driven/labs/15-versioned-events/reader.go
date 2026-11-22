package lab15

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// File ini yang kamu ubah di Step 1 dan Step 2.

// DecodeV1 adalah kode consumer milik tim channel sync: ia membaca payload
// BookingCreated versi 1.
func DecodeV1(payload []byte) (BookingCreatedV1, error) {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields() // "supaya ketahuan kalau ada yang aneh"

	var ev BookingCreatedV1
	if err := dec.Decode(&ev); err != nil {
		return BookingCreatedV1{}, fmt.Errorf("decode BookingCreated v1: %w", err)
	}
	return ev, nil
}
