# 05 Lab - Middlewares

[[04 Lab - Watermill Basics]] · [[00 Start Here]] · [[06 Lab - Errors]]

---

**Goal:** menulis dua middleware sendiri (correlation ID dan structured logging), lalu melihat kenapa urutan pemasangannya menentukan hasil.

**Covers:** middleware sebagai pembungkus handler, cross-cutting concern, correlation ID di metadata dan di context, logging dengan `log/slog`, dependency lewat closure, urutan middleware.

**Prerequisites:** [[04 Lab - Watermill Basics]]. Kamu sudah pernah memakai `router.AddHandler`, ack/nack, dan `gochannel`.

**Infrastructure:** none.

**Time:** 2–3 jam.

---

## Why This Exists

Seorang tamu menelepon: booking B-1 berhasil, tetapi email konfirmasinya tidak pernah datang. Booking itu melewati tiga tempat: PMS yang menerima booking, handler yang mengurangi availability, dan layanan email. Masing-masing menulis log sendiri.

Kamu membuka ketiga log itu dan mencari "B-1". Log PMS menyebut booking ID. Log layanan email hanya menyebut alamat penerima. Mencocokkan keduanya berarti membandingkan jam dan menebak. Kalau kamu pernah melacak satu email dari log aplikasi ke log `email-service`, kamu sudah merasakan pekerjaan ini.

Obatnya dikenal luas: satu ID yang lahir saat request masuk dan ikut ke setiap message dan setiap baris log. ID itu disebut correlation ID. Masalah berikutnya adalah siapa yang menulis kodenya. Setiap handler harus membaca ID dari message masuk, menaruhnya di log, dan menyalinnya ke message keluar. Dengan tiga puluh handler, itu tiga puluh salinan kode yang sama, dan satu handler yang lupa sudah cukup untuk memutus jejak.

Kebutuhan seperti ini disebut cross-cutting concern: bukan bagian dari logika bisnis handler mana pun, tetapi dibutuhkan oleh semuanya. Logging, retry, metrics, dan tracing termasuk di dalamnya. Middleware adalah tempat menulisnya satu kali.

## Concepts

### Recap Lab 04

`message.Router` menjalankan consume loop untukmu. Kamu mendaftarkan handler dengan `router.AddHandler`, dan router memanggilnya untuk setiap message. Handler yang mengembalikan `nil` berarti ack; handler yang mengembalikan error berarti nack, dan `gochannel` mengirim ulang message itu.

### Middleware adalah fungsi yang membungkus handler

Handler di Watermill punya satu bentuk:

```go
type HandlerFunc func(msg *Message) ([]*Message, error)
```

Middleware menerima handler dan mengembalikan handler baru dengan bentuk yang sama:

```go
type HandlerMiddleware func(h HandlerFunc) HandlerFunc
```

Karena bentuk masuk dan keluarnya sama, router tidak bisa membedakan handler asli dari handler yang sudah dibungkus. Itu sebabnya middleware bisa ditumpuk sebanyak apa pun.

```go
func Example(h message.HandlerFunc) message.HandlerFunc {
	return func(msg *message.Message) ([]*message.Message, error) {
		// kode di sini berjalan sebelum handler
		produced, err := h(msg)
		// kode di sini berjalan sesudah handler, dan bisa melihat hasilnya
		return produced, err
	}
}
```

Padanan terdekatnya adalah petugas di lobi gedung kantor. Ia mencatat siapa yang masuk, memberi kartu tamu, lalu mencatat lagi saat orang itu keluar. Ruangan yang dituju tidak perlu tahu petugas itu ada.

Perbedaan pentingnya: petugas lobi tidak bisa mengubah hasil rapat. Middleware bisa. Ia boleh mengubah message keluar, menelan error, memanggil handler tiga kali, atau tidak memanggilnya sama sekali. Kekuatan itu yang membuat retry di [[06 Lab - Errors]] mungkin, dan kekuatan itu juga yang membuat middleware berbahaya kalau dipakai sembarangan.

### Correlation ID tinggal di dua tempat

| Tempat | Jangkauan | Dipakai oleh |
|---|---|---|
| `msg.Metadata` | Antar proses. Metadata ikut terkirim bersama message lewat broker. | Service berikutnya |
| `context.Context` | Di dalam satu proses, selama satu message diproses. | Handler, dan semua yang handler panggil dengan `ctx` |

Metadata saja tidak cukup. Handler memanggil `availability.Decrease(ctx, ...)`, dan fungsi itu tidak menerima message; ia hanya menerima `ctx`. Kalau ID tidak ada di context, HTTP call dan query yang dilakukan handler tidak bisa menyertakannya.

Context saja juga tidak cukup, karena context tidak menyeberang ke proses lain. Middleware correlation ID menjembatani keduanya: baca dari metadata, taruh di context, lalu tulis lagi ke metadata setiap message keluar.

```
message masuk                                              message keluar
metadata: correlation_id=req-123                           metadata: correlation_id=req-123
        │                                                          ▲
        ▼                                                          │
  ┌─ CorrelationID ──────────────────────────────────────────────────┐
  │   ctx = ctx + req-123                                            │
  │   ┌─ handler ─────────────────────────────────────────────┐     │
  │   │  availability.Decrease(ctx, ...)   ← ctx membawa ID    │     │
  │   └───────────────────────────────────────────────────────┘     │
  └──────────────────────────────────────────────────────────────────┘
```

### Dependency masuk lewat closure

Middleware logging butuh logger. Bentuk `HandlerMiddleware` tidak punya parameter untuk itu, jadi logger diberikan satu tingkat di luarnya:

```go
func Logging(logger *slog.Logger) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) { ... }
	}
}
```

Ada tiga lapis fungsi di sini, dan masing-masing berjalan pada waktu yang berbeda. Lapis terluar berjalan sekali saat service dirakit dan menerima dependency. Lapis tengah berjalan sekali per handler. Lapis terdalam berjalan sekali per message. Middleware bawaan Watermill yang butuh banyak pengaturan memakai variasi lain dari ide yang sama: sebuah struct berisi config dengan method `Middleware`, misalnya `middleware.Retry{MaxRetries: 3}.Middleware`.

### Urutan menentukan siapa membungkus siapa

`router.AddMiddleware(A, B)` membuat `A` menjadi lapisan terluar. Message melewati `A`, lalu `B`, lalu handler, dan keluar dengan urutan terbalik.

```
AddMiddleware(A, B)

masuk  ──► A (sebelum) ──► B (sebelum) ──► handler
keluar ◄── A (sesudah) ◄── B (sesudah) ◄──┘
```

Middleware di lapisan luar tidak melihat apa yang disiapkan lapisan dalam pada saat message masuk. Step 4 membuatmu menabrak akibatnya.

`router.AddMiddleware` berlaku untuk semua handler di router. Untuk satu handler saja, `router.AddHandler` mengembalikan `*message.Handler` yang punya method `AddMiddleware` sendiri.

## The Exercise

Kode ada di `labs/05-middlewares/`.

| File | Isi | Diubah? |
|---|---|---|
| `service.go` | Router dengan satu handler `reserve-rooms`: terima `BookingCreated`, kurangi availability, terbitkan `SendConfirmationEmail` | Tidak |
| `middleware.go` | `CorrelationID`, `Logging`, dan `Middlewares` | Ya |
| `middleware_test.go` | Lima test | Tidak |

Baca `service.go` dulu. Perhatikan bahwa handler tidak menyebut correlation ID atau logger sama sekali.

### Step 0: Handler Works Without Middleware

```bash
cd ~/Developer/playground/event-driven/labs/05-middlewares
go test -run Step0 -v
```

Hasilnya `PASS`. Kedua middleware di `middleware.go` masih mengembalikan handler apa adanya, dan handler tetap bekerja. Middleware menambah perilaku; ia bukan syarat agar handler jalan.

### Step 1: Propagate the Correlation ID

```bash
go test -run Step1 -v
```

Test menerbitkan `BookingCreated` dengan metadata `correlation_id=req-123`, lalu memeriksa message `SendConfirmationEmail` yang keluar. Tebak dulu: apa isi metadata message keluar itu?

Pesan gagalnya:

```
correlation_id di message keluar = "", seharusnya "req-123": jejak request putus di handler ini
```

Handler membuat message baru dengan `message.NewMessage`, dan message baru lahir dengan metadata kosong. Tidak ada yang menyalin ID.

**Task:** ubah `CorrelationID` supaya setiap message yang dikembalikan handler membawa `correlation_id` yang sama dengan message masuk.

> [!hint]- Hint
> Panggil `h(msg)` dulu dan simpan hasilnya. Hasil pertama adalah slice message keluar. Isi metadata tiap elemen dengan `out.Metadata.Set(CorrelationIDMetadataKey, id)`, lalu kembalikan slice dan error-nya tanpa diubah.

### Step 2: Put the ID in the Context

```bash
go test -run Step2 -v
```

Test ini punya dua bagian. Bagian pertama memeriksa bahwa `Availability.Decrease` bisa membaca `req-123` dari `ctx`. Bagian kedua menerbitkan message tanpa correlation ID dan menuntut middleware membuatkan ID baru.

```
Availability.Decrease melihat correlation ID [""] di context, seharusnya ["req-123"]
```

**Task:** lengkapi `CorrelationIDFromContext`, lalu buat `CorrelationID` menaruh ID di context message sebelum memanggil handler. Kalau message masuk tidak membawa ID, buat satu dengan `watermill.NewUUID()`.

> [!hint]- Hint
> Context message dibaca dengan `msg.Context()` dan diganti dengan `msg.SetContext(...)`. Pakai `context.WithValue` dengan tipe kunci privat, misalnya `type correlationIDKey struct{}`. Kunci bertipe string bisa bertabrakan dengan package lain.

### Step 3: Write Structured Log Records

```bash
go test -run Step3 -v
```

Di test ini `Availability.Decrease` gagal satu kali, jadi message diproses dua kali. Test membaca log sebagai JSON dan menuntut:

| Baris | Level | Kapan | Field wajib |
|---|---|---|---|
| `message received` | INFO | Sebelum handler | `message_uuid`, `handler` |
| `message handled` | INFO | Handler berhasil | `message_uuid`, `handler` |
| `message failed` | ERROR | Handler gagal | `message_uuid`, `handler`, `error` |

Tebak dulu berapa baris `message received` yang akan muncul untuk satu message ini.

**Task:** tulis `Logging`. Nama handler tersedia lewat `message.HandlerNameFromCtx(msg.Context())`. Error harus tetap dikembalikan apa adanya; middleware logging mengamati, tidak memutuskan.

> [!hint]- Hint
> `logger.With("message_uuid", msg.UUID, "handler", ...)` menghasilkan logger baru yang selalu menyertakan field itu. Buat satu kali di awal, lalu pakai untuk ketiga baris. Isi field `error` dengan `err.Error()`.

### Step 4: Fix the Order

```bash
go test -run Step4 -v
```

Message masuk tanpa correlation ID. Test menuntut **kedua** baris log, termasuk `message received`, membawa `correlation_id` yang sama dengan message keluar.

Tambahkan dulu field `correlation_id` ke logger di `Logging`, diisi dari `CorrelationIDFromContext`. Jalankan test lagi. Pesannya berubah menjadi:

```
baris "message received" membawa correlation_id , seharusnya "…": Logging berjalan sebelum CorrelationID sempat membuat ID
```

Kode kedua middleware sudah benar. Yang salah adalah susunannya.

**Task:** perbaiki `Middlewares`.

> [!hint]- Hint
> Elemen pertama slice menjadi lapisan terluar. `Logging` menulis baris pertamanya saat message masuk, jadi pada saat itu ID harus sudah ada di context. Middleware mana yang harus berada di luar?

Setelah itu jalankan semuanya:

```bash
go test -race -v
```

## Solution

> [!success]- Solution
> Ide kuncinya ada tiga. `CorrelationID` bekerja di dua sisi handler: sebelum memanggilnya ia mengisi context, sesudahnya ia mengisi metadata message keluar. `Logging` menerima logger lewat closure dan tidak pernah mengubah hasil handler. `CorrelationID` dipasang di luar `Logging`.
>
> ```go
> type correlationIDKey struct{}
>
> func CorrelationID(h message.HandlerFunc) message.HandlerFunc {
> 	return func(msg *message.Message) ([]*message.Message, error) {
> 		id := msg.Metadata.Get(CorrelationIDMetadataKey)
> 		if id == "" {
> 			id = watermill.NewUUID()
> 		}
> 		msg.SetContext(context.WithValue(msg.Context(), correlationIDKey{}, id))
>
> 		produced, err := h(msg)
> 		for _, out := range produced {
> 			out.Metadata.Set(CorrelationIDMetadataKey, id)
> 		}
> 		return produced, err
> 	}
> }
>
> func Middlewares(logger *slog.Logger) []message.HandlerMiddleware {
> 	return []message.HandlerMiddleware{CorrelationID, Logging(logger)}
> }
> ```
>
> Kode lengkap, termasuk `Logging`, ada di `labs/05-middlewares/_solusi/middleware.go`.

Watermill sudah membawa `middleware.CorrelationID` di package `message/router/middleware`. Middleware bawaan itu hanya menyalin metadata dari message masuk ke message keluar. Ia tidak membuat ID baru dan tidak menyentuh context, jadi versi yang kamu tulis melakukan lebih banyak.

## When This Is the Wrong Choice

Middleware cocok untuk perilaku yang sama di semua handler dan tidak peduli isi payload. Di luar itu ia merugikan.

Logika bisnis di dalam middleware adalah kesalahan yang paling sering. Aturan seperti "booking dari channel tertentu tidak dikirimi email" terlihat rapi sebagai middleware, tetapi orang yang membaca handler tidak akan menemukannya. Aturan bisnis tempatnya di handler, di mana ia terlihat.

Middleware yang harus membongkar payload untuk bekerja juga tanda salah tempat. Begitu ia melakukan `json.Unmarshal` ke struct tertentu, ia hanya berlaku untuk satu jenis message dan seharusnya menjadi bagian dari handler itu.

Logging isi payload di middleware terasa membantu saat debugging dan menjadi masalah di production. Payload booking berisi nama dan email tamu, dan volume log tumbuh sebanding dengan trafik. Catat ID dan metadata, bukan isi.

Terakhir, tumpukan middleware yang panjang membuat perilaku service bergantung pada urutan yang tidak tertulis di mana pun selain satu slice. Step 4 menunjukkan betapa sunyinya kegagalan akibat urutan yang salah: tidak ada error, hanya field yang kosong.

## What Is Still Broken

1. **ID yang dibuat di consumer berubah setiap redelivery.** Kalau message masuk tanpa ID dan handler gagal, `gochannel` mengirim ulang salinan message asli, yang tetap tanpa ID. Middleware membuat ID baru untuk percobaan kedua. ID seharusnya lahir di tepi sistem, saat request pertama masuk, dan ditulis ke message oleh pengirimnya. Envelope dari [[03 Lab - Events]] adalah tempatnya.
2. **Message yang diterbitkan lewat publisher tidak ikut.** Middleware ini hanya melihat message yang dikembalikan handler. Handler yang memanggil `publisher.Publish` sendiri, seperti event bus di [[08 Lab - Events with CQRS]], lolos dari penyalinan ID. Jejak yang menyeberang dengan benar dibahas di [[18 Lab - Tracing]].
3. **Handler yang gagal diulang tanpa batas.** Step 3 hanya mencatat kegagalan. Kalau `Availability` mati terus, message yang sama berputar selamanya dan log terisi baris ERROR. Itu pekerjaan [[06 Lab - Errors]].

## Reflection Questions

1. Kenapa correlation ID harus ada di metadata **dan** di context? Apa yang putus kalau salah satunya dihilangkan?
2. `Logging` menulis baris `message received` sebelum memanggil handler. Kalau baris itu dihapus dan hanya baris akhir yang tersisa, apakah urutan middleware masih berpengaruh pada test Step 4? Jelaskan dengan mengikuti jalannya satu message.
3. Sebuah email konfirmasi diminta oleh aplikasi di kantormu, lewat Kafka, lalu dikirim `email-service`. Di titik mana correlation ID seharusnya dibuat, dan lewat bagian mana dari record Kafka ia dibawa ke layanan email?
4. Kamu diminta menambah aturan "jangan proses booking dengan `rooms` lebih dari 20". Middleware atau handler? Apa alasannya?

**My answers:**

1.
2.
3.
4.

## Read Next in the Vault

- [[Correlation IDs]]
- [[Structured Logging and Log Levels]]
- [[net-http Handlers and Middleware]] (pola yang sama untuk HTTP)
- [[Context Propagation in HTTP Servers]]
- [[Case - Log Volume That Costs More Than The Servers]]
