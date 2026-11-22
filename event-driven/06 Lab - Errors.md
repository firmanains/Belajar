# 06 Lab - Errors

[[05 Lab - Middlewares]] · [[00 Start Here]] · [[07 Lab - Component Testing]]

---

**Goal:** membedakan tiga jenis kegagalan handler dan memberi masing-masing perlakuan yang tepat, supaya satu message bermasalah tidak menghentikan semua message di belakangnya.

**Covers:** transient error, malformed message, code bug, retry dengan batas dan backoff, menyingkirkan message yang gagal, logging error satu kali.

**Prerequisites:** [[05 Lab - Middlewares]]. Kamu sudah bisa menulis middleware dan tahu bahwa urutannya penting.

**Infrastructure:** none.

**Time:** 3–4 jam.

---

## Why This Exists

Pukul 19.00 server SMTP berhenti menjawab selama sepuluh menit. Layanan email sedang memegang satu message `SendConfirmationEmail` dan handler-nya mengembalikan error. Apa yang seharusnya terjadi pada message itu?

Tiga jawaban yang paling mudah ditulis semuanya buruk. Membuang message berarti tamu tidak pernah menerima email. Mencoba lagi saat itu juga, terus-menerus, berarti SMTP yang sedang sekarat dihantam ribuan kali per detik. Membiarkan service mati berarti ia hidup lagi, mengambil message yang sama, dan mati lagi.

Ada masalah kedua yang lebih halus. Tidak semua error seperti SMTP timeout. Message dengan JSON terpotong tidak akan pernah berhasil, berapa kali pun dicoba. Message yang memicu bug di kode juga begitu, sampai seseorang memperbaiki kodenya. Handler yang memperlakukan ketiganya sama akan salah untuk minimal dua di antaranya.

`email-service` menjawab pertanyaan ini dengan dead letter topic `emails.DLT` yang dicoba ulang oleh cron. Lab ini membangun versi kecil dari keputusan yang sama, supaya kamu tahu kenapa topic itu ada.

## Concepts

### Recap Lab 04 dan 05

Handler yang mengembalikan error membuat router melakukan nack, dan broker mengirim ulang message itu. Middleware membungkus handler dan bisa mengubah hasilnya, termasuk menelan error atau memanggil handler berkali-kali. Middleware pertama di `AddMiddleware` menjadi lapisan terluar.

### Satu message yang gagal menyumbat antrean

Di Kafka, satu partition dibaca berurutan. Consumer menandai kemajuannya dengan satu angka, yaitu offset. Ia tidak bisa berkata "message 5 gagal, tapi message 6 sudah selesai", karena melakukan commit pada offset 6 berarti menyatakan semua message sebelumnya beres.

```
partition:  [ m4 ✔ ] [ m5 ✘ ] [ m6 ] [ m7 ] [ m8 ]
                        ▲
                        └─ nack, diantar lagi, nack, diantar lagi, ...
                           m6, m7, m8 menunggu selamanya
```

Selama m5 terus gagal, m6 sampai m8 tidak pernah disentuh. Satu email dengan payload rusak cukup untuk menghentikan semua email berikutnya.

`gochannel` tidak punya sifat ini, karena ia mengantar setiap message secara terpisah. Lab ini memakai `Broker` di `broker.go`: broker tiruan di memory yang mengantar satu message pada satu waktu dan mengulanginya sampai ack, seperti satu partition Kafka. Bacalah fungsi `deliver`; isinya dua puluh baris.

### Tiga jenis kegagalan

| Jenis | Contoh | Apakah mengulang membantu? | Perlakuan |
|---|---|---|---|
| Transient | SMTP timeout, koneksi database putus | Ya, setelah menunggu | Retry dengan jeda, dibatasi |
| Malformed message | JSON terpotong, field wajib kosong | Tidak akan pernah | Singkirkan saat itu juga |
| Code bug | nil pointer pada kombinasi data tertentu | Tidak, sampai kode diperbaiki | Singkirkan, simpan untuk diproses ulang setelah perbaikan |

Pikirkan kasir dengan satu jalur antrean. Kartu pembeli ditolak mesin: kasir mencoba sekali atau dua kali lagi, karena mesin kadang lambat. Pembeli ternyata tidak membawa dompet: mencoba lagi tidak ada gunanya, jadi ia diminta menepi supaya antrean jalan. Mesin kasir rusak setiap kali memindai satu jenis barang: pembeli itu juga menepi, dan teknisi dipanggil.

Yang tidak ditangkap analogi ini: kasir melihat sendiri apa yang salah. Kode retry hanya menerima satu nilai `error`, dan semua error tampak sama. Pengetahuan bahwa sebuah kegagalan itu permanen harus ditulis ke dalam error-nya oleh pihak yang tahu, yaitu handler. Di lab ini caranya adalah membungkus error dengan `Permanent(err)`.

### Retry: dibatasi dan makin sabar

Retry tanpa jeda memperburuk keadaan dependency yang sedang kesulitan. Backoff berarti jeda yang makin panjang di tiap percobaan: 20 ms, 40 ms, 80 ms. Dependency mendapat waktu bernapas, dan gangguan singkat tetap tertangani cepat.

Retry tanpa batas kembali ke masalah antrean tersumbat, hanya lebih lambat. Jadi jumlah percobaan dibatasi, dan setelah batas itu message harus pergi ke tempat lain.

### Menyingkirkan message berarti mengambil tanggung jawab

Untuk membuat broker maju, handler harus mengembalikan `nil`, yaitu ack. Ack adalah pernyataan "message ini sudah kuurus". Pernyataan itu hanya jujur kalau message sudah disimpan di tempat lain lebih dulu. Urutannya tidak boleh dibalik: simpan, baru ack.

Di lab ini tempat lain itu adalah `ParkingLot`, sebuah slice di memory. Bentuk production-nya adalah dead letter queue, yang dibahas di [[19 Lab - Fault Tolerance]].

### Susunan middleware

```
        ┌─ Park ──────────────────────────────────────────────┐
        │  error apa pun yang sampai ke sini sudah final:      │
        │  simpan ke ParkingLot, log ERROR sekali, lalu ack    │
        │  ┌─ Retry ───────────────────────────────────────┐  │
        │  │  error transient: tunggu, coba lagi, maks N    │  │
        │  │  error permanent atau panic: langsung naik     │  │
        │  │  ┌─ Recoverer ─────────────────────────────┐  │  │
        │  │  │  panic  →  error biasa                   │  │  │
        │  │  │  ┌─ handler ─────────────────────────┐  │  │  │
        │  │  │  └───────────────────────────────────┘  │  │  │
        │  │  └─────────────────────────────────────────┘  │  │
        │  └───────────────────────────────────────────────┘  │
        └─────────────────────────────────────────────────────┘
```

Kamu tidak memasang susunan ini sekaligus. Setiap step menambah satu lapis, karena test berikutnya gagal tanpanya.

## The Exercise

Kode ada di `labs/06-errors/`.

| File | Isi | Diubah? |
|---|---|---|
| `broker.go` | Broker tiruan: satu partition per topic, redelivery sampai ack | Tidak |
| `email.go` | Tipe message, `Emailer`, `Permanent`, `ParkingLot`, perakitan router, dan `buildSubject` yang sengaja punya bug | Tidak |
| `handler.go` | `EmailHandler.Handle` | Ya, di Step 3 |
| `middleware.go` | `Retry`, `Park`, `Middlewares` | Ya |
| `errors_test.go` | Enam test | Tidak |

Config retry di semua test: `MaxAttempts: 3`, `InitialInterval: 20ms`, `Multiplier: 2`.

### Step 0: A Healthy Message Is Sent

```bash
cd ~/Developer/belajar/event-driven/labs/06-errors
go test -run Step0 -v
```

`PASS`. SMTP sehat, payload benar, email terkirim.

### Step 1: Retry Transient Errors with Backoff

```bash
go test -run Step1 -v
```

SMTP tiruan gagal dua kali dengan `ErrSMTPTimeout`, lalu pulih. Tebak dulu: tanpa middleware apa pun, apakah email akhirnya terkirim?

```
jeda percobaan 1 ke 2 = 1.1595ms, seharusnya minimal 20ms: SMTP yang sedang sekarat dihantam tanpa jeda
```

Email memang terkirim, karena broker mengantar ulang setiap message yang mendapat nack. Itulah retry yang kamu dapat gratis: seketika dan tanpa batas.

**Task:** tulis `Retry`. Ia memanggil handler sampai berhasil atau sampai `cfg.MaxAttempts` percobaan, menunggu `cfg.InitialInterval` sebelum percobaan kedua, dan mengalikan jeda dengan `cfg.Multiplier` setiap kali. Setelah jatah habis, kembalikan error terakhir. Lalu pasang `Retry` di `Middlewares`.

> [!hint]- Hint
> Sebuah `for` dengan penghitung percobaan sudah cukup. Untuk menunggu, pakai `select` dengan dua cabang: `time.After(delay)` dan `msg.Context().Done()`. Cabang kedua membuat service bisa berhenti di tengah jeda; kembalikan `msg.Context().Err()` di sana.

### Step 2: Do Not Let Exhausted Retries Block the Queue

```bash
go test -run Step2 -v
```

SMTP mati total untuk alamat `mati@example.com`. Test menerbitkan B-1 ke alamat itu, lalu B-2 ke alamat yang sehat.

```
email untuk message berikutnya (B-2) terkirim; antrean tersumbat oleh message di depannya: tidak terjadi dalam 2s
```

`Retry` menyerah setelah tiga percobaan dan mengembalikan error. Router melakukan nack, broker mengantar ulang, dan tiga percobaan berikutnya dimulai. B-2 tidak pernah mendapat giliran.

**Task:** tulis `Park`. Kalau handler di dalamnya mengembalikan error, simpan message ke `lot` dengan `lot.Add(msg, err)`, lalu kembalikan `nil, nil` supaya message mendapat ack. Pasang `Park` di `Middlewares`, di luar `Retry`.

> [!hint]- Hint
> `Park` tidak perlu tahu berapa kali `Retry` sudah mencoba. Karena `Park` berada di luar, setiap error yang sampai kepadanya sudah melewati `Retry` dan berarti jatahnya habis. Satu pengecualian: kalau error-nya `context.Canceled`, service sedang berhenti. Kembalikan error itu apa adanya supaya message diantar lagi nanti.

### Step 3: Park Malformed Messages Without Retrying

```bash
go test -run Step3 -v
```

Message pertama berisi JSON terpotong.

```
message rusak diproses 3 kali, seharusnya 1: payload rusak tidak sembuh dengan retry
```

Message itu sudah diparkir, jadi antrean tidak tersumbat. Yang salah adalah tiga percobaan dan dua jeda yang terbuang untuk sesuatu yang mustahil berhasil. Dengan backoff sungguhan, itu berarti beberapa detik per message rusak, dan semua message di belakangnya ikut menunggu.

**Task:** dua perubahan. Di `handler.go`, bungkus error decode dengan `Permanent(...)`. Di `Retry`, jangan mengulang kalau `IsPermanent(err)`.

> [!hint]- Hint
> `Permanent` dan `IsPermanent` sudah ada di `email.go`. `IsPermanent` memakai `errors.As`, jadi ia tetap mengenali error permanen walau sudah dibungkus lagi dengan `fmt.Errorf("...: %w", err)`.

### Step 4: Survive a Code Bug

```bash
go test -run Step4 -v
```

Message pertama punya `"guest": null`. Fungsi `buildSubject` tidak menduga itu dan mengalami panic. Jangan perbaiki `buildSubject`. Anggap kamu belum tahu bug itu ada; tugasmu membuat service selamat dari bug yang belum ditemukan.

```
email untuk message berikutnya (B-2) terkirim; antrean tersumbat oleh message di depannya: tidak terjadi dalam 2s
```

Panic bukan error. Ia tidak dikembalikan oleh `h(msg)`; ia melompati `Retry` dan `Park` sampai ditangkap router, yang lalu melakukan nack. Hasilnya putaran tanpa akhir.

**Task:** tambahkan `middleware.Recoverer` dari Watermill sebagai lapisan terdalam di `Middlewares`. Ia mengubah panic menjadi error bertipe `middleware.RecoveredPanicError`. Jalankan test lagi, baca pesan barunya, lalu buat `Retry` tidak mengulang error jenis itu.

> [!hint]- Hint
> Import-nya `github.com/ThreeDotsLabs/watermill/message/router/middleware`. Untuk mengenali panic: `var panicErr middleware.RecoveredPanicError`, lalu `errors.As(err, &panicErr)`. Gabungkan dengan `IsPermanent` dalam satu fungsi kecil, misalnya `retryable(err) bool`.

### Step 5: Log Each Failure Once

```bash
go test -run Step5 -v
```

Test menghitung baris log untuk message yang gagal total: tepat satu baris ERROR dengan field `message_uuid` dan `error`, dan satu baris WARN untuk setiap percobaan ulang. Message yang berhasil tidak boleh menghasilkan WARN atau ERROR.

**Task:** tambahkan logging ke `Retry` (WARN sebelum menunggu) dan `Park` (ERROR saat memarkir).

> [!hint]- Hint
> ERROR berarti ada yang harus dilakukan manusia. Percobaan ulang yang mungkin berhasil belum memenuhi syarat itu. Hanya `Park` yang tahu sebuah message gagal untuk seterusnya, jadi hanya `Park` yang menulis ERROR.

Setelah itu jalankan semuanya:

```bash
go test -race -v
```

## Solution

> [!success]- Solution
> Ide kuncinya: setiap lapis memutuskan satu hal. Handler memutuskan apakah sebuah error permanen. `Retry` memutuskan apakah mencoba lagi masuk akal. `Park` memutuskan bahwa message harus pergi, dan hanya ia yang melakukan ack atas kegagalan.
>
> ```go
> func retryable(err error) bool {
> 	var panicErr middleware.RecoveredPanicError
> 	return !IsPermanent(err) && !errors.As(err, &panicErr)
> }
>
> // inti Retry
> for attempt := 1; ; attempt++ {
> 	produced, err := h(msg)
> 	if err == nil {
> 		return produced, nil
> 	}
> 	if !retryable(err) || attempt >= cfg.MaxAttempts {
> 		return nil, err
> 	}
> 	logger.Warn("retrying message", "message_uuid", msg.UUID, "attempt", attempt, "error", err.Error())
> 	select {
> 	case <-time.After(delay):
> 	case <-msg.Context().Done():
> 		return nil, msg.Context().Err()
> 	}
> 	delay = time.Duration(float64(delay) * cfg.Multiplier)
> }
>
> // inti Park
> lot.Add(msg, err)
> logger.Error("message parked", "message_uuid", msg.UUID, "error", err.Error())
> return nil, nil
>
> func Middlewares(retry RetryConfig, lot *ParkingLot, logger *slog.Logger) []message.HandlerMiddleware {
> 	return []message.HandlerMiddleware{Park(lot, logger), Retry(retry, logger), middleware.Recoverer}
> }
> ```
>
> Kode lengkap ada di `labs/06-errors/_solusi/`.

Watermill membawa `middleware.Retry` dengan exponential backoff dan `middleware.PoisonQueue` yang menerbitkan message gagal ke topic lain. Di production, pakai keduanya. Kamu menulis sendiri di sini supaya tahu apa yang mereka putuskan untukmu.

## When This Is the Wrong Choice

**Retry di tempat menahan partition selama jeda.** Dengan tiga percobaan dan jeda 20 ms, itu tidak terasa. Dengan jeda sampai lima menit, semua message di belakangnya menunggu lima menit, padahal mungkin tujuannya sehat. Untuk jeda panjang, message dipindahkan ke tempat lain dan dicoba lagi nanti. `emails.DLT` yang dicoba ulang oleh cron adalah bentuk dari pilihan itu.

**Memarkir merusak urutan.** Kalau `BookingCreated` untuk B-1 diparkir dan `BookingCancelled` untuk B-1 lolos, pembatalan diproses sebelum pembuatan. Untuk email konfirmasi, urutan tidak penting. Untuk perubahan availability, urutan menentukan kebenaran. [[13 Lab - Message Ordering]] membahasnya.

**Memarkir saat dependency mati total hanya memindahkan masalah.** Kalau SMTP mati satu jam, setiap message melewati tiga percobaan lalu diparkir. Antrean utama kosong dan tempat parkir berisi ribuan message yang semuanya harus diproses ulang. Dalam keadaan itu yang benar adalah berhenti mengambil message sampai dependency pulih. Itu circuit breaker, di [[19 Lab - Fault Tolerance]].

**Klasifikasi yang salah lebih buruk daripada tanpa klasifikasi.** Menandai error transient sebagai permanen berarti menyerah pada message yang sebetulnya bisa berhasil. Kalau ragu, perlakukan sebagai transient: biayanya beberapa percobaan sia-sia, bukan pekerjaan yang hilang.

## What Is Still Broken

1. **`ParkingLot` hilang saat restart.** Isinya hanya slice di memory. Begitu proses mati, semua message yang diparkir lenyap, padahal broker sudah menerima ack untuknya. [[19 Lab - Fault Tolerance]] menggantinya dengan topic.
2. **Tidak ada yang memproses ulang message yang diparkir**, dan tidak ada yang tahu bahwa tempat parkir mulai terisi. Baris ERROR di log hanya berguna kalau ada yang membacanya. Alert dibahas di [[17 Lab - Metrics and Alerting]].
3. **Handler yang diulang bisa mengerjakan hal yang sama dua kali.** Kalau handler mengirim email lalu gagal di langkah berikutnya, retry mengirim email itu lagi. [[09 Lab - At-Least-Once Delivery]] membahas handler yang idempotent.
4. **Tidak ada jitter.** Seratus consumer yang gagal bersamaan akan mencoba lagi bersamaan juga, tepat 20 ms kemudian.

## Reflection Questions

1. Di Step 1, email terkirim walau `Retry` belum ditulis. Dari mana percobaan ulang itu datang, dan apa dua kekurangannya?
2. `Park` mengembalikan `nil, nil` untuk message yang gagal. Kenapa itu bukan kebohongan kepada broker, dan dalam keadaan apa ia menjadi kebohongan?
3. Kenapa `middleware.Recoverer` harus menjadi lapisan terdalam? Ikuti jalannya satu panic kalau `Recoverer` dipasang paling luar.
4. `email-service` memindahkan email yang gagal ke `emails.DLT` dan sebuah cron mencobanya lagi. Dibanding retry di tempat seperti lab ini, apa yang didapat dan apa yang dikorbankan? Jenis kegagalan mana dari tabel di atas yang akan terus berputar di DLT itu tanpa pernah berhasil?
5. Alamat penerima dipakai sebagai key record di topic email. Kalau satu message untuk sebuah alamat tersumbat, email untuk alamat mana saja yang ikut tertahan?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Retries with Exponential Backoff and Jitter]]
- [[Dead Letter Queues]]
- [[Term - Poison Message]]
- [[Queue vs Log Semantics]]
- [[Error Wrapping]] dan [[Sentinel Errors vs Error Types]]
- [[Case - The Retry Storm That Became a Total Outage]]
