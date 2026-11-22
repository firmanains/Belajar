# 15 Lab - Versioned Events

[[14 Lab - Data Lake]] · [[00 Start Here]] · [[16 Lab - Migrating Read Models]]

---

**Goal:** mengubah bentuk event tanpa mematikan consumer milik tim lain: membedakan perubahan yang aman dari yang breaking, menerbitkan versi baru berdampingan dengan versi lama, dan memisahkan event internal dari event publik.
**Covers:** schema evolution, backward dan forward compatibility, tolerant reader, event versioning, upcaster, internal vs public event.
**Prerequisites:** [[03 Lab - Events]] (envelope, nama event, marshalling). [[14 Lab - Data Lake]] membantu memahami kenapa event lama tidak pernah hilang.
**Infrastructure:** none.
**Time:** 2 jam.

---

## Why This Exists

PMS menerbitkan `BookingCreated` dengan field `total: 1500000`. Semua orang tahu itu rupiah, karena hotelnya di Indonesia. Lalu hotel mulai menerima booking dalam dolar, dan `total` perlu membawa mata uang.

Perubahan di sisi PMS memakan waktu lima menit: `total` menjadi `{"amount": 120, "currency": "USD"}`. Deploy berjalan, dan test PMS lulus semua.

Lima menit kemudian consumer milik tim channel sync berhenti memproses booking, karena ia mengharapkan angka dan menerima object. Tim itu tidak ikut rapat, tidak membaca merge request, dan tidak bisa deploy sampai minggu depan.

API HTTP punya masalah yang sama, dengan satu keringanan: pemanggil yang rusak langsung menerima error. Pada event, producer tidak pernah tahu siapa consumer-nya. Kerusakan muncul di service orang lain, di log orang lain, dan baru sampai ke kamu lewat tiket.

Masalahnya juga lebih panjang dari itu. Event lama tidak hilang. Ia masih ada di topic, di dead letter queue, dan di data lake dari lab 14. Kode yang membaca sejarah harus memahami setiap bentuk yang pernah diterbitkan.

## Concepts

### Event adalah kontrak begitu ada yang membacanya

Selama hanya kode kamu yang membaca sebuah event, bentuknya urusan kamu. Begitu tim lain berlangganan, bentuk itu menjadi janji. Nama event, nama field, tipe, dan arti setiap field adalah bagian dari janji tersebut.

Padanan terdekatnya adalah formulir resmi yang dipakai banyak instansi. Menambah kolom opsional di bagian bawah tidak mengganggu siapa pun. Mengganti arti kolom "jumlah" dari rupiah ke dolar tanpa mengganti nomor formulir membuat semua pihak yang mengisinya keliru, dengan penuh keyakinan. Analogi ini tidak menangkap satu hal: formulir lama akhirnya habis dan dibuang, sedangkan event lama tersimpan dan suatu hari diputar ulang.

### Dua arah compatibility

Producer dan consumer tidak pernah di-deploy bersamaan, jadi selalu ada masa ketika keduanya berbeda versi.

| Istilah | Siapa yang lebih baru | Pertanyaannya |
|---|---|---|
| **Backward compatible** | Consumer baru, event lama | Bisakah kode baru membaca event yang dibuat kemarin? |
| **Forward compatible** | Consumer lama, event baru | Bisakah kode lama membaca event yang dibuat besok? |

Backward compatibility dibutuhkan setiap kali kamu membaca sejarah. Forward compatibility dibutuhkan setiap kali producer di-deploy lebih dulu dari consumer, dan itu hampir selalu terjadi.

### Perubahan yang aman dan yang tidak

| Perubahan | Aman? | Syarat |
|---|---|---|
| Menambah field opsional | Ya | Consumer mengabaikan field yang tidak dikenal, dan punya default kalau field itu tidak ada |
| Menghapus field | Tidak | Consumer yang membacanya mendapat nilai kosong tanpa error |
| Mengganti nama field | Tidak | Sama dengan menghapus lalu menambah |
| Mengganti tipe field | Tidak | Decode gagal di consumer |
| Mengganti arti atau satuan | Tidak, dan paling berbahaya | Decode berhasil, angkanya salah |

Baris pertama punya dua syarat, dan keduanya ada di sisi consumer. Consumer yang memenuhinya disebut **tolerant reader**: ia hanya mengambil field yang ia butuhkan dan tidak mengeluh soal sisanya.

Baris terakhir pantas mendapat perhatian khusus. Mengganti `total` dari rupiah ke sen, dengan tipe yang tetap integer, tidak menggagalkan satu test pun. Tagihan yang salah seratus kali lipat baru ketahuan dari keluhan tamu.

### Breaking change: versi baru, nama baru

Kalau perubahan memang harus breaking, jangan ubah event lama. Terbitkan event baru dengan nama baru, `BookingCreated_v2`, berdampingan dengan yang lama.

```
                         ┌─► BookingCreated     ──► consumer lama (belum pindah)
PMS ── booking dibuat ───┤
                         └─► BookingCreated_v2  ──► consumer baru
```

Masa transisi berakhir ketika tidak ada lagi consumer yang membaca v1. Baru saat itu v1 berhenti diterbitkan. Producer tidak bisa menebak kapan itu terjadi, jadi ia harus bertanya, atau memantau consumer group yang masih aktif di topic v1.

Consumer yang sudah pindah ke v2 cukup berlangganan v2 untuk event baru. Untuk sejarah sebelum v2 ada, ia memakai **upcaster**: satu fungsi yang mengubah v1 menjadi v2 saat dibaca. Handler di belakangnya hanya mengenal bentuk terbaru. Pengetahuan yang dulu tersirat, bahwa `total` tanpa mata uang berarti rupiah, ditulis satu kali di upcaster dan tidak tersebar ke setiap handler.

### Internal event dan public event

Tidak semua event perlu menjadi kontrak. Event yang hanya dibaca kode di dalam service yang sama disebut **internal event**. Bentuknya boleh berubah di setiap deploy, karena producer dan consumer-nya di-deploy bersama.

**Public event** dibaca service lain, dan untuk itulah semua aturan di atas berlaku. Di antara keduanya ada satu fungsi penerjemah di batas service. Fungsi itu memilih field mana yang keluar, satu per satu.

Tanpa batas itu, struct internal menjadi kontrak secara tidak sengaja. Setiap kolom yang ditambahkan untuk keperluan dalam langsung terlihat oleh semua consumer. Sebagian mulai bergantung padanya, dan kolom itu tidak bisa diubah lagi.

## The Exercise

Kode ada di `labs/15-versioned-events/`.

| File | Isi | Diubah? |
|---|---|---|
| `events.go` | Semua bentuk event dan nama-namanya | Tidak |
| `reader.go` | `DecodeV1`: consumer milik tim channel sync | Ya, Step 1–2 |
| `publisher.go` | `MessagesFor`: apa yang diterbitkan PMS | Ya, Step 3 |
| `upcaster.go` | `Upcast`: v1 menjadi v2 | Ya, Step 4 |
| `boundary.go` | `ToPublic`: event internal menjadi publik | Ya, Step 5 |
| `events_test.go` | Enam test | Tidak |

Di lab ini kamu bergantian memegang dua peran: tim PMS (producer) dan tim channel sync (consumer).

### Step 0: Sepakat pada satu bentuk

```bash
cd labs/15-versioned-events
go test -run Step0 -v
```

Test ini lulus. Baca `DecodeV1` dan perhatikan baris `DisallowUnknownFields`. Seseorang menulisnya dengan niat baik.

### Step 1: Field baru yang tidak dikenal

```bash
go test -run Step1 -v
```

Peranmu: tim channel sync. PMS menambah field opsional `guest_note`. Tebak dulu apa yang terjadi pada consumer-mu.

Test gagal: `consumer berhenti karena field baru yang tidak ia butuhkan: ... json: unknown field "guest_note"`. Perubahan yang seharusnya paling aman mematikan consumer.

**Tugas:** jadikan `DecodeV1` tolerant reader.

> [!hint]- Hint
> `json.Unmarshal` sudah mengabaikan field yang tidak dikenal. Yang perlu dilakukan hanya menghapus decoder yang ketat itu.

### Step 2: Field opsional yang tidak ada

```bash
go test -run Step2 -v
```

Sekarang arah sebaliknya. Event lama, dibuat sebelum rate plan ada, tidak punya `rate_plan`. `DecodeV1` mengembalikan string kosong, dan setiap kode setelahnya harus menebak arti string kosong itu.

Test gagal: `RatePlan = "" untuk event tanpa rate_plan, seharusnya "standard"`.

**Tugas:** isi `DefaultRatePlan` kalau field itu tidak ada.

> [!hint]- Hint
> Satu `if` setelah `Unmarshal`. Default diberikan di satu tempat, saat membaca, supaya tidak ada handler yang melihat nilai kosong.

### Step 3: Breaking change

```bash
go test -run Step3 -v
```

Peranmu: tim PMS. `total` harus membawa mata uang. Baca `MessagesFor`: seseorang sudah "meng-upgrade" event dengan nama lama dan bentuk baru.

Test gagal: `consumer lama tidak bisa membaca "BookingCreated" lagi: json: cannot unmarshal object into Go struct field .total of type int64`. Itu pesan error yang akan muncul di log tim lain.

**Tugas:** terbitkan dua message. `BookingCreated` tetap berbentuk v1 dengan `total` berupa angka, dan `BookingCreated_v2` membawa `Money`.

Test juga memeriksa booking dalam USD. Pikirkan dulu sebelum membaca hint: bentuk v1 tidak punya tempat untuk mata uang, jadi apa yang seharusnya terjadi pada booking USD di topic v1?

> [!hint]- Hint
> Booking USD yang diterbitkan sebagai v1 akan dibaca consumer lama sebagai 120 rupiah. Tidak ada cara jujur untuk menyatakannya di v1. Jangan terbitkan v1 untuk booking non-IDR. Consumer lama tidak melihat booking itu sampai mereka pindah ke v2, dan itu lebih baik daripada melihat angka yang salah.

### Step 4: Upcaster

```bash
go test -run Step4 -v
```

Peranmu: consumer baru yang membangun read model dari sejarah di data lake. Sejarah itu berisi v1, sedangkan event baru berisi v2. `Upcast` sekarang hanya menerima v2.

Test gagal: `Upcast(BookingCreated): event "BookingCreated" tidak didukung`.

**Tugas:** buat `Upcast` menerima kedua nama dan selalu mengembalikan `BookingCreatedV2`. Event v1 mendapat `Currency: "IDR"`.

> [!hint]- Hint
> `switch m.Name`. Untuk v1, panggil `DecodeV1` yang sudah kamu perbaiki, karena default rate plan sudah ditangani di sana. Lalu petakan field demi field ke `BookingCreatedV2`.

### Step 5: Event internal yang bocor

```bash
go test -run Step5 -v
```

Peranmu kembali ke tim PMS. `ToPublic` melakukan marshal pada struct internal apa adanya.

Test gagal: `field yang keluar dari service: [booking_id cost_amount currency rate_plan room_type rooms row_id sell_amount staff_note]`. Harga pokok, catatan staf, dan ID baris database sekarang ada di broker, terbaca oleh setiap consumer.

**Tugas:** petakan event internal ke `BookingCreatedV2` secara eksplisit. Total publik adalah harga jual.

> [!hint]- Hint
> Bangun `BookingCreatedV2{...}` field demi field, lalu lakukan marshal pada struct itu. Jangan mencoba menyembunyikan field dengan tag `json:"-"` di struct internal. Cara itu membalik default: field baru otomatis ikut keluar kecuali seseorang ingat menyembunyikannya.

Lalu jalankan semuanya:

```bash
go test -v
```

## Solution

> [!success]- Solution
> Tolerant reader adalah decoder biasa ditambah default:
>
> ```go
> var ev BookingCreatedV1
> if err := json.Unmarshal(payload, &ev); err != nil {
> 	return BookingCreatedV1{}, fmt.Errorf("decode BookingCreated v1: %w", err)
> }
> if ev.RatePlan == "" {
> 	ev.RatePlan = DefaultRatePlan
> }
> ```
>
> Publisher menerbitkan v2 selalu, dan v1 hanya kalau v1 bisa menyatakannya dengan jujur:
>
> ```go
> msgs := []Message{{Name: NameBookingCreatedV2, Payload: v2}}
> if b.Total.Currency != "IDR" {
> 	return msgs, nil
> }
> // ... marshal BookingCreatedV1 dengan Total: b.Total.Amount, lalu append
> ```
>
> Upcaster menuliskan pengetahuan yang dulu tersirat:
>
> ```go
> case NameBookingCreatedV1:
> 	old, err := DecodeV1(m.Payload)
> 	// ...
> 	Total: Money{Amount: old.Total, Currency: "IDR"},
> ```
>
> Kode lengkap ada di `_solusi/`.

## When This Is the Wrong Choice

**Versioning untuk event yang hanya kamu baca sendiri.** Kalau producer dan consumer ada di satu service dan di-deploy bersama, ubah saja event-nya. Upcaster dan dua versi paralel untuk event internal adalah upacara tanpa penonton. Satu pengecualian: event internal yang tersimpan lama, misalnya di outbox atau data lake, tetap harus bisa dibaca kode baru.

**Menerbitkan v2 untuk perubahan yang sebenarnya kompatibel.** Field opsional baru tidak butuh nama baru. Setiap versi tambahan adalah satu topic lagi, satu bentuk lagi untuk dipelihara, dan satu migrasi lagi untuk setiap consumer. Versi baru hanya untuk perubahan yang memang breaking.

**Tolerant reader untuk data yang memang wajib ada.** Mengabaikan field yang tidak dikenal itu sehat. Mengisi default untuk field wajib yang hilang menyembunyikan bug producer. Kalau `booking_id` kosong, tolak event itu, jangan dikarang.

**Menerbitkan dua versi selamanya.** Masa transisi tanpa tanggal akhir menjadi keadaan permanen. Tentukan sejak awal siapa yang memutuskan v1 berhenti dan dari data apa keputusan itu diambil.

Industri tidak sepakat soal tempat nomor versi. Sebagian menaruhnya di nama event, seperti lab ini. Sebagian menaruhnya di header dan mempertahankan satu topic. Nama terpisah lebih sederhana untuk consumer, karena ia memilih versi lewat subscription dan tidak pernah menerima bentuk yang tidak ia pahami. Header lebih hemat topic, tetapi setiap consumer wajib memeriksa versi sebelum decode. Untuk tim kecil tanpa schema registry, nama terpisah lebih sulit disalahgunakan.

## What Is Still Broken

1. **Tidak ada yang mencegah breaking change masuk.** Lab ini mengandalkan disiplin. Pertahanan yang sebenarnya adalah test kontrak di pipeline, atau schema registry yang menolak schema yang tidak kompatibel. Keduanya di luar cakupan kursus ini.
2. **Consumer yang berlangganan v1 dan v2 sekaligus memproses setiap booking dua kali.** Selama transisi, satu booking terbit di kedua nama. Consumer harus memilih satu untuk event baru, atau memakai idempotency key berbasis booking ID seperti di [[09 Lab - At-Least-Once Delivery]].
3. **Dua message tidak terbit secara atomik.** `MessagesFor` mengembalikan dua message, dan pengirimnya bisa mati setelah mengirim yang pertama. Menaruh keduanya di outbox dalam satu transaction menutup lubang itu. Lihat [[10 Lab - Outbox]].
4. **Read model yang dibangun dari v1 tidak ikut berubah.** Bentuk event sudah baru, sedangkan tabel yang dibangun dari event lama masih berbentuk lama. Itu [[16 Lab - Migrating Read Models]].

## Reflection Questions

1. Menambah field opsional disebut perubahan aman. Sebutkan dua syarat di sisi consumer yang membuatnya aman, dan apa yang terjadi kalau masing-masing tidak dipenuhi.
2. Kenapa mengganti satuan sebuah field lebih berbahaya daripada mengganti tipenya?
3. Di Step 3, booking USD tidak diterbitkan sebagai v1. Apa akibatnya bagi consumer lama, dan kenapa itu tetap pilihan yang lebih baik?
4. Aplikasi di kantormu menaruh pesan email di Kafka dan `email-service` membacanya. Misalkan pesan itu perlu field baru yang wajib, contohnya jenis template. Dua aplikasi itu tidak di-deploy bersamaan. Tuliskan urutan deploy yang aman, dan sebutkan apa yang terjadi pada pesan lama yang masih menunggu di dead letter topic.
5. Kapan sebuah internal event berubah menjadi kontrak tanpa ada yang memutuskannya?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Event Schema Evolution]]
- [[Contract Negotiation and Versioning]]
- [[API Versioning]]
- [[Schema Evolution in Protobuf]]
- [[Expand-Contract Schema Migration]]
