# 07 Lab - Component Testing

[[06 Lab - Errors]] · [[00 Start Here]] · [[08 Lab - Events with CQRS]]

---

**Goal:** menguji satu service utuh sebagai kotak hitam, dengan router dan handler sungguhan, dan menemukan dua bug yang tidak terlihat oleh unit test.

**Covers:** component test, fake untuk dependency luar (`httptest` dan fake emailer), menunggu hasil asinkron dengan helper `eventually`, kenapa `time.Sleep` di test itu salah.

**Prerequisites:** [[04 Lab - Watermill Basics]] dan [[06 Lab - Errors]]. Kamu sudah pernah menulis test dengan package `testing`.

**Infrastructure:** none.

**Time:** 2–3 jam.

---

## Why This Exists

Service sinkronisasi PMS punya tiga handler dan setiap handler punya unit test. Semua test hijau. Service di-deploy, booking pertama masuk, dan tamu tidak menerima email konfirmasi. Tidak ada error di log, karena memang tidak ada yang gagal: handler email tidak pernah dipanggil.

Penyebabnya satu string. Handler email didaftarkan ke topic `bookings.created`, sedangkan event terbit di `booking.created`. Unit test memanggil fungsi handler langsung, jadi ia tidak pernah melewati baris yang salah itu.

Di service event-driven, bagian yang menyambungkan potongan-potongan kode cukup besar: nama topic, subscriber mana untuk handler mana, urutan middleware, bentuk JSON yang disepakati dengan service lain, cara membaca jawaban HTTP. Compiler tidak memeriksa satu pun dari itu. Mendaftar ke topic yang tidak pernah diisi bukan error; ia hanya sunyi.

Component test ada untuk menguji sambungan itu.

## Concepts

### Recap Lab 04

Router dijalankan dengan `router.Run(ctx)`. Channel `router.Running()` tertutup ketika semua handler sudah subscribe. `gochannel` adalah Pub/Sub di memory, dan ia membuang message yang terbit sebelum ada subscriber.

### Garis batasnya adalah proses

Component test menjalankan service apa adanya: router sungguhan, handler sungguhan, wiring sungguhan. Yang diganti hanya apa yang berada di luar proses.

```
                         ┌──────────── service (kode asli, tidak diubah) ───────────┐
 test menerbitkan        │                                                          │   fake channel manager
 BookingCreated  ──────► │  router ──► sync-channel-manager ── POST /availability ──┼──► (httptest.Server)
 lewat gochannel         │        └──► send-confirmation-email ── SendConfirmation ─┼──► fake emailer
                         │                                                          │
                         └──────────────────────────────────────────────────────────┘
        ▲                                                                                    │
        └────────────── test memeriksa apa yang direkam kedua fake ◄────────────────────────┘
```

Test tidak tahu ada berapa handler atau bagaimana namanya. Ia hanya tahu: kalau event ini masuk, akibat itu harus terlihat di luar. Karena itu test tetap berlaku ketika isi service diubah total, selama perilakunya sama.

| | Unit test | Component test | End-to-end test |
|---|---|---|---|
| Yang asli | Satu fungsi | Satu service utuh | Semua service dan infrastruktur |
| Yang diganti | Semua di sekitarnya | Dunia di luar proses | Tidak ada |
| Kecepatan | Milidetik | Puluhan milidetik | Detik sampai menit |
| Menangkap | Logika satu fungsi | Wiring, serialisasi, penanganan error antar bagian | Konfigurasi dan perilaku infrastruktur asli |

### Fake, bukan mock

`httptest.NewServer` menyalakan HTTP server sungguhan di port acak localhost. Kode service memakai `http.Client` aslinya, membuat JSON aslinya, dan menerima status code asli. Tidak ada bagian dari jalur HTTP yang dilompati.

Fake seperti ini punya perilaku: ia menerima request, menjawab, dan mengingat apa yang ia terima. Test bertanya kepadanya setelah kejadian. Itu berbeda dari mock, yang diberi tahu di awal call apa saja yang harus datang. Fake membuat test memeriksa hasil, bukan cara mencapainya.

### Menunggu hasil asinkron

`Publish` langsung kembali. Handler berjalan di goroutine lain, beberapa saat kemudian. Assertion yang ditulis tepat setelah `Publish` hampir pasti berjalan terlalu cepat.

Jalan pintasnya adalah `time.Sleep(100 * time.Millisecond)` sebelum assertion. Jalan pintas itu salah dengan dua cara sekaligus. Kalau 100 ms terlalu pendek untuk mesin CI yang sedang sibuk, test gagal tanpa ada bug. Kalau angkanya dinaikkan supaya aman, setiap test membayar waktu itu penuh, setiap kali, juga ketika hasilnya sudah siap dalam 2 ms. Dua ratus test dengan sleep satu detik berarti lebih dari tiga menit menunggu tanpa guna.

Helper `eventually` memeriksa kondisi berulang-ulang dengan jeda pendek. Ia kembali begitu kondisinya terpenuhi, dan timeout hanya dibayar ketika test memang gagal.

Cara paling mudah memahaminya: menunggu paket. `time.Sleep` sama dengan memutuskan untuk melihat teras tepat pukul 15.00, satu kali. `eventually` sama dengan melihat teras setiap beberapa menit sampai pukul 17.00, dan berhenti begitu paketnya ada.

Analogi ini berhenti bekerja untuk membuktikan bahwa sesuatu **tidak** terjadi. "Tidak ada email kedua" tidak bisa ditunggu sampai terpenuhi; ia hanya bisa diamati selama satu jendela waktu, dan jendela itu selalu dibayar penuh. Assertion negatif di test asinkron memang lebih lambat dan lebih lemah. Pakai seperlunya.

### Menunggu service siap juga bukan pekerjaan sleep

Karena `gochannel` membuang message tanpa subscriber, test harus menunggu sampai service siap sebelum menerbitkan apa pun. `startService` di `component_test.go` menunggu channel `service.Running()` tertutup. Itu menunggu sinyal, dengan timeout sebagai pengaman, bukan menebak durasi.

## The Exercise

Kode ada di `labs/07-component-testing/`.

| File | Isi | Diubah? |
|---|---|---|
| `events.go` | Event, `AvailabilityUpdate`, interface `Emailer` | Tidak |
| `service.go` | Service dengan tiga handler. Punya dua bug | Ya, di Step 3 dan 4 |
| `unit_test.go` | Unit test handler | Tidak |
| `testkit_test.go` | Helper `eventually` dan `fakeChannelManager` | Ya, di Step 1 dan 2 |
| `component_test.go` | Component test Step 1–4 dan `startService` | Tidak |
| `cancel_test.go` | Tempat component test yang kamu tulis sendiri | Ya, di Step 5 |

Di lab ini kamu mengubah file test. Itu disengaja: perkakas test adalah bagian dari yang dipelajari.

### Step 0: Unit Tests Pass on a Broken Service

```bash
cd ~/Developer/playground/event-driven/labs/07-component-testing
go test -run Step0 -v
```

`PASS`. Handler email memanggil `Emailer` dengan benar. Handler sync mengirim JSON yang benar. Service ini tetap tidak bisa mengirim email dan bisa kehilangan update. Ingat hasil ini saat mengerjakan Step 3 dan 4.

### Step 1: Replace Sleep with Polling

```bash
go test -run Step1 -v
```

Buka `eventually` di `testkit_test.go` sebelum menjalankan. Ia tidur 50 ms lalu memeriksa sekali. Tebak dulu: apa hasilnya untuk kondisi yang baru terpenuhi setelah 300 ms?

```
eventually mengembalikan false padahal kondisi terpenuhi jauh sebelum timeout: ia berhenti memeriksa terlalu cepat
eventually menyerah setelah 51.011333ms, seharusnya menunggu sampai timeout 100ms
```

**Task:** tulis ulang `eventually` supaya memeriksa `cond` berulang-ulang sampai `true` atau sampai `timeout` habis.

> [!hint]- Hint
> Hitung `deadline := time.Now().Add(timeout)`. Di dalam `for`: periksa `cond()` dulu, lalu periksa apakah deadline sudah lewat, baru tidur 5 ms. Memeriksa kondisi sebelum tidur membuat kasus "sudah siap" selesai tanpa jeda.

### Step 2: Make the Fake Channel Manager Listen

```bash
go test -run Step2 -v
```

```
channel manager tiruan merekam 0 update dalam 2s, seharusnya 1
```

Kali ini service-nya benar. Yang tuli adalah fake-nya: ia menjawab 200 untuk semua request dan tidak mencatat apa pun.

**Task:** lengkapi `fakeChannelManager`:

- `handle` membaca body sebagai `AvailabilityUpdate` dan merekamnya.
- `failFirst` request pertama dijawab `503` dan **tidak** masuk daftar yang diterima.
- `accepted()` mengembalikan update yang dijawab 200. `requests()` mengembalikan jumlah semua request.

> [!hint]- Hint
> `handle` berjalan di goroutine milik HTTP server, sedangkan test membaca dari goroutine lain. Semua field yang dibagi harus dijaga `sync.Mutex`. Jalankan `go test -race -run Step2` untuk memastikannya. Kembalikan salinan slice dari `accepted()`, bukan slice aslinya.

### Step 3: Find the Wiring Bug

```bash
go test -run Step3 -v
```

```
email konfirmasi tidak terkirim dalam 2s, padahal unit test handler email lulus
```

Step 0 membuktikan handler email benar. Jadi pertanyaannya bukan "apa yang salah di handler", melainkan "kenapa handler tidak dipanggil".

**Task:** temukan dan perbaiki bug di `service.go`.

> [!hint]- Hint
> Lihat fungsi `New`. Bandingkan argumen topic di ketiga baris `router.AddConsumerHandler`. Dua memakai konstanta; satu memakai string yang diketik tangan.

### Step 4: Do Not Trust a Response You Did Not Read

```bash
go test -run Step4 -v
```

Fake channel manager menjawab `503` dua kali, lalu pulih.

```
update tidak pernah diterima channel manager setelah ia pulih (total request: 1): jawaban 503 dianggap berhasil dan update hilang
```

Service mengirim satu request, menerima 503, dan menganggapnya selesai. Message mendapat ack. Availability di channel manager sekarang salah, dan tidak ada yang akan memperbaikinya.

**Task:** perbaiki `push` di `service.go`.

> [!hint]- Hint
> `client.Do` hanya mengembalikan error untuk kegagalan jaringan. Jawaban 503 adalah response yang sah di mata `http.Client`. Periksa `resp.StatusCode` dan kembalikan error kalau bukan 2xx. Error itu membuat router melakukan nack, dan message diantar ulang.

Unit test di Step 0 tidak menangkap bug ini karena ia hanya menguji jawaban 200. Component test menangkapnya karena fake bisa berperilaku seperti partner yang sedang bermasalah.

### Step 5: Write Your Own Component Test

```bash
go test -run Step5 -v
```

```
test ini belum ditulis
```

Service punya handler ketiga: `BookingCancelled` mengembalikan kamar ke channel manager dengan `delta` positif. Belum ada yang mengujinya dari luar.

**Task:** tulis `TestStep5_CancelledBookingRestoresAvailability` di `cancel_test.go`. Jalankan service dengan `startService`, terbitkan `BookingCancelled` untuk 2 kamar dengan `publish`, tunggu dengan `eventually`, lalu periksa isi update yang diterima fake.

> [!hint]- Hint
> Bentuknya sama dengan Step 2. Yang berbeda hanya topic (`TopicBookingCancelled`), tipe event, dan tanda `Delta`. Setelah test lulus, ubah sementara `Delta: event.Rooms` di `service.go` menjadi negatif dan pastikan test-mu gagal. Test yang tidak pernah terlihat gagal belum terbukti menguji apa pun.

Setelah itu jalankan semuanya:

```bash
go test -race -v
```

## Solution

> [!success]- Solution
> Tiga perubahan kecil, dan ketiganya di tempat yang tidak dijangkau unit test.
>
> ```go
> // testkit_test.go
> func eventually(timeout time.Duration, cond func() bool) bool {
> 	deadline := time.Now().Add(timeout)
> 	for {
> 		if cond() {
> 			return true
> 		}
> 		if time.Now().After(deadline) {
> 			return false
> 		}
> 		time.Sleep(5 * time.Millisecond)
> 	}
> }
>
> // service.go, wiring
> router.AddConsumerHandler("send-confirmation-email", TopicBookingCreated, cfg.PubSub, email.handleBookingCreated)
>
> // service.go, push
> if resp.StatusCode < 200 || resp.StatusCode > 299 {
> 	return fmt.Errorf("push availability for booking %s: channel manager answered %s", update.BookingID, resp.Status)
> }
> ```
>
> Fake channel manager dan test Step 5 ada di `labs/07-component-testing/_solusi/`.

## When This Is the Wrong Choice

Component test bukan pengganti unit test. Menguji dua puluh kombinasi aturan tarif lewat router dan HTTP server tiruan membuat suite lambat dan pesan gagalnya kabur. Logika bercabang banyak diuji di unit test; component test cukup membuktikan bahwa jalur utamanya tersambung dan kegagalan dependency ditangani.

Component test dengan `gochannel` tidak membuktikan apa pun tentang broker asli. `gochannel` tidak punya consumer group, tidak menjamin urutan, dan tidak menyimpan message. Perilaku seperti rebalancing, commit offset, dan urutan dalam satu partition hanya bisa diuji terhadap Kafka sungguhan, seperti di [[02 Lab - Message Broker]] dan [[13 Lab - Message Ordering]].

Fake hanya sebaik pengetahuanmu tentang partner. Fake di lab ini menjawab 503 karena kamu memutuskan begitu. Channel manager asli mungkin menjawab 200 dengan body berisi error, atau 429 dengan header `Retry-After`. Kalau fake dan partner berbeda, test hijau dan production merah. Fake harus diperbarui setiap kali kamu belajar hal baru tentang perilaku partner.

Terakhir, jangan menuntut urutan antar handler yang independen. Email dan sync berjalan di goroutine berbeda. Test yang menuntut email terkirim sebelum request HTTP akan gagal sesekali tanpa ada bug.

## What Is Still Broken

1. **Retry bisa membuat channel manager menerima update dua kali.** Bayangkan channel manager sudah menerapkan `delta: -2`, lalu jawabannya hilang di jaringan. Service melihat error dan mengirim lagi. Availability berkurang 4. Fake di lab ini tidak meniru kasus itu, dan service belum punya pertahanan. [[09 Lab - At-Least-Once Delivery]] membahasnya.
2. **Retry di sini tanpa jeda dan tanpa batas.** Kalau channel manager mati sejam, service menghantamnya terus. Middleware dari [[06 Lab - Errors]] belum dipasang di service ini.
3. **Semua test memakai broker di memory.** Wiring terbukti, perilaku Kafka belum.

## Reflection Questions

1. Unit test handler email lulus, tetapi email tidak pernah terkirim. Jelaskan dengan kalimatmu sendiri baris mana yang tidak pernah dieksekusi oleh unit test.
2. Sebuah test memakai `time.Sleep(500 * time.Millisecond)` lalu assertion, dan selalu lulus di laptopmu. Sebutkan dua cara test itu merugikan tim.
3. Kamu ingin memastikan service **tidak** mengirim email untuk booking yang dibatalkan. Kenapa `eventually` tidak bisa dipakai, dan apa yang kamu pakai sebagai gantinya?
4. Spesifikasi integrasi channel manager menyebut webhook sebagai jalur utama dan polling feed sebagai jaring pengaman. Kalau kamu menulis fake untuk channel manager itu, perilaku buruk apa saja yang harus bisa ia tiru supaya component test-mu berguna?

**My answers:**

1.
2.
3.
4.

## Read Next in the Vault

- [[Testing HTTP Handlers with httptest]]
- [[Mocking Through Interfaces]]
- [[Meaningful Test Coverage]]
- [[Integration Testing Across an Organisational Boundary]]
- [[Stdlib Testing vs Testify]]
