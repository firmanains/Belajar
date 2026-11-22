package lab02

// File ini sudah jadi. Jangan diubah.

import "context"

// Message adalah satu pesan di broker. Key menentukan partition; Value adalah
// isinya.
type Message struct {
	Key   string
	Value []byte
}

// Publisher menitipkan pesan ke sebuah topic.
type Publisher interface {
	Publish(ctx context.Context, topic string, msgs ...Message) error
}

// Handler memproses satu pesan. Return nil berarti "sudah kuproses, jangan
// kirim lagi".
type Handler func(ctx context.Context, msg Message) error

// Subscriber membaca topic sebagai anggota sebuah consumer group dan memanggil
// handler untuk setiap pesan.
//
// Subscribe baru kembali kalau ctx dibatalkan (return nil) atau handler
// mengembalikan error (return error itu). Kode aplikasi hanya mengenal dua
// interface ini; ia tidak tahu bahwa di baliknya ada Kafka.
type Subscriber interface {
	Subscribe(ctx context.Context, topic, group string, handler Handler) error
}
