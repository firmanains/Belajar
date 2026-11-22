# 01 Lab - Basic Async

[[00 Start Here]] · [[02 Lab - Message Broker]]

---

**Goal:** mengubah rantai panggilan synchronous menjadi pekerjaan background dengan goroutine dan queue di memori, lalu melihat sendiri apa yang masih bisa hilang.
**Covers:** sync vs async, background worker, retry, graceful shutdown.
**Prerequisites:** goroutine dan channel dasar. Tidak perlu pengetahuan event-driven.
**Infrastructure:** none.
**Time:** 2–3 jam.

---

## Why This Exists

Seorang tamu memesan dua kamar deluxe. PMS harus melakukan tiga hal: menyimpan booking, mengirim confirmation email, dan memberi tahu channel manager bahwa sisa kamar berkurang.

Cara paling sederhana adalah mengerjakan ketiganya berurutan di dalam satu request. Cara itu mengikat nasib tamu ke dua sistem yang tidak kamu kuasai. Kalau SMTP butuh 30 detik, tamu menatap layar loading selama 30 detik. Kalau SMTP mati, tamu melihat "booking gagal", padahal kamarnya ada dan datanya sudah tersimpan.

Aplikasi di kantormu menghindari masalah itu dengan memisahkan pengiriman email dari request: aplikasi hanya menitipkan email, dan `email-service` yang mengirimnya belakangan. Lab ini membangun versi terkecil dari ide itu, tanpa Kafka, supaya masalah yang kelak diselesaikan broker terlihat satu per satu.

## Concepts

### Synchronous: pemanggil ikut menunggu

```
tamu ──► CreateBooking
            ├─ simpan booking        5 ms
            ├─ kirim email (SMTP)    30.000 ms   ◄── tamu menunggu di sini
            └─ notify channel mgr    800 ms
         ◄── "Booking berhasil" setelah ±31 detik
```

Dalam panggilan synchronous, pemanggil baru mendapat jawaban setelah semua langkah selesai. Akibatnya ada dua. Waktu tunggu tamu adalah jumlah waktu semua langkah. Kegagalan satu langkah menjadi kegagalan seluruh request.

### Asynchronous: pekerjaan dititipkan

Pertanyaan yang memisahkan keduanya: langkah mana yang harus selesai sebelum tamu boleh mendapat jawaban? Hanya satu, yaitu booking tersimpan. Email dan notifikasi boleh terjadi beberapa detik kemudian.

```
tamu ──► CreateBooking
            ├─ simpan booking
            ├─ titip "kirim email"      ──┐
            └─ titip "notify channel"   ──┤   queue (channel Go)
         ◄── "Booking berhasil"           │
                                          ▼
                                   worker goroutine
                                   mengerjakan satu per satu,
                                   mencoba lagi kalau gagal
```

Padanan terdekatnya di luar software adalah resepsionis dan bagian housekeeping. Resepsionis menerima tamu, menulis permintaan handuk tambahan di buku tugas, lalu melayani tamu berikutnya. Ia tidak naik sendiri ke lantai lima. Yang tidak ditangkap analogi ini: buku tugas hotel tetap ada kalau listrik padam, sedangkan queue di lab ini hidup di memori proses dan ikut hilang saat proses mati.

### Tiga hal yang berubah begitu pekerjaan menjadi async

**Error tidak punya tujuan.** Saat email gagal, tamu sudah menerima "Booking berhasil". Tidak ada lagi request yang bisa diberi tahu. Worker harus memutuskan sendiri: mencoba lagi, atau menyerah dan mencatatnya.

**Context dari request tidak boleh dipakai.** `ctx` milik request dibatalkan begitu response terkirim. Pekerjaan background yang memakai `ctx` itu akan dibatalkan tepat saat ia mulai. Worker butuh context sendiri.

**Mematikan proses menjadi keputusan.** Selama semuanya synchronous, tidak ada pekerjaan yang "sedang menunggu". Begitu ada queue, proses yang dimatikan saat deploy membawa serta isi queue itu, kecuali ia diberi kesempatan menghabiskannya dulu.

### Retry

Kegagalan SMTP sering bersifat sementara: timeout, koneksi diputus, server sedang sibuk. Mencoba lagi beberapa saat kemudian biasanya berhasil. Dua angka mengatur retry: berapa kali mencoba (`MaxAttempts`) dan berapa lama jeda antar percobaan (`RetryDelay`). Tanpa batas percobaan, satu email ke alamat yang salah akan dicoba selamanya.

## The Exercise

Kode ada di `labs/01-basic-async/`.

| File | Isi | Diubah? |
|---|---|---|
| `deps.go` | `Booking`, `Emailer`, `ChannelNotifier`, `Config`, `BookingStore` | Tidak |
| `service.go` | `BookingService` versi synchronous | Ya |
| `service_test.go` | Lima skenario, dengan SMTP tiruan yang bisa ditahan dan digagalkan | Tidak |

Signature `NewBookingService`, `CreateBooking`, dan `Shutdown` jangan diubah. Isi struct dan isi fungsinya bebas.

### Step 0: The Happy Path

```bash
cd labs/01-basic-async
go test -run Step0 -v
```

Hasilnya `PASS`. Selama SMTP dan channel manager cepat dan sehat, versi synchronous tidak punya masalah. Baca `service.go` sampai kamu bisa menunjuk baris tempat tamu menunggu SMTP.

### Step 1: A Slow SMTP Server

```bash
go test -run Step1 -v
```

Test menahan SMTP tiruan sampai test selesai, lalu memeriksa apakah `CreateBooking` kembali dalam satu detik. Tebak dulu hasilnya sebelum menjalankan.

Pesan gagalnya: `CreateBooking belum kembali setelah 1 detik: tamu ikut menunggu SMTP yang lambat`.

**Task:** buat `CreateBooking` kembali segera setelah booking tersimpan. Email dan notifikasi dikerjakan oleh goroutine lain.

> [!hint]- Hint
> Buat satu channel berisi pekerjaan, misalnya `chan job` dengan `job` berupa struct yang memegang `func(context.Context) error`. `NewBookingService` menjalankan satu goroutine yang melakukan `for j := range s.jobs`. `CreateBooking` cukup mengirim ke channel itu. Beri channel buffer supaya pengiriman tidak menunggu worker.

### Step 2: SMTP Is Down

```bash
go test -run Step2 -v
```

SMTP selalu gagal. Test menuntut dua hal: `CreateBooking` tetap mengembalikan `nil`, dan channel manager tetap diberi tahu.

Di kode awal pesan gagalnya: `CreateBooking gagal karena SMTP mati, padahal kamarnya ada`. Setelah Step 1, kamu mungkin melihat pesan lain: `kegagalan email tidak boleh menahan pekerjaan lain`. Itu terjadi kalau email dan notifikasi dititipkan sebagai satu pekerjaan yang berhenti di error pertama.

**Task:** pisahkan email dan notifikasi menjadi dua pekerjaan yang berdiri sendiri.

> [!hint]- Hint
> Titipkan dua `job` ke queue, satu untuk email dan satu untuk notifikasi. Worker mencatat error sebuah job dengan `log.Printf`, lalu lanjut ke job berikutnya.

### Step 3: SMTP Recovers After Two Failures

```bash
go test -run Step3 -v
```

SMTP gagal dua kali, lalu berhasil. Test memakai `Config{MaxAttempts: 5, RetryDelay: time.Millisecond}` dan menuntut email terkirim tepat sekali pada percobaan ketiga.

**Task:** worker mencoba lagi sampai berhasil atau sampai `MaxAttempts` tercapai, dengan jeda `RetryDelay` di antaranya.

> [!hint]- Hint
> Bungkus pemanggilan job dalam loop `for attempt := 1; ; attempt++`. Keluar saat `err == nil`. Menyerah saat `attempt >= MaxAttempts`. Di antaranya `time.Sleep(s.cfg.RetryDelay)`. Jalankan job dengan `context.Background()`, bukan dengan `ctx` dari `CreateBooking`.

### Step 4: Deploying While Work Is Queued

```bash
go test -run Step4 -v
```

Lima booking masuk saat SMTP tertahan, jadi lima email menunggu di queue. SMTP pulih, lalu `Shutdown` langsung dipanggil. Begitu `Shutdown` kembali, test menghitung email tanpa menunggu lagi.

Kalau `Shutdown` belum menunggu worker, pesan gagalnya: `setelah Shutdown kembali, email terkirim 0 dari 5: sisanya hilang bersama proses`. Angkanya bisa berbeda di tiap percobaan.

**Task:** `Shutdown` menutup queue, menunggu worker menghabiskan isinya, dan menyerah kalau `ctx` habis lebih dulu.

> [!hint]- Hint
> `close(s.jobs)` membuat `for range` di worker berakhir setelah isi channel habis. Pakai `sync.WaitGroup` untuk menunggu worker selesai. Supaya tetap menghormati `ctx`, tunggu `wg.Wait()` di goroutine kecil yang menutup channel `done`, lalu `select` antara `done` dan `ctx.Done()`. Mengirim ke channel yang sudah ditutup menyebabkan panic, jadi `CreateBooking` setelah `Shutdown` harus ditolak dengan error.

### Finish

```bash
go test -race -v
```

Lima test harus `PASS`, tanpa peringatan data race.

## Solution

> [!success]- Solution
> Ide kuncinya: `CreateBooking` hanya mengerjakan yang harus selesai sebelum tamu mendapat jawaban. Sisanya menjadi dua job terpisah di queue.
>
> ```go
> func (s *BookingService) worker() {
> 	defer s.wg.Done()
> 	for j := range s.jobs {
> 		if err := s.runWithRetry(j); err != nil {
> 			log.Printf("job dropped: %v", err)
> 		}
> 	}
> }
>
> func (s *BookingService) runWithRetry(j job) error {
> 	ctx := context.Background() // bukan ctx milik request
> 	maxAttempts := max(s.cfg.MaxAttempts, 1)
> 	for attempt := 1; ; attempt++ {
> 		err := j.run(ctx)
> 		if err == nil {
> 			return nil
> 		}
> 		if attempt >= maxAttempts {
> 			return fmt.Errorf("%s: gave up after %d attempts: %w", j.name, attempt, err)
> 		}
> 		time.Sleep(s.cfg.RetryDelay)
> 	}
> }
> ```
>
> `Shutdown` menandai service sebagai tertutup di bawah mutex, menutup channel, lalu menunggu `WaitGroup` dengan batas `ctx`. Kode lengkap: `labs/01-basic-async/_solusi/service.go`.

## When This Is the Wrong Choice

Mengubah sesuatu menjadi async adalah pilihan yang salah kalau pemanggil butuh hasilnya untuk melanjutkan. Mengecek sisa kamar sebelum menerima booking harus synchronous: jawaban "berhasil" yang dikirim sebelum pengecekan selesai adalah janji yang belum tentu bisa ditepati.

Async juga memindahkan kegagalan ke tempat yang lebih sulit dilihat. Dalam versi synchronous, SMTP yang mati langsung terlihat sebagai error di layar. Dalam versi async, semua request tampak sehat sementara email menumpuk diam-diam. Tanpa pemantauan panjang queue dan jumlah job yang dibuang, kamu baru tahu ada masalah saat tamu menelepon. Topik itu dibahas di [[17 Lab - Metrics and Alerting]].

Untuk sistem kecil dengan satu langkah lambat yang jarang gagal, panggilan synchronous dengan timeout yang ketat sering kali cukup dan jauh lebih mudah dipahami.

## What Is Still Broken

1. **Queue hilang saat proses mati.** `Shutdown` hanya menolong pada penghentian yang sopan. Kalau proses dibunuh, kehabisan memori, atau servernya mati listrik, semua job di channel lenyap tanpa jejak. Tamu sudah menerima "Booking berhasil", dan tidak ada yang tahu emailnya belum terkirim. [[02 Lab - Message Broker]] memindahkan queue ke luar proses.
2. **Job yang menyerah hanya tercatat di log.** Setelah `MaxAttempts`, email dibuang. Tidak ada tempat untuk menyimpannya dan mencobanya lagi besok. Itu peran dead letter queue di [[19 Lab - Fault Tolerance]].
3. **Satu job yang sedang retry menahan semua job di belakangnya.** Worker hanya satu dan ia tidur di antara percobaan. Klasifikasi error dan retry yang tidak menyumbat antrean dibahas di [[06 Lab - Errors]].
4. **Retry bisa mengirim email dua kali.** Kalau SMTP menerima email tapi koneksinya putus sebelum menjawab, worker menganggapnya gagal dan mengirim lagi. Itu topik [[09 Lab - At-Least-Once Delivery]].

## Reflection Questions

1. Dari tiga langkah di `CreateBooking`, kenapa hanya penyimpanan booking yang tetap synchronous? Apa yang rusak kalau itu juga dibuat async?
2. Kenapa worker tidak boleh memakai `ctx` yang diterima `CreateBooking`?
3. Service ini di-deploy ulang lewat rolling update di Kubernetes, dan pod lama diberi waktu 30 detik sebelum dibunuh. Queue berisi 200 email dan SMTP sedang lambat, 1 detik per email. Apa yang terjadi pada email ke-31 dan seterusnya?
4. Aplikasi di kantormu menitipkan email ke Kafka, bukan ke channel Go di dalam proses PHP atau Java-nya. Kerugian apa dari lab ini yang dihindari oleh keputusan itu?

**My answers:**

1.
2.
3.
4.

## Read Next in the Vault

- [[Synchronous vs Asynchronous Communication]]
- [[Worker Pools]]
- [[Buffered vs Unbuffered Channels]]
- [[Retries with Exponential Backoff and Jitter]]
- [[Graceful Shutdown]]
