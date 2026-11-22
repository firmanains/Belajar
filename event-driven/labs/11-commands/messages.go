package lab11

// File ini sudah jadi. Jangan diubah; baca saja.

// Command: permintaan kepada satu handler untuk melakukan sesuatu.
// Namanya kalimat perintah, dan handler boleh menolaknya.

// CapturePayment meminta pembayaran sebuah booking ditarik dari kartu tamu.
type CapturePayment struct {
	BookingID string `json:"booking_id"`
	AmountIDR int64  `json:"amount_idr"`
}

// Event: fakta yang sudah terjadi. Namanya bentuk lampau, dan siapa pun boleh
// mendengarkan. Fakta tidak bisa ditolak.

// PaymentCaptured menyatakan pembayaran sebuah booking sudah berhasil ditarik.
type PaymentCaptured struct {
	BookingID string `json:"booking_id"`
	AmountIDR int64  `json:"amount_idr"`
}

// PaymentRejected menyatakan pembayaran ditolak karena alasan bisnis,
// misalnya kartu ditolak bank. Mencoba lagi tidak akan mengubah hasilnya.
type PaymentRejected struct {
	BookingID string `json:"booking_id"`
	Reason    string `json:"reason"`
}

const (
	StatusPaid          = "paid"
	StatusPaymentFailed = "payment_failed"
)
