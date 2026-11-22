# 02 Lab - Message Broker

[[01 Lab - Basic Async]] · [[00 Start Here]] · [[03 Lab - Events]]

---

**Goal:** memindahkan queue ke luar proses dengan Kafka, menulis subscriber sendiri, dan melihat cara kerja consumer group, offset, dan commit.
**Covers:** peran broker, publish/subscribe, consumer group, offset dan commit, broker sebagai detail implementasi.
**Prerequisites:** [[01 Lab - Basic Async]].
**Infrastructure:** Kafka. Jalankan `make up` di root kursus.
**Time:** 3–4 jam.

---

## Why This Exists

Lab 01 berakhir dengan satu lubang besar: queue hidup di memori proses. Proses mati, pekerjaan hilang.

Ada lubang kedua yang belum terlihat. Queue di dalam proses hanya bisa dibaca oleh proses itu sendiri. Begitu ada service kedua yang ingin tahu tentang booking baru, misalnya service laporan, `CreateBooking` harus diubah untuk memanggilnya. Setiap peminat baru berarti perubahan di PMS.

Message broker menjawab keduanya. Ia adalah proses terpisah yang tugasnya hanya menyimpan pesan dan menyerahkannya ke siapa pun yang berhak. Pengirim tidak perlu hidup saat pesan dibaca, pembaca tidak perlu hidup saat pesan dikirim, dan pengirim tidak perlu tahu siapa saja pembacanya.

Kamu sudah mengoperasikan satu: cluster Kafka tiga broker yang dipakai aplikasi di kantormu untuk menitipkan email. Lab ini membuka isinya.

## Concepts

### Topic, partition, offset

Kafka menyimpan pesan di **topic**. Topic dibagi menjadi beberapa **partition**. Setiap partition adalah log: deretan record yang hanya bisa ditambah di ujungnya. Posisi sebuah record di dalam partition disebut **offset**.

```
topic booking-events
  partition 0:  [0] [1] [2] [3] [4] ─►  (record baru masuk di sini)
  partition 1:  [0] [1] [2] ─►
  partition 2:  [0] [1] [2] [3] ─►
```

Record tidak dihapus saat dibaca. Di sinilah Kafka berbeda dari queue di lab 01: membaca tidak mengosongkan apa pun. Yang berubah hanya catatan "sudah sampai mana" milik pembaca.

Record punya **key**. Record dengan key yang sama selalu masuk ke partition yang sama. Urutan hanya dijamin di dalam satu partition, tidak antar partition. Akibatnya dibahas di [[13 Lab - Message Ordering]].

### Consumer group

Pembaca Kafka bergabung dalam **consumer group**, yang dikenali lewat namanya. Dua aturan mengatur semuanya:

1. Di dalam satu group, setiap partition dipegang oleh tepat satu anggota. Anggota group membagi pekerjaan.
2. Setiap group punya catatan offset sendiri. Group yang berbeda tidak saling memengaruhi.

```
                         ┌─ group "email"         consumer A ◄─ partition 0, 1
topic (4 partition) ─────┤                        consumer B ◄─ partition 2, 3
                         └─ group "channel-sync"  consumer C ◄─ partition 0, 1, 2, 3
```

Aturan pertama memberi **scaling**: menambah instance `email-service` dalam group yang sama membagi beban, bukan menggandakan email. Aturan kedua memberi **publish/subscribe**: service baru cukup memakai nama group baru untuk menerima semua pesan, tanpa mengubah pengirim.

Cara paling mudah memahaminya: topic adalah buku tamu hotel yang tidak pernah dihapus, dan setiap departemen punya pembatas bukunya sendiri. Housekeeping dan keuangan membaca buku yang sama dengan kecepatan masing-masing. Analogi ini berhenti bekerja di pembagian partition: dua petugas dari departemen yang sama tidak membaca halaman yang sama bergantian, mereka masing-masing memegang jilid yang berbeda.

Saat anggota bergabung atau keluar, Kafka membagi ulang partition. Proses itu disebut **rebalance** dan butuh waktu beberapa detik. Step 2 menunggunya selesai sebelum menerbitkan pesan.

### Commit: pembatas buku milik group

Consumer memberi tahu Kafka sampai mana ia sudah memproses dengan melakukan **commit** offset. Commit adalah satu-satunya ingatan group. Kalau consumer mati, penggantinya mulai dari offset terakhir yang di-commit.

Urutan antara memproses dan commit menentukan apa yang terjadi saat consumer mati di tengah jalan:

```
commit dulu, baru proses:   commit [2] ─► proses B-2 ✗ mati
                            pengganti mulai dari [3]         → B-2 hilang

proses dulu, baru commit:   proses B-2 ✗ mati (belum commit)
                            pengganti mulai dari [2]         → B-2 diproses lagi
```

Tidak ada pilihan ketiga yang bebas risiko. Hampir semua sistem memilih yang kedua, karena pesan yang diproses dua kali bisa ditangani oleh consumer, sedangkan pesan yang hilang tidak bisa dikembalikan. Harga pilihan itu dibayar di [[09 Lab - At-Least-Once Delivery]].

### Broker sebagai detail implementasi

Kode aplikasi di lab ini hanya mengenal dua interface kecil di `broker.go`: `Publisher` dan `Subscriber`. Kata "Kafka", "partition", dan "offset" tidak muncul di sana. Kalau besok broker diganti, yang ditulis ulang hanya `publisher.go` dan `subscriber.go`.

## The Exercise

Kode ada di `labs/02-message-broker/`. Nyalakan Kafka dulu:

```bash
make up          # di root kursus
```

| File | Isi | Diubah? |
|---|---|---|
| `broker.go` | `Message`, `Publisher`, `Handler`, `Subscriber` | Tidak |
| `publisher.go` | `KafkaPublisher` dengan franz-go, sudah jadi | Tidak, baca sebagai contoh |
| `subscriber.go` | `KafkaSubscriber` yang belum membaca apa pun | Ya |
| `kafka_test.go` | Lima skenario. Setiap test membuat topic sendiri | Tidak |

Kontrak `Subscribe`: ia baru kembali kalau `ctx` dibatalkan (return `nil`) atau handler mengembalikan error (return error itu).

Library yang dipakai adalah franz-go, package `kgo`. Dokumentasinya bisa dibaca dari terminal:

```bash
go doc github.com/twmb/franz-go/pkg/kgo.Client.PollFetches
```

### Step 0: Messages Live Outside the Process

```bash
cd labs/02-message-broker
go test -run Step0 -v
```

Hasilnya `PASS`. Test menerbitkan tiga pesan lewat `KafkaPublisher`, lalu membacanya dengan client lain. Pesan sudah berada di luar proses pengirim. Baca `publisher.go` dan perhatikan `ProduceSync`: publisher baru kembali setelah broker mengonfirmasi record tersimpan.

Kalau test gagal dengan pesan tentang `make up`, Kafka belum menyala.

### Step 1: Write the Consume Loop

```bash
go test -run Step1 -v
```

Test menerbitkan lima pesan, lalu menjalankan `Subscribe`. Kode awal hanya menunggu `ctx`, jadi setelah 20 detik muncul: `handler menerima 0 dari 5 pesan`.

**Task:** tulis consume loop di `Subscribe`.

> [!hint]- Hint
> Buat client dengan `kgo.NewClient(kgo.SeedBrokers(s.brokers...), kgo.ConsumerGroup(group), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))`, lalu `defer client.Close()`. Di dalam `for`, panggil `client.PollFetches(ctx)`. Kalau `ctx.Err() != nil`, return `nil`. Periksa `fetches.Errors()`. Telusuri record dengan `iter := fetches.RecordIter()` dan `for !iter.Done() { record := iter.Next() ... }`, lalu panggil handler dengan `Message{Key: string(record.Key), Value: record.Value}`. Opsi `AtStart` membuat group yang belum punya offset mulai dari record paling awal.

### Step 2: One Group, Two Consumers

```bash
go test -run Step2 -v
```

Topic punya empat partition. Dua consumer bergabung ke group yang sama, lalu 40 pesan diterbitkan. Sebelum menjalankan, tebak: berapa pesan yang diterima masing-masing consumer, dan apakah ada pesan yang diterima keduanya?

Kalau Step 1 sudah memakai `kgo.ConsumerGroup`, test ini lulus tanpa perubahan kode. Jalankan dengan `-v` beberapa kali dan perhatikan waktunya: beberapa detik di antaranya adalah rebalance.

Kalau test gagal dengan `ada pesan yang dikerjakan dua kali`, subscriber kamu membaca partition secara langsung tanpa bergabung ke group.

### Step 3: Two Groups, One Topic

```bash
go test -run Step3 -v
```

Group `email` dan group `channel-sync` membaca topic yang sama. Tebak dulu: apakah sepuluh pesan dibagi dua, atau masing-masing menerima sepuluh?

Test ini juga lulus tanpa perubahan kode. Perhatikan apa yang tidak terjadi: publisher tidak diubah, dan group `email` tidak tahu bahwa `channel-sync` ada.

### Step 4: The Consumer Dies Before Commit

```bash
go test -run Step4 -v
```

Tiga pesan diterbitkan: B-1, B-2, B-3. Consumer pertama berhasil memproses B-1, lalu handler-nya gagal di B-2, meniru proses yang mati di tengah pekerjaan. Consumer pengganti di group yang sama harus menerima `[B-2 B-3]`: tidak mengulang B-1, tidak melompati B-2.

Dengan consume loop dari hint Step 1, pesan gagalnya: `consumer pengganti menerima [B-1 B-2 B-3], seharusnya [B-2 B-3]`. Client masih melakukan commit otomatis di latar, dan commit itu belum sempat terjadi untuk B-1 saat `Subscribe` berhenti. Kapan sebuah offset di-commit tidak kamu kendalikan.

**Task:** ambil alih commit. Offset sebuah record di-commit tepat setelah handler berhasil memprosesnya, dan tidak di-commit kalau handler gagal.

> [!hint]- Hint
> Tambahkan opsi `kgo.DisableAutoCommit()` saat membuat client. Setelah handler mengembalikan `nil`, panggil `client.CommitRecords(ctx, record)`. Kalau handler mengembalikan error, return error itu tanpa commit.

### Finish

```bash
go test -race -v
```

## Solution

> [!success]- Solution
> Ide kuncinya ada di urutan dua baris: handler dulu, commit sesudahnya.
>
> ```go
> iter := fetches.RecordIter()
> for !iter.Done() {
> 	record := iter.Next()
> 	msg := Message{Key: string(record.Key), Value: record.Value}
> 	if err := handler(ctx, msg); err != nil {
> 		// Tidak ada commit. Anggota group berikutnya menerima record ini lagi.
> 		return fmt.Errorf("handle message %q: %w", msg.Key, err)
> 	}
> 	if err := client.CommitRecords(ctx, record); err != nil {
> 		return fmt.Errorf("commit offset %d: %w", record.Offset, err)
> 	}
> }
> ```
>
> Commit per record mudah dipahami tapi lambat, karena setiap record berarti satu perjalanan ke broker. Kode production biasanya melakukan commit per batch. Kode lengkap: `labs/02-message-broker/_solusi/subscriber.go`.

## When This Is the Wrong Choice

Broker adalah satu sistem lagi yang harus dijalankan, dipantau, dan di-upgrade. Cluster Kafka kantormu butuh tiga broker hanya supaya satu broker boleh mati. Untuk satu producer dan satu consumer di dalam satu aplikasi dengan satu database, tabel job di database itu sering kali cukup, dan ia ikut ter-backup bersama datanya. Vault membahas keputusan ini di case study tentang Kafka melawan tabel outbox.

Kafka juga pilihan yang kurang cocok kalau setiap pesan butuh penanganan terpisah: retry per pesan dengan jeda berbeda, prioritas, atau penjadwalan. Di Kafka, consumer tidak bisa melompati satu record yang bermasalah tanpa memindahkannya ke tempat lain. Queue seperti RabbitMQ, atau job queue di Postgres, lebih alami untuk pola itu.

Pembungkus `Publisher`/`Subscriber` pun punya harga. Interface yang terlalu sempit menyembunyikan hal yang kelak kamu butuhkan, misalnya header record atau nomor partition. Interface di lab ini sengaja kecil dan akan terasa sempit di lab berikutnya.

## What Is Still Broken

1. **Pesan bisa diproses dua kali.** Kalau consumer mati setelah handler selesai tapi sebelum commit, penggantinya memproses pesan itu lagi. Step 4 membuktikan sisi baiknya (tidak ada yang hilang); sisi buruknya adalah email ganda. Lihat [[09 Lab - At-Least-Once Delivery]].
2. **Satu pesan yang selalu gagal menghentikan consumer.** `Subscribe` kembali dengan error, proses di-restart, lalu gagal lagi di pesan yang sama. Ini disebut poison message. Lihat [[06 Lab - Errors]] dan [[19 Lab - Fault Tolerance]].
3. **Menyimpan booking dan menerbitkan pesan adalah dua langkah.** Kalau proses mati di antaranya, booking tersimpan tapi pesannya tidak pernah terbit. Lihat [[10 Lab - Outbox]].
4. **Isi pesan masih asal.** Test mengirim JSON seadanya. Bentuk pesan adalah kontrak antar-service dan perlu dirancang. Lihat [[03 Lab - Events]].
5. **Consume loop ditulis tangan.** Setiap service baru akan mengulang 40 baris yang sama. Lihat [[04 Lab - Watermill Basics]].

## Reflection Questions

1. Kenapa dua consumer dalam satu group tidak pernah menerima record yang sama selama group stabil, sedangkan dua group selalu menerima semuanya?
2. Topic punya empat partition dan group punya enam consumer. Apa yang dikerjakan consumer kelima dan keenam?
3. `email-service` memakai alamat penerima sebagai key record. Apa akibatnya terhadap urutan email untuk satu penerima, dan terhadap pembagian beban kalau satu alamat menerima jauh lebih banyak email daripada yang lain?
4. Di Step 4, consumer pengganti menerima B-2 lagi. Seandainya handler pertama sempat mengirim email untuk B-2 sebelum mati, apa yang dialami tamu?
5. Kamu ingin menambah service audit yang mencatat setiap email yang pernah dititipkan aplikasi kantormu, termasuk yang lama. Apa saja yang perlu diubah di aplikasi pengirim?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Kafka]]
- [[Consumer Groups and Rebalancing]]
- [[Queue vs Log Semantics]]
- [[Exactly-Once Delivery as an Illusion]]
- [[Case - Choosing Between Kafka and a Database Outbox Table for One Consumer]]
