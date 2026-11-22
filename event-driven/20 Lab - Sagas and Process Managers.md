# 20 Lab - Sagas and Process Managers

[[19 Lab - Fault Tolerance]] · [[00 Start Here]] · [[21 Project - Hotel Channel Sync]]

**Goal:** menulis process manager yang menjalankan satu booking melewati tiga service tanpa transaction bersama, dan yang tahu cara mundur dengan tertib ketika salah satu step gagal atau tidak menjawab.
**Covers:** sagas and process managers: local transaction, compensating action, choreography vs orchestration, state yang disimpan, duplikat, timeout, jawaban yang terlambat.
**Prerequisites:** [[09 Lab - At-Least-Once Delivery]] (event bisa datang dua kali), [[11 Lab - Commands]] (command vs event), [[10 Lab - Outbox]] (kenapa "simpan lalu kirim" tidak atomic).
**Infrastructure:** none.
**Time:** 3–4 jam.

---

## Why This Exists

Satu booking dari OTA menyentuh tiga service:

1. **Inventory service** menahan kamar untuk tanggal yang diminta.
2. **Payment service** mengambil uang dari kartu tamu.
3. **Channel service** mengonfirmasi booking ke channel manager.

Selama ketiganya hidup dalam satu database, masalahnya tidak ada. Satu `BEGIN`, tiga perubahan, satu `COMMIT`. Kalau pembayaran gagal, `ROLLBACK` mengembalikan kamar seolah tidak pernah ditahan.

Begitu tiap service punya database sendiri, `ROLLBACK` itu hilang. Inventory sudah melakukan commit di databasenya sebelum payment service bahkan menerima permintaan. Kalau kartu ditolak, tidak ada mekanisme database yang melepas kamar. Kamar itu tertahan untuk booking yang tidak akan pernah terjadi, dan hotel kehilangan satu malam yang bisa dijual.

### Why not two-phase commit

Two-phase commit (2PC) adalah jawaban klasik untuk transaction lintas database. Sebuah coordinator bertanya kepada semua peserta "siap commit?", dan baru menyuruh commit setelah semua menjawab siap.

Di sini 2PC tidak bisa dipakai karena tiga alasan. Peserta harus mendukung protokolnya, sedangkan payment gateway dan channel manager adalah HTTP API milik pihak lain. Selama menunggu keputusan, setiap peserta menahan lock; kalau coordinator mati di tengah jalan, lock itu tertahan sampai ia kembali. Dan ketersediaan seluruh operasi menjadi serendah peserta yang paling sering mati.

## Concepts

### A saga is a sequence of local transactions

Saga melepas tuntutan "semua atau tidak sama sekali dalam satu saat". Sebagai gantinya, setiap step adalah local transaction yang langsung commit di service-nya sendiri. Kalau step berikutnya gagal, step yang sudah berhasil dibatalkan dengan **compensating action**: operasi baru yang membalik akibatnya.

| Step | Action | Compensating action |
|---|---|---|
| 1 | `ReserveRoom` | `ReleaseRoom` |
| 2 | `CapturePayment` | `RefundPayment` |
| 3 | `ConfirmWithChannel` | (step terakhir; kalau gagal, batalkan 1 dan 2) |

Padanan terdekatnya di luar software adalah memesan perjalanan lewat tiga loket: tiket pesawat, hotel, lalu mobil sewaan. Kalau mobil ternyata tidak tersedia, kamu kembali ke loket hotel dan loket pesawat untuk membatalkan. Tidak ada tombol yang membatalkan ketiganya sekaligus.

Yang tidak ditangkap analogi ini adalah siapa yang mengingat. Kamu ingat loket mana yang sudah didatangi. Program tidak ingat apa pun setelah restart, kecuali yang ia tulis ke database. State saga harus disimpan.

### Compensation is not rollback

Rollback membuat perubahan tidak pernah terlihat oleh siapa pun. Compensation tidak punya sifat itu, dan perbedaannya terasa di tiga tempat.

**Keadaan antara terlihat dari luar.** Di antara `RoomReserved` dan `RoomReleased`, kamar itu sungguh tidak tersedia. Tamu lain yang mencari pada detik itu melihat hotel penuh. Tidak ada isolation.

**Sebagian akibat tidak bisa dibalik.** Refund bukan kebalikan dari capture. Tamu melihat dua baris di mutasi rekeningnya, uangnya kembali beberapa hari kemudian, dan biaya transaksi mungkin tidak kembali. Email yang terkirim tidak bisa ditarik; compensation-nya adalah email kedua yang meminta maaf.

**Compensation sendiri bisa gagal.** `RefundPayment` adalah panggilan ke service yang bisa mati. Karena itu compensating action harus idempotent dan dicoba ulang sampai berhasil. Saga yang tersangkut di tengah compensation butuh manusia.

Konsekuensi untuk desain: status booking yang dilihat tamu harus jujur tentang keadaan antara. "Sedang diproses" lebih benar daripada "Berhasil" yang lalu berubah menjadi "Dibatalkan".

### Choreography vs orchestration

Ada dua cara menyusun saga.

**Choreography:** tidak ada pusat. Setiap service bereaksi terhadap event service lain.

```
inventory ──RoomReserved──► payment ──PaymentCaptured──► channel ──ChannelConfirmed──► ...
    ▲                          │
    └──────PaymentFailed───────┘   (inventory mendengar PaymentFailed lalu melepas kamar)
```

Service tidak saling mengenal, dan tidak ada komponen tambahan. Harganya: alur bisnis tidak tertulis di satu tempat pun. Untuk menjawab "apa yang terjadi kalau channel menolak?", kamu harus membaca kode tiga service dan menyusun sendiri gambarnya. Setiap step baru menambah event yang harus didengar service lama.

**Orchestration:** satu komponen, **process manager**, memegang alur. Ia menerima event dan mengirim command. Service lain tidak tahu bahwa mereka bagian dari saga.

```
                      ┌───────────────────┐
   BookingRequested ─►│  process manager  │─── ReserveRoom ──────────► inventory
       RoomReserved ─►│                   │─── CapturePayment ───────► payment
    PaymentCaptured ─►│  state per booking│─── ConfirmWithChannel ───► channel
   ChannelConfirmed ─►│  disimpan di DB   │
      PaymentFailed ─►│                   │─── ReleaseRoom, RefundPayment (compensation)
                      └───────────────────┘
```

Alurnya terbaca di satu file, state setiap booking bisa ditanyakan, dan timeout punya tempat yang jelas. Harganya: satu komponen lagi yang harus dijalankan, dan godaan untuk menaruh aturan bisnis milik service lain di dalamnya.

Pedoman kasarnya: dua atau tiga step tanpa cabang cukup dengan choreography. Begitu ada compensation bertingkat, timeout, atau pertanyaan "booking ini sedang di step mana?", orchestration membayar biayanya. Lab ini membangun versi orchestration.

### What a process manager is

Process manager adalah state machine yang disimpan. Untuk setiap booking ia menyimpan satu baris: posisi saat ini, apa yang sudah terjadi, dan kapan step terakhir dimulai. Ia tidak mengerjakan pekerjaan bisnis apa pun. Seluruh isinya adalah tabel keputusan: pada posisi X, kalau datang event Y, pindah ke posisi Z dan kirim command W.

```
                 RoomReserved          PaymentCaptured         ChannelConfirmed
reserving_room ──────────────► capturing_payment ──────────► confirming_channel ──────────► completed
      │                              │                              │
      │ RoomReservationFailed        │ PaymentFailed / timeout      │ ChannelConfirmationFailed / timeout
      ▼                              ▼                              ▼
    failed  ◄──────────────────  compensating  ◄────────────────────┘
              semua compensation     (menunggu RoomReleased, PaymentRefunded)
              sudah dikonfirmasi
```

Tiga hal membuat state machine ini lebih sulit daripada gambarnya.

**Event datang dua kali.** Broker at-least-once. `RoomReserved` kedua tidak boleh menghasilkan `CapturePayment` kedua. Karena posisi saga disimpan, pemeriksaannya sederhana: event yang tidak cocok dengan posisi sekarang diabaikan.

**Menyimpan state dan mengirim command bukan satu operasi atomic.** Ini masalah dual write dari [[10 Lab - Outbox]]. Tanpa outbox, urutannya menentukan jenis kerusakan. Simpan dulu lalu kirim: kalau pengiriman gagal, state sudah maju, event yang dikirim ulang dianggap duplikat, dan command hilang selamanya. Kirim dulu lalu simpan: kalau penyimpanan gagal, event yang dikirim ulang mengirim command lagi. Command ganda aman selama penerimanya idempotent; command hilang tidak pernah aman.

**Diam bukan jawaban.** Payment service yang tidak menjawab tidak mengirim `PaymentFailed`. Tanpa timeout, saga menunggu selamanya dan kamar tertahan selamanya. Dan setelah saga menyerah, jawaban yang terlambat masih bisa datang: `PaymentCaptured` tiba dua puluh menit kemudian, untuk booking yang sudah dibatalkan. Uang itu sudah diambil dan harus dikembalikan.

## The Exercise

Kode ada di `labs/20-sagas-and-process-managers/`.

| File | Isi | Diubah? |
|---|---|---|
| `types.go` | `Event`, `Command`, `Saga`, `Store`, `CommandBus`, `MemoryStore` | Tidak |
| `manager.go` | `Manager.Handle` dan `Manager.Tick` | Ya |
| `manager_test.go` | Lima skenario | Tidak |

Baca `types.go` dulu, terutama struct `Saga`. Empat field boolean (`RoomReserved`, `RoomReleased`, `PaymentCaptured`, `PaymentRefunded`) mencatat apa yang sudah terjadi di service lain. Dari keempatnya kamu bisa menghitung compensation mana yang masih tersisa.

### Step 0: The saga starts

```bash
cd labs/20-sagas-and-process-managers
go test -run Step0 -v
```

`PASS`. `BookingRequested` membuat state dengan status `reserving_room` dan mengirim `ReserveRoom`. Perhatikan urutan di kode awal: command dikirim dulu, state disimpan kemudian.

### Step 1: The happy path

```bash
go test -run Step1 -v
```

**Task:** buat `Handle` mengenali `RoomReserved`, `PaymentCaptured`, dan `ChannelConfirmed`. Setiap event memajukan status dan mengirim command untuk step berikutnya. `CapturePayment` membawa `AmountIDR` yang diambil dari state saga, karena event `RoomReserved` tidak memuatnya.

> [!hint]- Hint
> Muat saga dengan `m.store.Load`. Pisahkan keputusan dari efek samping: tulis satu function `transition(saga *Saga, evt Event) []Command` yang hanya mengubah struct dan mengembalikan command, lalu satu function `commit` yang mengirim command dan menyimpan saga. Di `transition`, `switch` pada `evt.Name`, dan di setiap cabang periksa `saga.Status` dulu. Jangan lupa memperbarui `StepStartedAt` setiap kali pindah step; Step 4 membutuhkannya.

### Step 2: Compensation

```bash
go test -run Step2 -v
```

Tiga kegagalan, tiga kedalaman:

| Gagal di | Yang sudah terjadi | Compensation |
|---|---|---|
| `RoomReservationFailed` | belum ada | tidak ada, langsung `failed` |
| `PaymentFailed` | kamar ditahan | `ReleaseRoom` |
| `ChannelConfirmationFailed` | kamar ditahan, uang diambil | `RefundPayment` dan `ReleaseRoom` |

Status menjadi `compensating` sampai setiap compensation dikonfirmasi oleh event (`RoomReleased`, `PaymentRefunded`), baru kemudian `failed`. Simpan alasan kegagalan di `FailureReason`.

Sebelum menulis kode, jawab dulu: pada sub-test kedua, di antara `PaymentFailed` dan `RoomReleased`, apa yang dilihat tamu lain yang mencari kamar deluxe?

> [!hint]- Hint
> Tulis satu function `compensate(saga *Saga, reason string) []Command`. Ia mengisi status dan alasan, lalu melihat boolean: kalau `PaymentCaptured && !PaymentRefunded` tambahkan `RefundPayment`; kalau `RoomReserved && !RoomReleased` tambahkan `ReleaseRoom`. Tulis satu function lagi, `settle`, yang mengubah `compensating` menjadi `failed` kalau tidak ada lagi yang tersisa. Panggil `settle` di akhir `compensate` (untuk kasus tanpa compensation) dan setiap kali `RoomReleased` atau `PaymentRefunded` datang.

### Step 3: Duplicates and redelivery

```bash
go test -run Step3 -v
```

Tiga sub-test:

1. Setiap event dikirim dua kali. Jumlah command tidak boleh bertambah.
2. Event untuk booking yang tidak punya saga diabaikan tanpa error.
3. Command bus gagal sekali saat `RoomReserved` diproses. `Handle` harus mengembalikan error, dan ketika broker mengirim ulang `RoomReserved`, `CapturePayment` harus terkirim.

Sub-test ketiga menguji urutan kirim dan simpan. Kalau kamu menyimpan state dulu, pengiriman ulang melihat status `capturing_payment`, menganggap event itu duplikat, dan command hilang.

> [!hint]- Hint
> Kalau setiap cabang di `transition` sudah memeriksa `saga.Status`, duplikat tertangani sendiri: event kedua tidak cocok dengan posisi baru dan tidak mengubah apa pun. Untuk mendeteksi "tidak ada yang berubah", salin saga sebelum `transition` dan bandingkan sesudahnya dengan `==`; struct `Saga` hanya berisi field yang bisa dibandingkan. `BookingRequested` duplikat ditangani dengan memeriksa `found` dari `Load`. Di `commit`: kirim semua command, dan hanya kalau semuanya berhasil, simpan.

### Step 4: Timeout and the late reply

```bash
go test -run Step4 -v
```

Booking `B-1` menunggu jawaban payment service. Lima belas menit berlalu tanpa event.

**Task:** isi `Tick`. Untuk setiap saga aktif yang menunggu lebih lama dari `stepTimeout`, jalankan compensation dengan alasan yang memuat kata `timeout`. `Tick` kedua tidak boleh mengirim command lagi.

Lalu tangani jawaban yang terlambat: `PaymentCaptured` yang datang setelah saga menyerah harus menghasilkan `RefundPayment`, tepat satu kali, dan tidak boleh melanjutkan saga ke `ConfirmWithChannel`.

> [!hint]- Hint
> `Tick`: ambil `m.store.Active(ctx)`, lewati saga yang statusnya bukan salah satu dari tiga status menunggu, lewati yang belum melewati `stepTimeout`, lalu `compensate` dan `commit`. Saga yang sudah `compensating` otomatis terlewati di Tick berikutnya. Jawaban terlambat: di cabang `PaymentCaptured`, tambahkan kasus kedua, yaitu status `compensating` atau `failed` dan `PaymentCaptured` masih `false`. Isi `PaymentCaptured = true`, kembalikan status ke `compensating`, dan kirim `RefundPayment`. Boolean itulah yang mencegah refund ganda saat duplikatnya datang.

## Solution

> [!success]- Solution
> Tiga potongan memuat hampir semua idenya. Pertama, `Handle` memisahkan keputusan dari efek samping dan membuang event yang tidak mengubah apa pun:
>
> ```go
> before := saga
> commands := m.transition(&saga, evt)
> if saga == before {
> 	return nil // duplikat atau event yang tidak cocok dengan posisi saga
> }
> return m.commit(ctx, saga, commands)
> ```
>
> Kedua, compensation dihitung dari apa yang sudah terjadi, bukan dari step mana yang gagal:
>
> ```go
> func compensate(saga *Saga, reason string) []Command {
> 	saga.Status = StatusCompensating
> 	saga.FailureReason = reason
>
> 	var commands []Command
> 	if saga.PaymentCaptured && !saga.PaymentRefunded {
> 		commands = append(commands, refundPayment(*saga))
> 	}
> 	if saga.RoomReserved && !saga.RoomReleased {
> 		commands = append(commands, releaseRoom(*saga))
> 	}
> 	settle(saga)
> 	return commands
> }
> ```
>
> Ketiga, jawaban yang terlambat diperlakukan sebagai fakta baru yang butuh compensation sendiri:
>
> ```go
> case stopped && !saga.PaymentCaptured:
> 	saga.PaymentCaptured = true
> 	saga.Status = StatusCompensating
> 	return []Command{refundPayment(*saga)}
> ```
>
> Kode lengkap ada di `_solusi/manager.go`.

## When This Is the Wrong Choice

Saga adalah harga yang dibayar karena data dipecah ke beberapa service. Kalau ketiga tabel bisa berada dalam satu database, satu transaction lebih sederhana, lebih cepat, dan lebih benar. Jangan memecah service lalu membangun saga untuk menambal akibatnya.

Saga juga salah kalau keadaan antara tidak boleh terlihat. Memindahkan saldo antara dua rekening di bank yang sama butuh isolation yang tidak bisa diberikan saga.

Hati-hati dengan step yang tidak punya compensation. Kalau step ketiga mengirim barang fisik atau menerbitkan dokumen resmi, urutkan saga supaya step yang tidak bisa dibalik berada paling akhir, setelah semua yang bisa gagal sudah berhasil.

Dan untuk alur dua step tanpa cabang, process manager berlebihan. Satu consumer yang mendengar satu event dan mengirim satu command sudah cukup.

## What Is Still Broken

1. **Kirim lalu simpan masih bisa menggandakan command.** Urutan yang dipilih di lab ini menukar "command hilang" dengan "command ganda". Yang benar adalah menyimpan state dan command dalam satu transaction lewat outbox ([[10 Lab - Outbox]]), dan membuat setiap penerima command idempotent dengan `BookingID` sebagai key.
2. **Compensation yang tidak dikonfirmasi tersangkut.** Kalau `RoomReleased` tidak pernah datang, saga berada di `compensating` selamanya. `Tick` tidak memeriksa status itu. Sistem nyata mengirim ulang compensating command secara berkala dan memanggil manusia setelah batas tertentu.
3. **Timeout di step channel tidak aman.** Kalau saga menyerah saat `confirming_channel`, lalu `ChannelConfirmed` datang terlambat, booking itu sudah tercatat di OTA padahal uangnya dikembalikan. Butuh compensating command tambahan (`CancelChannelBooking`) yang tidak ada di lab ini.
4. **Dua event untuk booking yang sama bisa diproses bersamaan.** `Load` lalu `Save` tanpa lock berarti dua goroutine bisa saling menimpa. Perbaikannya optimistic locking: kolom version di baris saga, dan `Save` gagal kalau version sudah berubah.
5. **Tidak ada yang melihat saga yang gagal.** Jumlah saga per status dan umur saga tertua di `compensating` adalah metric yang layak diberi alert ([[17 Lab - Metrics and Alerting]]).

## Reflection Questions

1. Kenapa `ROLLBACK` tidak bisa melepas kamar ketika pembayaran gagal di service lain? Apa yang sudah terjadi pada saat itu?
2. Sebutkan tiga perbedaan antara compensation dan rollback yang bisa dilihat oleh tamu hotel atau front office.
3. Di Step 3, apa yang hilang kalau `commit` menyimpan state sebelum mengirim command? Kenapa command ganda lebih bisa diterima daripada command hilang?
4. Billing di kantormu: sebuah baris outbox diproses cron, sistem billing sudah membuat tagihan, lalu proses mati sebelum status baris diperbarui. Gambarkan ini sebagai saga dua step. Apa compensating action-nya, dan apakah compensation memang jalan keluar yang tepat, atau cukup membuat step-nya idempotent?
5. Integrasi channel manager: revisi booking masuk lewat webhook, diterapkan ke database, dan availability baru dikirim balik ke channel manager. Kamu pilih choreography atau orchestration? Sebutkan satu kejadian yang akan membuatmu berubah pikiran.

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Sagas - Orchestration vs Choreography]]
- [[Compensating Transactions]]
- [[Two-Phase Commit and Why It Is Avoided]]
- [[Dual Writes and Their Dangers]]
- [[Defensible Eventual Consistency]]
- [[Idempotent Consumers]]
