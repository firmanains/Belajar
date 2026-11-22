# 04 Lab - Watermill Basics

[[03 Lab - Events]] · [[00 Start Here]] · [[05 Lab - Middlewares]]

---

**Goal:** mengganti consume loop tulisan tangan dengan Router Watermill, lalu memahami tiga hal yang diurusnya: ack dan nack, graceful shutdown, dan health check.
**Covers:** Watermill `message.Router`, handler, ack/nack, graceful shutdown, health check.
**Prerequisites:** [[02 Lab - Message Broker]] dan [[03 Lab - Events]].
**Infrastructure:** none. Pub/Sub berjalan di memori.
**Time:** 2–3 jam.

---

## Why This Exists

Subscriber di lab 02 panjangnya sekitar 40 baris, dan hanya satu baris di antaranya yang berisi logika bisnis: pemanggilan handler. Sisanya adalah pekerjaan rutin. Membuat client, polling, memeriksa error, menelusuri record, commit, berhenti saat `ctx` dibatalkan.

Service nyata punya beberapa handler, dan setiap handler butuh pekerjaan rutin yang sama. Menyalinnya berarti menyalin juga bug-nya. Handler yang lupa commit, handler yang berhenti di tengah pesan saat deploy, handler yang tidak pernah berhenti: semuanya lahir dari loop yang ditulis ulang dengan sedikit perbedaan.

Watermill adalah library Go open source yang mengambil alih pekerjaan rutin itu. Kamu menulis fungsi yang menerima satu pesan dan mengembalikan error. Library yang mengurus sisanya, di atas broker apa pun yang punya adapter: Kafka, Redis Streams, Postgres, atau channel Go di memori.

## Concepts

### Publisher, Subscriber, Message

Watermill mendefinisikan dua interface yang bentuknya mirip dengan yang kamu tulis di lab 02:

```go
type Publisher interface {
	Publish(topic string, messages ...*Message) error
	Close() error
}

type Subscriber interface {
	Subscribe(ctx context.Context, topic string) (<-chan *Message, error)
	Close() error
}
```

Bedanya ada di `Subscribe`: ia mengembalikan channel berisi pesan, bukan menerima handler. Setiap implementasi broker disebut Pub/Sub. Lab ini memakai `gochannel`, Pub/Sub di memori, supaya perhatian tertuju pada Router dan bukan pada infrastruktur.

`Message` punya empat bagian yang penting: `UUID`, `Payload` (byte), `Metadata` (pasangan key-value, padanan envelope di lab 03), dan sepasang method `Ack()` dan `Nack()`.

### Ack dan nack

**Ack** berarti "sudah kuproses, jangan kirim lagi". **Nack** berarti "aku gagal, kirim lagi". Arti persisnya di tiap broker berbeda. Di Kafka, ack berujung pada commit offset. Di GoChannel, nack membuat pesan langsung dikirim ulang ke subscriber yang sama.

`ConsumeManually` di `manual.go` memanggil keduanya dengan tangan. Lupa memanggil salah satunya berarti pesan menggantung selamanya.

### Router

Router menghubungkan Subscriber dengan handler, dan menerjemahkan nilai yang dikembalikan handler menjadi ack atau nack.

```
Subscriber ──► Router ──► handler(msg) error
                 │              │
                 │         return nil  ──► msg.Ack()
                 │         return err  ──► msg.Nack()
                 │
                 ├─ menjalankan semua handler yang terdaftar
                 ├─ berhenti dengan rapi saat ctx dibatalkan
                 └─ tahu kapan dirinya siap (Running) dan sudah tutup (IsClosed)
```

Padanan terdekatnya di dunia yang sudah kamu kenal adalah router HTTP. `http.ServeMux` menerima koneksi, mem-parsing request, dan memanggil handler sesuai path. Router Watermill menerima pesan dan memanggil handler sesuai topic. Analogi ini berhenti bekerja di nilai yang dikembalikan: handler HTTP yang gagal menghasilkan response 500 dan request-nya selesai, sedangkan handler pesan yang gagal membuat pesan itu datang lagi.

Konsekuensinya besar. **Nilai yang dikembalikan handler adalah keputusan tentang nasib pesan.** Return `nil` untuk pesan yang gagal diproses berarti membuangnya. Return error untuk pesan yang tidak akan pernah bisa diproses berarti memintanya dikirim ulang tanpa akhir.

### Graceful shutdown

Saat deploy, proses lama menerima sinyal berhenti. Ada tiga hal yang harus terjadi, dalam urutan ini: berhenti mengambil pesan baru, menunggu handler yang sedang berjalan sampai selesai, baru keluar. Kalau proses keluar di tengah handler, pesan itu belum di-ack dan akan dikirim ulang ke instance lain. Efek yang sudah telanjur terjadi, misalnya email yang sudah terkirim, akan terjadi lagi.

`Router.Run(ctx)` melakukan ketiganya saat `ctx` dibatalkan. Batas waktu menunggunya diatur lewat `RouterConfig.CloseTimeout`. Syaratnya satu: fungsi `Run` milikmu tidak boleh kembali lebih dulu daripada `Router.Run`.

### Health check

Platform seperti Kubernetes bertanya ke setiap instance: apakah kamu siap menerima pekerjaan? Untuk service HTTP, jawabannya sering kali "ya" begitu port terbuka. Untuk consumer, proses yang hidup belum tentu sedang membaca pesan. Router yang gagal melakukan subscribe atau yang sudah ditutup menghasilkan proses yang tampak sehat tapi tidak mengerjakan apa pun.

Health check yang jujur mengikuti keadaan Router. `IsRunning()` bernilai true setelah semua handler berhasil melakukan subscribe. `IsClosed()` bernilai true setelah Router ditutup.

## The Exercise

Kode ada di `labs/04-watermill-basics/`.

| File | Isi | Diubah? |
|---|---|---|
| `manual.go` | `BookingCreated`, `Emailer`, nama topic, dan `ConsumeManually` | Tidak, baca sebagai pembanding |
| `app.go` | `App`, `NewApp`, `Run`, `HealthHandler` yang masih kosong | Ya |
| `app_test.go` | Enam skenario dengan GoChannel dan SMTP tiruan | Tidak |

Signature `NewApp`, `Run`, dan `HealthHandler` jangan diubah. Dokumentasi Router bisa dibaca dari terminal:

```bash
go doc github.com/ThreeDotsLabs/watermill/message.Router
```

### Step 0: The Hand-Written Loop

```bash
cd labs/04-watermill-basics
go test -run Step0 -v
```

Hasilnya `PASS`. `ConsumeManually` bekerja. Baca fungsi itu dan tandai baris mana yang berisi logika bisnis dan mana yang akan sama persis di handler lain.

### Step 1: Register a Handler

```bash
go test -run Step1 -v
```

Test menjalankan `App`, menerbitkan `{"booking_id":"B-1"}`, lalu menunggu email. Pesan gagalnya: `belum ada handler yang menerima pesan dari booking.created`.

**Task:** di `NewApp`, buat Router dan daftarkan satu handler untuk `TopicBookingCreated` yang membaca payload dan memanggil `email.SendConfirmation`. Di `Run`, jalankan Router.

> [!hint]- Hint
> `message.NewRouter(message.RouterConfig{}, logger)` membuat Router. Simpan di field `App`. Daftarkan handler dengan `router.AddConsumerHandler("send_confirmation_email", TopicBookingCreated, sub, func(msg *message.Message) error { ... })`. Di dalamnya: `json.Unmarshal(msg.Payload, &event)`, lalu `email.SendConfirmation(msg.Context(), event.BookingID)`. `Run` cukup memanggil `a.router.Run(ctx)`.

### Step 2: An Error Means Nack

```bash
go test -run Step2 -v
```

SMTP gagal dua kali, lalu berhasil. Tebak dulu: tanpa satu baris kode retry pun, apakah email akan terkirim?

Kalau handler kamu meneruskan error dari `SendConfirmation`, test ini lulus tanpa perubahan. Router mengirim nack, GoChannel mengirim ulang, dan percobaan ketiga berhasil. Kalau handler kamu mencatat error lalu mengembalikan `nil`, pesan gagalnya: `error dari SMTP harus sampai ke Router supaya pesan dikirim ulang`.

**Task:** pastikan error dari `SendConfirmation` dikembalikan oleh handler.

### Step 3: A Message That Can Never Succeed

```bash
go test -run Step3 -v
```

Test menerbitkan `ini bukan JSON`, lalu pesan yang sehat. Setelah Step 2, refleksnya adalah mengembalikan error dari `json.Unmarshal`. Jalankan dan lihat angkanya:

`pesan rusak dikirim ke handler 37184 kali, seharusnya 1: nack membuatnya dikirim ulang tanpa akhir`

Angkanya berbeda di tiap mesin. Pesan itu tidak akan pernah menjadi JSON yang sah, berapa kali pun dikirim ulang. Di GoChannel ia berputar sambil membakar CPU sementara pesan lain tetap lewat. Di broker yang mempertahankan urutan per partition seperti Kafka, consumer tidak bisa maju melewatinya.

**Task:** pesan yang tidak bisa dibaca dicatat lewat `logger.Error`, lalu di-ack.

> [!hint]- Hint
> Di cabang `json.Unmarshal` gagal (atau `BookingID` kosong), panggil `logger.Error("dropping malformed message", err, watermill.LogFields{"message_uuid": msg.UUID})` lalu `return nil`. Membuang pesan terasa salah, dan memang belum ideal. Tempat yang benar untuk pesan seperti ini adalah dead letter queue, yang baru dibangun di lab 19.

### Step 4: Shutting Down Mid-Message

```bash
go test -run Step4 -v
```

Test menahan SMTP tiruan, menunggu sampai handler masuk ke dalamnya, lalu membatalkan `ctx`. Selama SMTP belum selesai, `Run` tidak boleh kembali. Setelah SMTP dilepas, `Run` harus kembali dan email harus terkirim tepat sekali.

Kalau `Run` milikmu hanya `return a.router.Run(ctx)`, test ini lulus: Router sudah menunggu handler yang sedang berjalan. Supaya kamu melihat apa yang dijaga test ini, ubah `Run` sementara menjadi versi di bawah, jalankan test, lalu kembalikan.

```go
go func() { _ = a.router.Run(ctx) }()
<-ctx.Done()
return nil
```

Pesan gagalnya: `Run kembali saat handler masih mengirim email: proses akan mati di tengah pekerjaan`. Pola ini sering muncul di `main` yang menjalankan beberapa komponen sekaligus.

### Step 5: An Honest Health Check

```bash
go test -run Step5 -v
```

Test memeriksa tiga keadaan: 503 sebelum `Run` dipanggil, 200 selama Router berjalan, dan 503 setelah `Run` kembali.

**Task:** `HealthHandler` menjawab sesuai keadaan Router.

> [!hint]- Hint
> Jawab 200 hanya kalau `a.router.IsRunning() && !a.router.IsClosed()`. Selain itu `http.Error(w, "not ready", http.StatusServiceUnavailable)`.

### Finish

```bash
go test -race -v
```

## Solution

> [!success]- Solution
> Ide kuncinya: handler hanya berisi logika bisnis ditambah satu keputusan, yaitu `nil` atau error.
>
> ```go
> router.AddConsumerHandler(
> 	"send_confirmation_email",
> 	TopicBookingCreated,
> 	sub,
> 	func(msg *message.Message) error {
> 		var event BookingCreated
> 		if err := json.Unmarshal(msg.Payload, &event); err != nil || event.BookingID == "" {
> 			// Tidak akan pernah bisa diproses: ack supaya tidak berputar.
> 			logger.Error("dropping malformed message", err, watermill.LogFields{"message_uuid": msg.UUID})
> 			return nil
> 		}
> 		if err := email.SendConfirmation(msg.Context(), event.BookingID); err != nil {
> 			// Mungkin berhasil nanti: nack supaya dikirim ulang.
> 			return fmt.Errorf("send confirmation for %s: %w", event.BookingID, err)
> 		}
> 		return nil
> 	},
> )
> ```
>
> `Run` menyerahkan `ctx` ke `router.Run` dan menunggunya kembali. `HealthHandler` memeriksa `IsRunning` dan `IsClosed`. Kode lengkap: `labs/04-watermill-basics/_solusi/app.go`.

## When This Is the Wrong Choice

Library seperti Watermill menyamakan semua broker ke satu bentuk: publish, subscribe, ack, nack. Hal yang hanya dimiliki satu broker ikut tersembunyi. Kalau kamu butuh mengatur offset Kafka sendiri, membaca ulang dari titik tertentu, atau memakai transaction Kafka, abstraksi itu menjadi penghalang dan client langsung seperti franz-go lebih tepat.

Untuk satu consumer dengan satu handler, consume loop 40 baris yang kamu pahami sepenuhnya bisa lebih baik daripada dependency yang perilakunya harus dipelajari. Library mulai membayar harganya saat handler bertambah dan saat kamu butuh middleware, yang dibahas di lab berikutnya.

GoChannel sendiri bukan broker untuk production. Pesannya hidup di memori, persis seperti channel di lab 01. Ia berguna untuk test dan untuk komunikasi di dalam satu proses.

## What Is Still Broken

1. **Nack di GoChannel mengirim ulang seketika, tanpa jeda.** SMTP yang mati lima menit berarti jutaan percobaan dalam lima menit. Retry dengan jeda dan batas ada di [[06 Lab - Errors]].
2. **Pesan rusak dibuang.** Log adalah satu-satunya jejaknya. Dead letter queue ada di [[19 Lab - Fault Tolerance]].
3. **`msg.Context()` dibatalkan begitu shutdown diminta.** Dengan Watermill v1.5.3 dan GoChannel, context milik pesan yang sedang diproses ikut dibatalkan saat `ctx` Router dibatalkan. SMTP tiruan di lab ini mengabaikan context, jadi emailnya tetap terkirim. Client sungguhan yang menghormati context akan berhenti di tengah, handler mengembalikan error, dan pesan dikirim ulang setelah restart. Graceful shutdown mengurangi pekerjaan yang terpotong; ia tidak menghapus kemungkinan pesan diproses dua kali. Lihat [[09 Lab - At-Least-Once Delivery]].
4. **Handler tidak mencatat apa pun dan tidak membawa correlation ID.** Menambahkannya ke setiap handler satu per satu adalah pekerjaan rutin jenis baru. Lihat [[05 Lab - Middlewares]].

## Reflection Questions

1. Handler mengembalikan `nil` untuk pesan rusak dan error untuk SMTP yang gagal. Apa kriteria yang membedakan keduanya? Beri satu contoh kegagalan yang tidak jelas masuk golongan mana.
2. Di Step 2, email terkirim pada percobaan ketiga tanpa kode retry. Siapa yang melakukan retry, dan apa yang terjadi kalau Pub/Sub diganti dengan Kafka?
3. Kenapa health check yang hanya menjawab "proses hidup" berbahaya untuk service consumer, tapi relatif aman untuk service HTTP?
4. `email-service` memindahkan pesan yang gagal ke `emails.DLT` dan mencobanya lagi lewat cron. Bandingkan dengan Step 3 lab ini: apa yang dimiliki `email-service` yang belum dimiliki handler kamu?
5. Kubernetes mengirim SIGTERM, menunggu 30 detik, lalu membunuh pod. `CloseTimeout` Router diatur 60 detik dan satu handler butuh 45 detik. Apa yang terjadi pada pesan itu?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Graceful Shutdown]]
- [[Context for Cancellation and Deadlines]]
- [[Term - Poison Message]]
- [[Dead Letter Queues]]
