# 13 Lab - Message Ordering

[[12 Lab - Read Models]] · [[00 Start Here]] · [[14 Lab - Data Lake]]

---

**Goal:** melihat sendiri kapan urutan event terjaga dan kapan tidak, memilih partition key yang benar di Kafka, lalu menulis handler yang tetap benar walau event datang tidak berurutan.
**Covers:** message ordering, partition key, hot partition, entity version, stale event, gap.
**Prerequisites:** [[02 Lab - Message Broker]] (topic, partition, consumer group) dan [[09 Lab - At-Least-Once Delivery]] (redelivery, idempotent handler).
**Infrastructure:** Kafka untuk Step 1. Step 0, 2, 3, dan 4 berjalan di memory.
**Time:** 2–3 jam.

---

## Why This Exists

Tamu memesan dua kamar, lalu mengubahnya menjadi tiga, lalu membatalkan. PMS menerbitkan tiga event: `BookingCreated`, `BookingModified`, `BookingCancelled`. Consumer yang menghitung availability harus berakhir dengan nol kamar terjual.

Sekarang balik urutannya. `BookingCancelled` diproses lebih dulu, dan consumer tidak menemukan booking untuk dibatalkan, jadi ia mengabaikannya. `BookingCreated` menyusul, dan booking itu hidup selamanya. Kamar yang sudah kosong tidak bisa dijual, tanpa satu pun error di log.

Pengirim tidak salah di sini, karena ia menerbitkan ketiga event dalam urutan yang benar. Urutan itu hilang di perjalanan, dan ada tiga tempat ia bisa hilang.

## Concepts

### Tiga cara urutan hilang

**Partition yang berbeda.** Topic Kafka dipecah menjadi beberapa partition supaya bisa diproses paralel. Kafka menjaga urutan record di dalam satu partition, dan tidak menjanjikan apa pun antar-partition. Kalau `BookingCreated` masuk partition 0 dan `BookingCancelled` masuk partition 2, dua consumer yang berbeda memprosesnya. Siapa yang lebih dulu selesai bergantung pada beban masing-masing.

```
producer:   Created(B-1) ──► Modified(B-1) ──► Cancelled(B-1)

partition 0: [Created B-1] ................  ──► consumer A (sedang lambat)
partition 2: [Cancelled B-1] ..............  ──► consumer B (langsung proses)
                                                  ▲
                                 Cancelled diproses sebelum Created
```

**Consumer yang paralel.** Urutan juga hilang kalau satu consumer membaca dari satu partition lalu membagikan record ke beberapa goroutine. Pembacaannya berurutan, tetapi pemrosesannya tidak.

**Retry.** Di [[06 Lab - Errors]] kamu melihat handler yang gagal lalu dicoba ulang. Kalau event revision 2 gagal dan diparkir untuk retry, sementara revision 3 berhasil, revision 2 akan diproses setelah revision 3. Semakin rajin sistem melakukan retry, semakin sering urutan terbalik.

### Partition key

Producer menentukan partition lewat **key** record. Client Kafka menghitung hash dari key, lalu mengambil sisa baginya terhadap jumlah partition. Key yang sama selalu menghasilkan partition yang sama.

Padanan terdekatnya adalah loket bank dengan beberapa antrean. Urutan hanya terjaga di dalam satu antrean. Kalau semua urusan satu nasabah selalu diarahkan ke antrean yang sama, urusan itu selesai berurutan, sementara nasabah lain tetap dilayani paralel di antrean sebelah. Analogi ini berhenti bekerja di satu titik: nasabah bisa pindah antrean kalau antreannya panjang, sedangkan record tidak bisa. Key menentukan partition secara mutlak, dan itu sumber masalah hot partition di bawah.

Pertanyaan desainnya menjadi: urutan apa yang harus terjaga? Untuk booking, jawabannya urutan antar-revisi satu booking. Urutan antara booking B-1 dan B-2 tidak penting. Jadi key yang tepat adalah booking ID.

`email-service` membuat pilihan yang sama dengan entity yang berbeda. Key record-nya alamat penerima, sehingga semua email untuk satu orang masuk ke satu partition dan terkirim dalam urutan dibuatnya.

### Biaya memilih key: hot partition

Key memusatkan record. Kalau satu key jauh lebih sibuk dari yang lain, partition tempat ia jatuh menjadi **hot partition**: satu consumer kewalahan sementara yang lain menganggur. Menambah consumer tidak membantu, karena satu partition hanya dibaca satu consumer dalam satu consumer group.

Contoh key yang berbahaya untuk hotel: `hotel_id`. Satu hotel besar dengan ribuan booking per hari akan memenuhi satu partition. Booking ID jauh lebih merata karena setiap booking hanya punya beberapa revisi.

Hash juga tidak menjamin pembagian rata untuk jumlah key yang sedikit. Kamu akan melihatnya di Step 1: empat booking dibagi ke tiga partition, dan satu partition tetap kosong.

Satu jebakan lagi: jumlah partition ikut masuk ke rumus. Menambah partition pada topic yang sudah berjalan mengubah hasil `hash(key) % jumlah partition`, sehingga revisi baru sebuah booking bisa jatuh ke partition yang berbeda dari revisi lamanya.

### Pertahanan kedua: entity version

Partition key menjaga urutan di dalam Kafka. Ia tidak melindungi dari retry, dari consumer yang memproses paralel, atau dari event yang datang lewat dua jalur. Spesifikasi channel manager yang kamu pegang punya dua jalur seperti itu: webhook, dan polling feed sebagai jaring pengaman.

Pertahanan yang bekerja di semua kasus itu ada di dalam event: sebuah **revision** yang naik satu setiap kali booking berubah. Handler membandingkan revision event dengan revision yang sudah ia terapkan, dan hanya ada tiga kemungkinan.

| Revision event | Artinya | Tindakan handler |
|---|---|---|
| `<= current` | Stale atau duplikat. Isinya sudah tercakup. | Abaikan, return `nil` (ack) |
| `== current + 1` | Tepat revisi berikutnya | Terapkan |
| `> current + 1` | Ada revisi yang belum datang (gap) | Jangan terapkan, return error supaya dikirim ulang |

Pembanding revision harus berasal dari pemilik data, yaitu PMS yang menaikkannya di dalam transaction yang sama dengan perubahan booking. Timestamp bukan pengganti yang aman, karena jam dua server tidak pernah persis sama.

## The Exercise

Kode ada di `labs/13-message-ordering/`.

| File | Isi | Diubah? |
|---|---|---|
| `event.go` | `BookingEvent`, `ErrGap`, `Booking` | Tidak |
| `publisher.go` | `RecordFor`: membungkus event menjadi record Kafka | Ya, Step 1 |
| `tracker.go` | `Tracker.Handle`: menerapkan event ke keadaan booking | Ya, Step 2–4 |
| `kafka_test.go` | Test Step 1 dan helper Kafka | Tidak |
| `tracker_test.go` | Test Step 0, 2, 3, 4 | Tidak |

Step 1 butuh Kafka. Jalankan `make up` di root kursus kalau belum.

### Step 0: Dunia ideal

```bash
cd labs/13-message-ordering
go test -run Step0 -v
```

Test ini lulus. Tiga event datang berurutan dan `Tracker` berakhir dengan booking yang batal. Baca `Tracker.Handle` sebelum lanjut dan catat satu hal: ia menulis `ev.Revision` ke keadaan, tetapi tidak pernah membacanya.

### Step 1: Satu booking, satu partition

```bash
go test -run Step1 -v
```

Test mengirim empat booking, masing-masing lima revisi, ke topic dengan 3 partition. Tebak dulu sebelum menjalankan: dengan key yang dipakai `RecordFor` sekarang, di mana lima revisi booking `B-101` akan berada?

Test gagal dengan pesan seperti ini:

```
partition 0: B-103#2 B-102#5
partition 1: B-101#1 B-104#1 B-101#2 B-101#3 B-102#3 ...
partition 2: B-102#1 B-103#1 B-102#2 B-104#2 B-103#3 ...
revisi booking B-101 tersebar di 2 partition [1 2], seharusnya 1
```

Baris `partition N:` menunjukkan isi tiap partition dalam urutan offset. Revisi `B-101` terpecah, jadi consumer partition 2 bisa memproses `B-101#4` sebelum consumer partition 1 selesai dengan `B-101#3`.

**Tugas:** ubah `RecordFor` supaya semua revisi satu booking masuk ke partition yang sama.

> [!hint]- Hint
> Key sekarang adalah `EventID`, yang unik untuk setiap event. Hash-nya menyebar ke mana-mana. Pakai nilai yang sama untuk semua event milik satu booking.

Setelah lulus, jalankan lagi dengan `-v` dan lihat baris `partition N:`. Satu partition kosong. Empat key hanya mengisi dua dari tiga partition, dan itu gambaran kecil dari pembagian beban yang tidak rata.

### Step 2: Revision yang stale

```bash
go test -run Step2 -v
```

Partition key sudah benar, tetapi broker tetap at-least-once. Test menerapkan revision 1, 2, 3, lalu mengirim ulang revision 2.

Test gagal: `booking B-1 = {Revision:2 Rooms:3 ...}, seharusnya {Revision:3 Rooms:5 ...}`. Event lama menimpa keadaan yang lebih baru. Tamu sudah punya lima kamar, dan sistem kembali mencatat tiga.

**Tugas:** buat `Handle` mengabaikan event yang revision-nya tidak lebih baru dari keadaan sekarang. "Mengabaikan" berarti return `nil`. Kalau kamu mengembalikan error, broker mengirim ulang event stale itu selamanya.

> [!hint]- Hint
> Booking yang belum dikenal punya revision 0, dan itu nilai yang kamu dapat dari `t.bookings[ev.BookingID].Revision` tanpa perlu cek `ok`. Bandingkan dengan `ev.Revision` di awal fungsi.

### Step 3: Gap

```bash
go test -run Step3 -v
```

`BookingCancelled` (revision 2) tiba sebelum `BookingCreated` (revision 1). Kode awal mengembalikan `nil` karena "tidak ada yang bisa dibatalkan", lalu pembatalan itu hilang.

Test gagal: `Handle(BookingCancelled revision 2) pada booking yang belum dikenal = <nil>, seharusnya error yang membungkus ErrGap`.

**Tugas:** kalau revision event melompati revision berikutnya, jangan sentuh keadaan dan kembalikan error yang membungkus `ErrGap`. Test memeriksanya dengan `errors.Is`.

> [!hint]- Hint
> `fmt.Errorf("booking %s: punya revision %d, menerima %d: %w", ..., ErrGap)`. Setelah pengecekan stale dan gap, yang tersisa hanya `ev.Revision == current+1`, jadi sisa fungsi boleh menerapkan event tanpa syarat tambahan.

### Step 4: Kacau total

```bash
go test -run Step4 -v
```

Tujuh event untuk dua booking, urutannya terbalik dan ada duplikat. Helper `deliverWithRedelivery` meniru broker: event yang handler-nya gagal dikirim ulang di putaran berikutnya.

Kalau Step 2 dan 3 benar, test ini lulus tanpa perubahan kode. Telusuri di kertas apa yang terjadi pada `B-1` di putaran pertama dan kedua, lalu jalankan semua test:

```bash
go test -v
```

## Solution

> [!success]- Solution
> Step 1 hanya mengganti key:
>
> ```go
> Key: []byte(ev.BookingID),
> ```
>
> Step 2 sampai 4 adalah tiga cabang di awal `Handle`:
>
> ```go
> current := t.bookings[ev.BookingID].Revision
>
> if ev.Revision <= current {
> 	return nil // stale atau duplikat: sudah tercakup, cukup ack
> }
> if ev.Revision > current+1 {
> 	return fmt.Errorf("booking %s: punya revision %d, menerima %d: %w",
> 		ev.BookingID, current, ev.Revision, ErrGap)
> }
> // dari sini ev.Revision == current+1: terapkan
> ```
>
> Perhatikan dua return yang berbeda. Stale mengembalikan `nil` karena tidak ada lagi yang perlu dilakukan. Gap mengembalikan error karena event itu masih dibutuhkan nanti. Menukar keduanya menghasilkan dua bug yang berbeda: event stale yang dikirim ulang tanpa henti, atau pembatalan yang hilang diam-diam.
>
> Kode lengkap ada di `_solusi/publisher.go` dan `_solusi/tracker.go`.

## When This Is the Wrong Choice

**Strict sequencing menahan seluruh booking kalau satu event hilang.** Kalau revision 2 tidak pernah datang, revision 3 dan seterusnya ditolak selamanya. Handler di lab ini aman hanya kalau ada jalur pemulihan: batas jumlah retry, lalu alert atau resync penuh dari sumbernya. Polling feed di spesifikasi channel manager adalah jalur semacam itu.

**Kalau setiap event membawa keadaan lengkap, gap boleh dilompati.** `BookingEvent` di lab ini membawa `Rooms` sebagai nilai mutlak. Untuk consumer yang hanya butuh keadaan terakhir, cukup aturan "terapkan kalau revision lebih besar". Aturan itu lebih sederhana dan tidak pernah tersangkut. Strict sequencing baru diperlukan kalau setiap revisi punya efek sendiri yang tidak boleh terlewat, misalnya mengirim notifikasi per perubahan, atau kalau event berisi selisih.

**Partition key tidak diperlukan kalau handler tidak peduli urutan.** Handler yang hanya menambah counter atau menyimpan event ke data lake memberi hasil yang sama untuk urutan apa pun. Memaksakan key di sana hanya menambah risiko hot partition.

**Urutan global hampir selalu permintaan yang salah.** Satu partition untuk semua event memberi urutan total, dengan harga satu consumer untuk seluruh sistem. Tanyakan dulu urutan per entity apa yang dibutuhkan.

## What Is Still Broken

1. **Revision hanya ada di memory.** `Tracker` lupa semuanya saat restart. Di sistem nyata, revision disimpan di baris booking dan dibandingkan di dalam transaction yang sama dengan perubahannya, misalnya `UPDATE ... WHERE revision = $1`. Polanya sama dengan [[10 Lab - Outbox]]: dua hal yang harus berubah bersama ditaruh dalam satu transaction.
2. **Event gap dicoba ulang tanpa batas.** Lab ini tidak punya batas retry atau tempat parkir. [[19 Lab - Fault Tolerance]] menutupnya dengan poison queue.
3. **Menambah partition merusak pemetaan key.** Tidak ada perbaikan di kode. Ini keputusan operasional: tentukan jumlah partition dengan cadangan sejak awal.
4. **Urutan antar-entity tidak dijaga.** Kalau `PaymentCaptured` harus diproses setelah `BookingCreated` dan keduanya punya key berbeda, partition key tidak menolong. Itu wilayah [[20 Lab - Sagas and Process Managers]].

## Reflection Questions

1. Kafka menjamin urutan di dalam partition. Sebutkan dua cara urutan tetap bisa terbalik walau semua event satu booking sudah berada di partition yang sama.
2. Di Step 2 event stale dijawab `nil`, di Step 3 event gap dijawab error. Apa yang terjadi di production kalau kedua jawaban itu tertukar?
3. `email-service` memakai alamat penerima sebagai key. Urutan apa yang dijaga pilihan itu? Bayangkan satu alamat yang menerima jauh lebih banyak email dari yang lain: apa akibatnya untuk consumer, dan apakah menambah consumer menolong?
4. Revisi booking dari channel manager bisa datang lewat webhook dan lewat polling feed. Jelaskan kenapa partition key tidak bisa menjaga urutan di antara dua jalur itu, dan apa yang bisa.
5. Sebuah handler hanya menambah counter "jumlah booking hari ini". Apakah ia butuh partition key? Apakah ia butuh revision?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Ordering Guarantees in Streaming Systems]]
- [[Topics, Partitions, and Offsets]]
- [[Sharding Strategies and Hot Partitions]]
- [[Term - Hot Partition]]
- [[Time and Ordering - Lamport and Vector Clocks]]
- [[Kafka]]
