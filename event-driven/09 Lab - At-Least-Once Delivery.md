# 09 Lab - At-Least-Once Delivery

[[08 Lab - Events with CQRS]] · [[00 Start Here]] · [[10 Lab - Outbox]]

- **Goal:** melihat sendiri kenapa satu message bisa diproses dua kali, lalu menulis handler dan repository yang tetap benar ketika itu terjadi.
- **Covers:** at-least-once delivery, redelivery, idempotent handler, pilihan idempotency key, idempotent repository.
- **Prerequisites:** [[02 Lab - Message Broker]] (ack dan offset), [[06 Lab - Errors]] (handler yang return error akan dicoba ulang).
- **Infrastructure:** none.
- **Time:** 1,5–2 jam.

## Why This Exists

Hotel punya 10 kamar deluxe. Satu tamu memesan 2 kamar lewat sebuah OTA. Beberapa detik kemudian tape chart menunjukkan sisa 6, bukan 8. Tidak ada yang salah ketik dan tidak ada bug di broker. Message `BookingCreated` yang sama diproses dua kali, dan handler mengurangi kamar dua kali.

Dua kamar yang hilang itu kamar kosong yang tidak bisa dijual. Kebalikannya lebih mahal lagi: kalau yang terproses dua kali adalah pembatalan, hotel menjual kamar yang tidak ada.

Duplikat seperti ini bukan kejadian langka yang bisa diabaikan. Ia akibat langsung dari jaminan yang dipilih hampir semua broker, dan satu-satunya tempat untuk menanganinya adalah consumer.

## Concepts

### Broker hanya tahu satu hal

Broker tidak bisa melihat apa yang terjadi di dalam consumer. Ia hanya tahu apakah ack untuk sebuah message sudah diterima. Selama belum, ia menganggap message itu belum diproses dan mengirimnya lagi. Pengiriman ulang ini disebut redelivery.

```
broker ──► m-1 ──► consumer: kurangi kamar ✔  kirim email ✔
broker ◄── ack ──  (ack hilang di jaringan)
broker: "belum ada ack untuk m-1"
broker ──► m-1 ──► consumer: kurangi kamar ✔ (lagi)  kirim email ✔ (lagi)
```

Ack bisa tidak sampai karena tiga hal, dan dari sisi broker ketiganya terlihat sama:

1. Consumer belum memproses message itu.
2. Consumer sedang memproses, lalu mati di tengah jalan.
3. Consumer sudah selesai memproses, tapi ack-nya hilang atau consumer mati tepat sebelum mengirim ack.

Di Kafka, ack berbentuk offset commit. Consumer yang mati setelah memproses 50 record tapi sebelum melakukan commit akan menerima 50 record itu lagi setelah restart.

### Dua jaminan yang bisa dipilih

Broker harus memutuskan apa yang dilakukan ketika ack tidak datang.

- **At-most-once:** jangan pernah kirim ulang. Tidak ada duplikat, tapi kasus 1 dan 2 berarti message hilang.
- **At-least-once:** kirim ulang sampai ada ack. Tidak ada yang hilang, tapi kasus 3 berarti message diproses dua kali.

"Exactly-once" tidak bisa dijamin broker sendirian, karena ia tidak bisa membedakan kasus 1 dari kasus 3. Yang bisa dicapai adalah efek yang sama dengan exactly-once: delivery tetap at-least-once, dan consumer dibuat **idempotent**. Sebuah operasi disebut idempotent kalau menjalankannya dua kali menghasilkan keadaan yang sama dengan menjalankannya sekali.

Padanan sehari-harinya adalah tombol lift. Menekan tombol lantai 5 tiga kali tidak membuat lift pergi ke lantai 15; tekanan kedua dan ketiga tidak mengubah apa pun. Tombol volume di remote berbeda: setiap tekanan menambah satu tingkat. Handler yang mengurangi sisa kamar adalah tombol volume, dan lab ini mengubahnya menjadi tombol lift. Analogi ini berhenti bekerja pada efek ke luar sistem: email yang sudah terkirim tidak bisa dibuat "tidak berubah" oleh tekanan kedua, dan bagian itu butuh perlakuan tersendiri.

### Duplikat datang dari dua arah

Redelivery oleh broker menghasilkan message yang sama persis, dengan message ID yang sama. Ada sumber duplikat kedua yang sering terlupa: pengirim. Producer yang melakukan publish lalu timeout tidak tahu apakah message-nya sampai. Ia mencoba lagi, dan broker sekarang menyimpan dua message dengan ID berbeda yang membawa booking yang sama.

Ini menentukan pilihan **idempotency key**, yaitu nilai yang dipakai consumer untuk mengenali "aku sudah pernah mengerjakan ini".

| Key | Menangkap redelivery broker | Menangkap kiriman ulang pengirim |
|---|---|---|
| Message ID | ya | tidak |
| Business key (`BookingID`, atau `BookingID` + nomor revisi) | ya | ya |

### Dua cara membuat consumer idempotent

**Cara pertama: ingat apa yang sudah diproses.** Consumer menyimpan daftar key yang sudah dikerjakan dan memeriksanya sebelum bekerja. Cara ini berlaku untuk operasi apa pun, termasuk yang sifatnya menambah atau mengurangi angka. Harganya: ada penyimpanan kedua yang terus tumbuh, dan tanda "sudah diproses" harus tersimpan bersamaan dengan efeknya. Kalau keduanya bisa terpisah, duplikat lolos lagi.

**Cara kedua: buat operasinya sendiri idempotent.** Simpan fakta, bukan selisih. "Booking B-1 memesan 2 kamar deluxe" bisa ditulis seratus kali ke tempat yang sama dan hasilnya tetap satu baris. Sisa kamar lalu dihitung dari booking yang tersimpan. Di SQL bentuknya `INSERT ... ON CONFLICT (booking_id) DO UPDATE`, yang biasa disebut upsert. Repository yang bekerja seperti ini disebut idempotent repository, dan ia tidak butuh daftar "sudah diproses".

Spesifikasi integrasi channel manager yang kamu pegang meminta keduanya dalam satu kalimat: revisi booking diterapkan idempotent, dan ack dikirim hanya setelah commit database. Urutan "commit dulu, baru ack" itu yang membuat sistem jatuh ke kasus 3 (duplikat) dan bukan ke kehilangan data.

## The Exercise

Kode ada di `labs/09-at-least-once-delivery/`.

| File | Isi | Diubah? |
|---|---|---|
| `broker.go` | Broker tiruan at-least-once, sekitar 60 baris | Tidak, baca saja |
| `inventory.go` | Handler yang mengurangi angka sisa kamar dan mengirim email | Ya, Step 1–3 |
| `ledger.go` | Repository yang menyimpan booking | Ya, Step 4 |
| `lab_test.go` | Lima skenario | Tidak |

### Step 0: The ideal world

```bash
cd labs/09-at-least-once-delivery
go test -run Step0 -v
```

Hasilnya `PASS`: 10 kamar, satu booking 2 kamar, sisa 8. Sebelum lanjut, baca `broker.go` dan temukan dua baris tempat broker memutuskan redelivery.

### Step 1: A lost ack

```bash
go test -run Step1 -v
```

Tebak dulu berapa sisa kamar yang dilaporkan test.

Test gagal dengan `sisa kamar = 6, seharusnya 8: satu booking terhitung dua kali`. Broker dijalankan dengan `LoseFirstAck: true`, jadi handler dipanggil dua kali untuk message yang sama.

**Tugas:** ubah `inventory.go` supaya message yang sama tidak mengurangi kamar dua kali.

> [!hint]- Hint
> Tambahkan `map[string]bool` di struct `Inventory` sebagai ingatan, dan periksa sebelum mengurangi kamar. Jangan lupa membuat map-nya di `NewInventory`.

### Step 2: Failure halfway through

```bash
go test -run Step2 -v
```

Kamar sudah dikurangi, lalu SMTP timeout. Handler return error dan broker melakukan redelivery. Test menuntut dua hal: kamar berkurang tepat sekali, dan email akhirnya terkirim.

Solusi Step 1 biasanya gagal di sini dengan salah satu dari dua pesan:

- `redelivery mengurangi kamar lagi`: tanda dipasang di akhir fungsi, setelah email. Saat email gagal, tanda belum terpasang.
- `email terkirim 0 kali`: handler langsung `return nil` untuk message yang sudah dikenal, sehingga percobaan kedua tidak pernah mengirim email.

**Tugas:** penuhi kedua syarat.

> [!hint]- Hint
> Handler ini punya dua efek dengan sifat berbeda. Pengurangan kamar harus terjadi tepat sekali, jadi tandanya dipasang bersamaan dengan pengurangan itu. Email boleh diulang sampai berhasil, jadi ia berada di luar pemeriksaan.

### Step 3: The sender resends

```bash
go test -run Step3 -v
```

Kali ini bukan broker yang mengulang. Pengirim melakukan publish booking `B-1` dua kali, sebagai `m-1` dan `m-2`. Kalau ingatanmu memakai message ID, kedua message terlihat baru dan test gagal dengan `message ID beda, tapi booking-nya sama`.

**Tugas:** ganti idempotency key.

> [!hint]- Hint
> Yang tidak boleh terjadi dua kali adalah "booking B-1 mengurangi kamar", bukan "message m-1 diproses". Pakai `m.Event.BookingID`.

### Step 4: An idempotent repository

```bash
go test -run Step4 -v
```

`Ledger` di `ledger.go` bekerja dengan cara lain. Ia tidak menyimpan angka sisa kamar. Ia menyimpan booking, dan `Available` menghitung sisa dari `capacity` dikurangi semua booking tersimpan.

Test mengirim tiga message untuk dua booking, dengan ack yang hilang. Kode awal gagal dengan `booking tersimpan = 6, seharusnya 2`, karena `Handle` melakukan `append` setiap kali dipanggil.

**Tugas:** buat `Ledger` aman terhadap duplikat **tanpa** menambahkan daftar "sudah diproses". Bagian kedua test mengirim revisi `B-1` dari 2 kamar menjadi 3; `Count` harus tetap 2 dan sisa kamar turun menjadi 6.

> [!hint]- Hint
> Ganti slice dengan `map[string]BookingCreated` yang kuncinya `BookingID`. Menulis ke kunci yang sama menimpa nilai lama, dan itu perilaku upsert.

Setelah lulus, coba bandingkan: apa yang terjadi kalau revisi 3 kamar itu dikirim ke `Inventory` dari Step 3?

### Finish

```bash
go test -v
```

Kelimanya harus `PASS`.

## Solution

> [!success]- Solution
> Ide kuncinya ada tiga. Tanda "sudah diproses" dipasang bersamaan dengan efek yang dijaganya. Key-nya adalah business key. Efek yang boleh diulang dibiarkan di luar penjagaan.
>
> ```go
> func (inv *Inventory) Handle(m Message) error {
> 	if !inv.processed[m.Event.BookingID] {
> 		inv.available[m.Event.RoomType] -= m.Event.Rooms
> 		inv.processed[m.Event.BookingID] = true
> 	}
> 	return inv.email.Send(m.Event.BookingID)
> }
> ```
>
> Untuk `Ledger`, tidak ada pemeriksaan apa pun. Bentuk penyimpanannya yang membuat duplikat tidak berbahaya:
>
> ```go
> func (l *Ledger) Handle(m Message) error {
> 	l.bookings[m.Event.BookingID] = m.Event
> 	return nil
> }
> ```
>
> Jawaban untuk pertanyaan di akhir Step 4: `Inventory` mengabaikan revisi 3 kamar, karena `B-1` sudah tercatat di `processed`. Daftar "sudah diproses" dengan key `BookingID` tidak bisa membedakan duplikat dari revisi. Key-nya harus diperluas menjadi `BookingID` + nomor revisi, dan handler harus menghitung selisih. `Ledger` menangani revisi tanpa perubahan apa pun.
>
> Kode lengkap: `labs/09-at-least-once-delivery/_solusi/`.

## When This Is the Wrong Choice

Idempotency punya harga, dan ada tempat di mana harga itu tidak perlu dibayar.

Data yang boleh hilang tidak butuh at-least-once. Metrik per detik atau posisi kursor di dokumen kolaboratif cukup dikirim at-most-once: nilai berikutnya segera menggantikan yang hilang, dan consumer-nya jauh lebih sederhana.

Daftar "sudah diproses" adalah pilihan yang salah ketika operasinya bisa dibuat idempotent secara alami. Daftar itu tumbuh selamanya kecuali dibersihkan, dan pembersihan butuh keputusan: berapa lama sebuah duplikat masih mungkin datang? Kalau jawabannya "tujuh hari" dan sebuah message tertahan delapan hari, duplikatnya lolos.

Upsert adalah pilihan yang salah ketika urutan kedatangan tidak terjamin dan tidak ada nomor versi. Revisi lama yang datang terlambat akan menimpa revisi baru. Itu topik [[13 Lab - Message Ordering]].

## What Is Still Broken

1. **Ingatan hilang saat restart.** `processed` dan `bookings` adalah map di memori. Di sistem nyata keduanya berada di database, dan tanda "sudah diproses" ditulis dalam transaction yang sama dengan perubahannya. [[10 Lab - Outbox]] memakai Postgres dan transaction; [[12 Lab - Read Models]] memakai tabel `processed_events` dengan cara itu.
2. **Email masih bisa terkirim dua kali.** Di Step 1 email terkirim dua kali dan test tidak mengeluh. SMTP tidak bisa ikut transaction database, jadi tepat sekali tidak bisa dijamin untuk efek ke luar. Yang bisa dilakukan: memperkecil jendelanya, atau meneruskan idempotency key ke pihak penerima kalau ia mendukungnya.
3. **Message yang gagal terus.** Broker tiruan menyerah setelah 5 percobaan dan menghentikan semuanya. Broker nyata harus memindahkan message itu supaya antrean tidak tersumbat. Itu [[19 Lab - Fault Tolerance]].
4. **Tidak ada concurrency.** Dua consumer yang memproses booking yang sama pada saat bersamaan bisa sama-sama lolos pemeriksaan `processed`. Unique constraint di database yang menutup lubang ini, bukan `if` di kode.

## Reflection Questions

1. Kenapa broker tidak bisa menjamin exactly-once sendirian? Sebutkan dua keadaan consumer yang tidak bisa ia bedakan.
2. Di Step 2, apa yang terjadi kalau urutannya dibalik menjadi "kirim email, kurangi kamar, pasang tanda" dan proses mati setelah email?
3. `email-service` membaca email dari Kafka. Kalau service itu mati setelah SMTP menerima email tetapi sebelum offset commit, apa yang dilihat penerima email? Apakah daftar "sudah diproses" di dalam service bisa mencegahnya sepenuhnya?
4. Di integrasi channel manager, revisi booking yang sama bisa datang lewat webhook dan lewat polling feed. Idempotency key apa yang kamu pilih, dan kenapa bukan ID request webhook?
5. Dari dua cara di lab ini, mana yang kamu pakai untuk "tambah 50.000 ke saldo deposit tamu"? Kenapa cara yang satunya tidak bisa dipakai apa adanya?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Exactly-Once Delivery as an Illusion]]
- [[Delivery Semantics]]
- [[Idempotent Consumers]]
- [[Idempotency Keys]]
- [[Idempotency]]
