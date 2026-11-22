# 03 Lab - Events

[[02 Lab - Message Broker]] · [[00 Start Here]] · [[04 Lab - Watermill Basics]]

---

**Goal:** merancang isi pesan sebagai kontrak: payload yang disengaja, envelope dengan metadata, dan pembacaan yang tahan terhadap perubahan.
**Covers:** event vs message vs command, desain event, envelope dan metadata, JSON marshalling, tolerant reader.
**Prerequisites:** [[02 Lab - Message Broker]]. Struct tag JSON di Go.
**Infrastructure:** none.
**Time:** 2–3 jam.

---

## Why This Exists

Di lab 02, isi pesan hanya `{"booking_id":"B-1"}` yang ditulis asal. Itu cukup selama pengirim dan penerima ditulis oleh orang yang sama pada sore yang sama.

Begitu pesan dibaca service lain, isinya berubah status. Ia menjadi API. Bedanya dengan API HTTP: kamu tidak tahu siapa saja pembacanya, dan pesan lama tetap tersimpan di topic berbulan-bulan setelah kode pengirimnya berubah. Field yang kamu ganti namanya hari ini masih akan dibaca dengan nama lamanya oleh consumer yang sedang mengejar ketertinggalan.

Jalan pintas yang paling menggoda adalah melakukan `json.Marshal` pada struct internal. Hasilnya tiga masalah sekaligus. Nama kolom tabel menjadi kontrak publik. Data yang tidak boleh keluar, seperti nomor kartu, ikut tersimpan di broker dan di log setiap consumer. Dan setiap perubahan skema database diam-diam mengubah pesan.

## Concepts

### Message, event, command

**Message** adalah amplopnya: sesuatu yang dikirim lewat broker. Kata itu tidak mengatakan apa pun tentang maksud isinya. Ada dua maksud yang umum.

**Event** menyatakan fakta yang sudah terjadi. Namanya berbentuk lampau: `BookingCreated`, `PaymentCaptured`. Pengirimnya tidak tahu dan tidak peduli siapa yang membaca. Event tidak bisa ditolak, karena kejadiannya sudah lewat.

**Command** meminta sesuatu dikerjakan. Namanya berbentuk perintah: `SendConfirmationEmail`, `CapturePayment`. Ia ditujukan ke satu penerima, dan penerima itu boleh gagal mengerjakannya.

| | Event | Command |
|---|---|---|
| Nama | lampau, `BookingCreated` | perintah, `SendConfirmationEmail` |
| Pembaca | nol sampai banyak, tidak dikenal pengirim | tepat satu |
| Pengirim tahu apa yang terjadi selanjutnya? | tidak | ya, itu tujuannya |

Perbedaannya menentukan arah ketergantungan. Kalau PMS mengirim command `SendConfirmationEmail`, PMS tahu bahwa ada service email dan apa tugasnya. Kalau PMS menerbitkan event `BookingCreated`, service email yang memutuskan bahwa booking baru berarti email baru. Menambah peminat kedua tidak menyentuh PMS. Command dibahas tersendiri di [[11 Lab - Commands]]; lab ini hanya memakai event.

### Apa yang masuk ke payload

Dua ekstrem sama-sama bermasalah.

Event yang terlalu kurus hanya membawa ID: `{"booking_id":"B-1"}`. Setiap consumer lalu harus memanggil PMS untuk menanyakan isinya. Ketergantungan synchronous yang susah payah dibuang di lab 01 kembali lewat pintu belakang, dan consumer membaca keadaan saat ia bertanya, bukan keadaan saat event terjadi.

Event yang terlalu gemuk menyalin seluruh baris tabel. Semua kolom menjadi kontrak, termasuk yang sensitif dan yang hanya bermakna di dalam PMS.

Pegangan yang bisa dipakai: payload memuat fakta yang dibutuhkan consumer untuk mengerjakan tugasnya tanpa bertanya balik, dan tidak lebih. Untuk `BookingCreated`, itu berarti tipe kamar, tanggal, jumlah kamar, dan alamat email tamu. Nomor kartu tidak dibutuhkan siapa pun di luar pembayaran.

Dua detail kecil yang sering menjadi sumber bug:

- **Nama field ditentukan oleh tag, bukan oleh nama field Go.** Tanpa tag, `encoding/json` memakai `BookingID`. Mengganti nama field saat refactor lalu mengubah kontrak tanpa ada yang sadar.
- **Tanggal menginap adalah tanggal kalender, bukan titik waktu.** Check-in 24 Desember tetap 24 Desember, di timezone mana pun pembacanya berada. Mengirimnya sebagai timestamp lengkap mengundang pergeseran hari saat dikonversi. Kirim sebagai string `2026-12-24`.

### Envelope

Beberapa keterangan dibutuhkan setiap consumer, apa pun jenis event-nya. Keterangan itu tidak termasuk fakta bisnis, jadi tempatnya bukan di payload. Ia ditaruh di **envelope** yang membungkus payload.

```json
{
  "id": "5f0c…",
  "name": "BookingCreated",
  "occurred_at": "2026-11-22T09:15:02Z",
  "correlation_id": "req-42",
  "payload": { "booking_id": "B-1", "room_type": "deluxe", "…": "…" }
}
```

| Field | Siapa yang memakainya |
|---|---|
| `id` | Consumer, untuk mengenali pesan yang sama datang dua kali |
| `name` | Consumer, untuk memilih cara membaca payload |
| `occurred_at` | Siapa pun yang perlu tahu kapan faktanya terjadi, bukan kapan pesannya dibaca |
| `correlation_id` | Orang yang sedang menelusuri satu request melintasi beberapa service |

Pikirkan envelope sebagai amplop surat: alamat, cap pos, dan nomor resi ada di luar, dan petugas bisa menyortir tanpa membuka isinya. Perbedaan pentingnya: di sini amplop dan isi dikirim dalam satu JSON. Banyak broker punya tempat khusus untuk metadata. Kafka menyebutnya record header dan Watermill menyebutnya `Metadata`. Lab 04 dan 05 memakainya.

`payload` bertipe `json.RawMessage` supaya envelope bisa dibaca dulu tanpa mengetahui bentuk isinya. Setelah `name` diketahui, barulah payload dibaca ke struct yang sesuai.

### Tolerant reader

Producer dan consumer tidak pernah di-deploy pada detik yang sama. Selalu ada masa ketika producer sudah mengirim bentuk baru sementara consumer masih versi lama. Consumer yang selamat dari masa itu mengikuti dua aturan:

- **Abaikan yang tidak dikenal.** Field baru di payload bukan error. `encoding/json` sudah berperilaku begitu, selama kamu tidak mengaktifkan `DisallowUnknownFields`. Nama event yang tidak dikenal juga bukan kerusakan; consumer cukup melewatinya.
- **Tuntut yang dibutuhkan.** Kalau `booking_id` kosong, `json.Unmarshal` tidak mengeluh. Ia mengisi zero value, dan kode di bawahnya mengurangi kamar untuk booking tanpa ID. Error yang jelas di pintu masuk lebih murah daripada data rusak yang ditemukan seminggu kemudian.

Dua jenis error itu sengaja dibedakan di lab ini. `ErrUnknownEvent` berarti "bukan urusanku". `ErrInvalidEvent` berarti "urusanku, tapi isinya tidak bisa dipakai". Consumer menanggapi keduanya dengan cara berbeda, dan itu topik [[06 Lab - Errors]].

## The Exercise

Kode ada di `labs/03-events/`.

| File | Isi | Diubah? |
|---|---|---|
| `booking.go` | Model internal `Booking`, nama event, dan `BookingCancelled` sebagai contoh yang sudah benar | Tidak |
| `event.go` | `Envelope`, `BookingCreated`, `NewBookingCreated`, `NewEnvelope`, `DecodePayload` | Ya |
| `event_test.go` | Lima skenario | Tidak |

Batasan: struct `Envelope` jangan diubah. Di `BookingCreated`, nama dan tipe field `BookingID`, `RoomType`, dan `Rooms` jangan diubah. Field lain boleh dihapus, diganti tipenya, dan diberi tag.

### Step 0: A Struct Becomes JSON and Back

```bash
cd labs/03-events
go test -run Step0 -v
```

Hasilnya `PASS`. `json.Marshal` dan `json.Unmarshal` bekerja pada struct apa pun. Lihat bentuk JSON yang dihasilkan kode awal:

```bash
go test -run Step1 -v
```

### Step 1: The Payload Is a Contract

Perintah di atas gagal dengan belasan baris. Tiga jenis keluhan muncul: field yang diharapkan tidak ada (`payload tidak punya field "booking_id"`), nama field mengikuti Go (`field "BookingID" mengikuti nama field Go`), dan data yang tidak boleh keluar ikut terkirim (`payload membawa nomor kartu di field "CardNumber"`).

**Task:** rancang ulang `BookingCreated` dan `NewBookingCreated`. JSON-nya harus memuat `booking_id`, `room_type`, `check_in`, `check_out`, dan `rooms`. Tanggal berbentuk `2026-12-24`. Tidak ada nomor kartu, tidak ada ID internal, dan semua nama field ditentukan lewat tag.

> [!hint]- Hint
> Lihat `BookingCancelled` di `booking.go` sebagai contoh tag. Ganti tipe `CheckIn` dan `CheckOut` menjadi `string`, lalu isi dengan `b.CheckIn.Format(time.DateOnly)`. Hapus `CardNumber` dan `InternalID` dari struct event. `GuestEmail` boleh tetap ada, karena service email membutuhkannya.

### Step 2: The Envelope Carries Metadata

```bash
go test -run Step2 -v
```

`NewEnvelope` di kode awal hanya mengisi payload. Pesan gagalnya menyebut alasan tiap field: `envelope tidak punya ID: consumer tidak bisa mengenali pesan yang sama datang dua kali`, lalu `Name`, `CorrelationID`, dan `OccurredAt`.

**Task:** isi semua field envelope. `ID` harus unik untuk setiap pemanggilan. `OccurredAt` adalah waktu sekarang dalam UTC.

> [!hint]- Hint
> Stdlib tidak punya UUID. Enam belas byte dari `crypto/rand` yang diubah dengan `hex.EncodeToString` sudah cukup unik. Untuk waktu, pakai `time.Now().UTC()`.

### Step 3: Decode by Event Name

```bash
go test -run Step3 -v
```

`DecodePayload` menerima envelope dan harus mengembalikan `BookingCreated` atau `BookingCancelled` sebagai value, sesuai `env.Name`. Nama yang tidak dikenal menghasilkan `ErrUnknownEvent`.

**Task:** tulis `DecodePayload` dengan `switch env.Name`.

> [!hint]- Hint
> Untuk tiap `case`, deklarasikan variabel bertipe event-nya, lakukan `json.Unmarshal(env.Payload, &e)`, lalu `return e, nil`. Di `default`, bungkus error-nya supaya nama event ikut terbaca: `fmt.Errorf("%w: %q", ErrUnknownEvent, env.Name)`. Test memeriksa dengan `errors.Is`.

### Step 4: Tolerate the Unknown, Insist on the Required

```bash
go test -run Step4 -v
```

Test mengirim empat payload: satu dengan field baru `loyalty_tier`, satu tanpa `booking_id`, satu dengan `rooms` bernilai 0, dan satu JSON yang terpotong. Tebak dulu: mana yang sudah ditangani dengan benar oleh kode Step 3, dan mana yang lolos diam-diam?

**Task:** payload dengan field baru harus tetap terbaca. Tiga lainnya harus menghasilkan `ErrInvalidEvent`.

> [!hint]- Hint
> Field baru sudah diabaikan oleh `encoding/json`; tidak ada yang perlu dikerjakan. Untuk tiga lainnya: bungkus error dari `json.Unmarshal` dengan `ErrInvalidEvent`, lalu setelah unmarshal berhasil, periksa `e.BookingID == ""`, `e.RoomType == ""`, dan `e.Rooms <= 0`.

### Finish

```bash
go test -race -v
```

## Solution

> [!success]- Solution
> Ide kuncinya: event adalah struct tersendiri yang ditulis untuk pembacanya, dan fungsi `NewBookingCreated` adalah satu-satunya tempat model internal diterjemahkan ke kontrak.
>
> ```go
> type BookingCreated struct {
> 	BookingID  string `json:"booking_id"`
> 	RoomType   string `json:"room_type"`
> 	CheckIn    string `json:"check_in"`
> 	CheckOut   string `json:"check_out"`
> 	Rooms      int    `json:"rooms"`
> 	GuestEmail string `json:"guest_email"`
> }
> ```
>
> Di sisi pembaca, setiap `case` melakukan dua pemeriksaan yang berbeda sifatnya: apakah JSON-nya sah, dan apakah field yang wajib memang terisi.
>
> ```go
> case BookingCreatedName:
> 	var e BookingCreated
> 	if err := json.Unmarshal(env.Payload, &e); err != nil {
> 		return nil, fmt.Errorf("%w: decode %s: %v", ErrInvalidEvent, env.Name, err)
> 	}
> 	if e.BookingID == "" || e.RoomType == "" || e.Rooms <= 0 {
> 		return nil, fmt.Errorf("%w: %s needs booking_id, room_type, and rooms > 0", ErrInvalidEvent, env.Name)
> 	}
> 	return e, nil
> ```
>
> Kode lengkap: `labs/03-events/_solusi/event.go`.

## When This Is the Wrong Choice

Envelope buatan sendiri adalah satu kontrak lagi yang harus dijaga. Kalau broker atau library yang dipakai sudah menyediakan tempat metadata, memakainya lebih murah daripada membungkus ulang. Envelope di dalam JSON masuk akal kalau pesan harus melewati beberapa jenis transport, misalnya Kafka lalu webhook HTTP, dan metadatanya tidak boleh hilang di perbatasan.

JSON juga bukan satu-satunya format. Ia mudah dibaca manusia dan tidak butuh alat tambahan, tapi tidak punya skema yang dipaksakan: tidak ada yang mencegah producer mengirim `rooms` sebagai string. Format dengan skema seperti Protobuf atau Avro menangkap kesalahan itu saat build. Harganya adalah alat tambahan dan pesan yang tidak bisa dibaca dengan mata. Untuk beberapa service yang dipegang satu tim, JSON dengan validasi di sisi pembaca biasanya cukup.

Event sendiri bisa menjadi pilihan yang salah. Kalau pengirim butuh memastikan satu penerima tertentu mengerjakan sesuatu dan ingin tahu hasilnya, menyamarkannya sebagai event hanya menyembunyikan ketergantungan yang nyata. Itu command, atau bahkan panggilan synchronous.

## What Is Still Broken

1. **Bentuk event akan berubah, dan lab ini hanya menangani penambahan field.** Mengganti nama field, mengubah makna, atau memecah satu event menjadi dua butuh strategi versi. Lihat [[15 Lab - Versioned Events]].
2. **`ErrUnknownEvent` dan `ErrInvalidEvent` belum ditanggapi siapa pun.** Fungsi ini hanya mengembalikannya. Consumer yang memutuskan mana yang dilewati, mana yang dicoba ulang, dan mana yang disisihkan ada di [[06 Lab - Errors]].
3. **`correlation_id` baru diisi, belum diteruskan.** Handler yang menerbitkan event lanjutan harus menyalin nilainya, dan itu mudah terlupa kalau dikerjakan tangan. Lihat [[05 Lab - Middlewares]].
4. **`id` belum dipakai untuk apa pun.** Ia baru berguna saat consumer mulai menolak pesan ganda. Lihat [[09 Lab - At-Least-Once Delivery]].

## Reflection Questions

1. PMS menerbitkan `BookingCreated`. Rekan satu tim mengusulkan menggantinya dengan `SendConfirmationEmail` karena "toh itu yang akan terjadi". Apa yang hilang kalau usul itu diterima, dan kapan usul itu benar?
2. Kenapa `check_in` dikirim sebagai `"2026-12-24"` sedangkan `occurred_at` dikirim sebagai timestamp UTC lengkap?
3. Event `BookingCreated` yang hanya berisi `booking_id` memaksa consumer memanggil PMS. Sebutkan dua hal yang bisa salah akibat panggilan itu, selain soal kecepatan.
4. Spesifikasi integrasi channel manager menuntut revisi booking diterapkan idempotent dan berurutan. Field envelope mana yang membantu syarat pertama, dan informasi apa yang belum ada di envelope ini untuk memenuhi syarat kedua?
5. Email yang dititipkan aplikasi di kantormu ke Kafka: menurutmu itu event atau command? Apa petunjuknya?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Struct Tags and JSON Marshalling]]
- [[Event Schema Evolution]]
- [[Case - A Schema Change That Broke a Partner Who Scraped Undocumented Fields]]
- [[Idempotency Keys]]
