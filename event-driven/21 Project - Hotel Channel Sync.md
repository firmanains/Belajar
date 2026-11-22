# 21 Project - Hotel Channel Sync

[[20 Lab - Sagas and Process Managers]] · [[00 Start Here]]

**Goal:** menyelesaikan satu service kecil yang menyinkronkan booking dari channel manager ke PMS hotel, di atas Postgres dan Kafka sungguhan, dengan delapan milestone yang masing-masing dijaga oleh satu test.
**Covers:** gabungan lab 02, 03, 04, 05, 06, 07, 09, 10, 12, 13, 17, dan 19. Lab 08, 11, 14, 15, 16, 18, dan 20 tidak dipakai di sini; lihat [[#Extensions]].
**Prerequisites:** minimal lab 09, 10, 12, dan 13 sudah selesai.
**Infrastructure:** Postgres dan Kafka (`make up` di root kursus).
**Time:** 3–5 sore.

---

## Goal

Dua puluh lab sebelumnya masing-masing mengisolasi satu konsep. Di sistem nyata konsep-konsep itu saling menginjak kaki. Outbox yang benar tetap menghasilkan event dobel, sehingga read model harus idempotent. Retry yang benar tetap bisa menyumbat partition, sehingga butuh dead letter topic. Graceful shutdown yang benar tetap membatalkan pekerjaan kalau waktunya habis, sehingga commit offset harus menunggu handler.

Proyek ini memaksa semua itu hidup di satu proses. Kerangkanya sudah ada: koneksi, migration, routing HTTP, test harness, dan interface. Bagian yang kosong adalah enam file yang membawa konsepnya.

Ceritanya: channel manager mengirim setiap perubahan booking ke webhook kita sebagai **revision**. Setiap revision punya `revision_id` yang unik dan `revision_number` yang naik per booking. Channel manager tidak menjamin apa pun soal pengiriman: revision bisa datang dua kali, bisa datang tidak berurutan, dan bisa datang bersamaan. Ini sama dengan tuntutan di spesifikasi integrasi channel manager yang pernah kamu baca: revision diterapkan idempotent dan berurutan, perubahan ditangkap lewat outbox dalam transaction yang sama, dan ack hanya setelah commit database.

## Architecture

```
 channel manager
       │  POST /webhooks/booking-revisions      (X-Webhook-Secret, X-Correlation-ID)
       ▼
 ┌─────────────────────────── satu transaction Postgres ───────────────────────────┐
 │ inbox (revision_id)   bookings   inventory (kamar terpakai per tanggal)   outbox │
 └──────────────────────────────────────────────────────────────────────────┬──────┘
       │ 200 dikirim setelah commit                                          │
       ▼                                                                     ▼
 channel manager                                         forwarder: polling tiap interval,
                                                         FOR UPDATE SKIP LOCKED,
                                                         publish, lalu tandai sent_at
                                                                             │
                                                                             ▼
                                 Kafka topic booking-events (3 partition, key = booking ID)
                                              │                              │
                         consumer group .projector            consumer group .email
                                              │                              │
                                              ▼                              ▼
                          rm_bookings, rm_tape_chart         EmailSender, retry + backoff
                                              │                              │ gagal terus / malformed
                                              ▼                              ▼
                     GET /tape-chart, GET /bookings/{id}     topic booking-events.dlq
```

Ada dua pasang tabel dengan bentuk yang sama. `bookings` dan `inventory` adalah **write model**: sumber kebenaran, ditulis oleh webhook. `rm_bookings` dan `rm_tape_chart` adalah **read model**: dibangun oleh projector dari event, dan boleh dihapus lalu dibangun ulang. Di PMS sungguhan kedua model itu biasanya berbeda bentuk, bahkan berbeda service. Di sini bentuknya disamakan supaya kamu bisa membandingkan keduanya dan melihat jeda di antaranya.

Peta kode di `proyek/`:

| Path | Isi | Kamu ubah? |
|---|---|---|
| `internal/event/` | Envelope, nama event, header Kafka | Tidak |
| `internal/platform/db.go`, `kafka.go`, `http.go` | Koneksi, migration, publisher, fault injection | Tidak |
| `internal/store/` | SQL untuk booking dan kamar terpakai | Tidak |
| `internal/inbox/webhook.go` | Handler HTTP webhook dan validasi | Tidak |
| `internal/inbox/apply.go` | Menerapkan revision | **Ya, step 1–3** |
| `internal/outbox/forwarder.go` | Outbox ke Kafka | **Ya, step 4** |
| `internal/readmodel/projector.go` | Event ke read model | **Ya, step 5** |
| `internal/email/consumer.go` | Confirmation email, retry, dead letter | **Ya, step 6** |
| `internal/obs/obs.go` | Metrics dan logger per message | **Ya, step 7** |
| `internal/platform/consume.go` | Loop consumer Kafka | **Ya, step 8** |
| `internal/app/`, `cmd/channelsync/` | Wiring dan `main` | Tidak |
| `harness_test.go`, `project_test.go` | Test harness dan delapan milestone | Tidak |
| `_solusi/` | Versi jadi dari enam file di atas | Jangan dibuka dulu |

Setiap bagian yang harus kamu isi ditandai `// TODO(step N):` di dalam kodenya.

## How to Run

Nyalakan infrastruktur satu kali, dari root kursus:

```bash
cd ~/Developer/playground/event-driven
make up
```

Test dijalankan per step dari folder `proyek/`:

```bash
cd ~/Developer/playground/event-driven/proyek
go test -run TestStep0 -v .
go test -run TestStep1 -v .
```

Setiap test menjalankan seluruh service di dalam proses test, dengan schema Postgres, topic Kafka, dan consumer group yang namanya acak. Test membersihkan semuanya setelah selesai, jadi kamu bisa mengulang sesukamu. Test yang gagal karena sesuatu "belum terjadi" menunggu 10 detik sebelum menyerah.

Setelah semua step selesai, periksa dengan race detector dan dengan pemeriksa kursus:

```bash
go test -race -count=1 .
cd .. && ./verify.sh proyek
```

`verify.sh` memeriksa kode awal dan `_solusi`, bukan pekerjaanmu. Kalau kamu sudah mengubah kode awal, ia akan melapor bahwa "kode awal harus gagal". Itu wajar.

### Poking It by Hand

Jalankan service di satu terminal:

```bash
WEBHOOK_SECRET=dev go run ./cmd/channelsync
```

Lalu kirim satu revision dari terminal lain:

```bash
curl -s -X POST localhost:8080/webhooks/booking-revisions \
  -H 'X-Webhook-Secret: dev' -H 'X-Correlation-ID: coba-1' \
  -d '{"revision_id":"rev-001","booking_id":"B-1001","revision_number":1,"status":"new",
       "room_type":"deluxe","check_in":"2026-12-10","check_out":"2026-12-12","rooms":2,
       "guest_email":"tamu@example.com"}'
```

Bandingkan write model dengan read model:

```bash
curl -s -H 'X-Webhook-Secret: dev' 'localhost:8080/admin/availability?from=2026-12-10&to=2026-12-12'
curl -s 'localhost:8080/tape-chart?from=2026-12-10&to=2026-12-12'
curl -s localhost:8080/bookings/B-1001
curl -s -H 'X-Webhook-Secret: dev' localhost:8080/admin/outbox
curl -s localhost:8080/metrics | grep channelsync
```

Lihat isi Kafka dan Postgres langsung. CLI Kafka di dalam container memakai `localhost:19092`:

```bash
cd ~/Developer/playground/event-driven
docker compose exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:19092 --topic channelsync.booking-events \
  --from-beginning --property print.key=true --property print.headers=true
docker compose exec kafka /opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server localhost:19092 --describe --group channelsync.email
docker compose exec postgres psql -U lab -c \
  'SELECT id, event_name, booking_id, sent_at FROM channelsync.outbox ORDER BY id'
```

Kolom `LAG` di keluaran `kafka-consumer-groups.sh` adalah jumlah record yang belum di-commit oleh consumer group itu. Angka itulah yang kamu jaga di step 8.

Service yang dijalankan dengan tangan memakai schema `channelsync` dan topic `channelsync.*`. Untuk mengulang dari nol, hapus schema itu dengan `DROP SCHEMA channelsync CASCADE` dan hapus topic-nya, atau jalankan `make down && make up`.

## The Exercise

### Step 0: The Ideal World

```bash
go test -run TestStep0 -v .
```

Test ini lulus di kode awal. Satu revision dikirim satu kali, dan kamar terpakai di write model bertambah. Sebelum lanjut, baca `harness_test.go` sampai kamu bisa menjawab: bagaimana test tahu bahwa sesuatu yang asinkron sudah selesai, tanpa `time.Sleep`? Polanya dari [[07 Lab - Component Testing]].

Baca juga `Apply` di `internal/inbox/apply.go`. Ia benar hanya kalau setiap revision datang tepat sekali dan berurutan.

### Step 1: Duplicate Revision

```bash
go test -run TestStep1 -v .
```

Tebak dulu angkanya. Test gagal dengan `kamar terpakai = 4, seharusnya 2`. Channel manager tidak menerima response, mengirim ulang, dan hotel kehilangan dua kamar yang tidak pernah dipesan.

**Tugas:** revision dengan `revision_id` yang sama hanya diterapkan sekali. Kiriman kedua tetap dijawab 200, dengan result `duplicate`. Konsepnya dari [[09 Lab - At-Least-Once Delivery]].

> [!hint]- Hint
> `markReceived` sudah ada di file yang sama. Ia mengembalikan `false` kalau `revision_id` sudah tercatat. Jawaban 200 untuk duplicate itu penting: kalau kamu menjawab error, channel manager akan mengirim ulang selamanya.

### Step 2: Modified, Cancelled, Stale, Concurrent

```bash
go test -run TestStep2 -v .
```

Empat subtest gagal. Revision `modified` menumpuk di atas state lama, `cancelled` tidak melepas kamar, revision lama yang terlambat menimpa state baru, dan dua belas revision yang datang bersamaan menghasilkan angka acak.

**Tugas:** ambil state lama, abaikan revision yang nomornya tidak lebih besar (result `stale`), lepas pemakaian kamar dari state lama, lalu pasang yang baru. Idenya dari [[13 Lab - Message Ordering]]: urutan kedatangan tidak bisa dipercaya, nomor versi bisa.

> [!hint]- Hint
> Subtest terakhir hanya lulus kalau semua langkah berada dalam satu transaction dan state lama diambil dengan `LockAndGetBooking`. Baca komentar fungsi itu di `internal/store/store.go`: ia memakai advisory lock karena `SELECT ... FOR UPDATE` tidak mengunci apa pun saat barisnya belum ada. Revision yang stale tetap perlu commit, supaya catatan inbox-nya tersimpan.

### Step 3: Booking and Event, Together or Not at All

```bash
go test -run TestStep3 -v .
```

Test gagal dengan `baris outbox = 0, seharusnya 1`. Booking tersimpan, tetapi tidak ada yang akan memberi tahu bagian lain.

Setelah itu test menyuntikkan kegagalan: insert ke outbox gagal satu kali. Yang dituntut: webhook menjawab 500, booking tidak tersimpan, dan kiriman ulang dari channel manager diterapkan seperti biasa.

**Tugas:** tulis event ke outbox di transaction yang sama. Ini [[10 Lab - Outbox]] dan bentuk event-nya dari [[03 Lab - Events]].

> [!hint]- Hint
> `event.New(eventName(old, next), correlationID, next)` membuat envelope-nya, dan `s.outbox.Insert(ctx, tx, env)` menulisnya. Perhatikan pesan gagal terakhir di test: kalau catatan inbox tidak ikut rollback, kiriman ulang dianggap duplicate dan booking itu hilang selamanya. Jadi `markReceived` harus memakai transaction yang sama.

### Step 4: From Outbox to Kafka

```bash
go test -run TestStep4 -v .
```

Test gagal dengan `belum terjadi setelah 10s: tiga event sampai di topic Kafka`. Baris outbox menumpuk dan tidak ada yang mengirimnya.

**Tugas:** isi `ForwardBatch` di `internal/outbox/forwarder.go`. Test juga membuat broker menolak tiga publish berturut-turut, lalu menuntut semua event tetap sampai dan outbox kembali kosong. Selain [[10 Lab - Outbox]], step ini memakai key dan partition dari [[02 Lab - Message Broker]].

> [!hint]- Hint
> Query-nya: `SELECT ... FROM outbox WHERE sent_at IS NULL ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`. Publish dulu, baru `UPDATE outbox SET sent_at = now() WHERE id = ANY($1)` untuk baris yang berhasil, lalu commit. Berhenti di kegagalan pertama supaya event sebuah booking tidak saling mendahului.

### Step 5: A Read Model That Survives Bad Delivery

```bash
go test -run TestStep5 -v .
```

Test gagal dengan `belum terjadi setelah 10s: B-1 muncul di read model`. Event sudah sampai di Kafka, tetapi projector membuangnya.

Bagian kedua test menulis langsung ke Kafka: event yang sama dua kali, revision 3 sebelum revision 2, dan satu message yang bukan JSON. Tape chart harus tetap benar dan projector tidak boleh macet.

**Tugas:** isi `Handle` di `internal/readmodel/projector.go`. Konsepnya dari [[12 Lab - Read Models]] dan [[13 Lab - Message Ordering]].

> [!hint]- Hint
> Logikanya hampir sama dengan step 2, memakai `store.ReadModel`. Tidak perlu tabel inbox: event dengan revision yang tidak lebih besar dari state di read model pasti duplicate atau stale. Message yang gagal `event.Decode` cukup ditulis ke log lalu `return nil`. Kalau kamu mengembalikan error, setelah step 8 consumer akan mengulang message itu selamanya.

### Step 6: Retry, Then Dead Letter

```bash
go test -run TestStep6 -v .
```

Test gagal dengan `belum terjadi setelah 10s: email B-1 terkirim setelah retry`.

Skenarionya empat. SMTP gagal dua kali lalu berhasil. SMTP selalu gagal untuk satu booking, dan booking berikutnya harus tetap dapat email. Satu message rusak. Satu event datang dua kali.

**Tugas:** isi `Handle` di `internal/email/consumer.go`. Pemilahan jenis kegagalan dari [[06 Lab - Errors]], dead letter queue dari [[19 Lab - Fault Tolerance]]. Ini padanan `emails.DLT` di `email-service`, dengan satu beda: di sini belum ada yang mencoba ulang isi dead letter topic.

> [!hint]- Hint
> Urutan di dalam handler: decode (gagal berarti dead letter), saring nama event, `alreadySent`, lalu loop percobaan. Jeda `c.backoff << (attempt - 1)` menghasilkan 1x, 2x, 4x. Bungkus jeda dengan `select` yang juga mendengarkan `ctx.Done()`. Record di dead letter topic harus membawa key, value, dan header aslinya, ditambah `event.HeaderDLQReason`.

### Step 7: Make It Visible

```bash
go test -run TestStep7 -v .
```

Test gagal dengan `belum terjadi setelah 10s: log consumer projector memuat correlation_id dari webhook`.

Sistem asinkron gagal tanpa suara. Tidak ada pengguna yang melihat 500 ketika email tidak terkirim atau outbox menumpuk. Test menuntut tiga hal: correlation ID dari webhook muncul di log kedua consumer, counter message per consumer dan outcome, dan dua gauge untuk backlog outbox.

**Tugas:** isi `internal/obs/obs.go`. Bentuk middleware dari [[05 Lab - Middlewares]], pilihan metric dari [[17 Lab - Metrics and Alerting]].

> [!hint]- Hint
> Correlation ID sudah mengalir sampai header Kafka kalau step 3 dan 4 benar: `event.New` menaruhnya di envelope, dan forwarder menyalinnya ke header. Yang kurang hanya sisi consumer. Di `Instrument`, buat logger dengan `log.With("consumer", ..., "correlation_id", r.Headers[event.HeaderCorrelationID])`, titipkan dengan `WithLogger`, lalu panggil `next`. Untuk backlog pakai `prometheus.NewGaugeFunc`, dan daftarkan semuanya ke `m.registry`, bukan ke registry global.

### Step 8: Stop Without Losing Work

```bash
go test -run TestStep8 -v .
```

Dua subtest gagal. Yang pertama: `email B-1 terkirim 0 kali, seharusnya 1: shutdown membatalkan pengiriman yang sedang berjalan`. Yang kedua: `offset consumer email yang di-commit = 1, seharusnya 0`.

Kode awal melakukan commit offset sebelum handler berjalan. Selama service tidak pernah berhenti, itu tidak terlihat. Begitu ada deploy di tengah pengiriman email, Kafka menganggap record itu selesai dan email itu hilang tanpa jejak.

**Tugas:** ubah `Run` dan `Shutdown` di `internal/platform/consume.go`. Commit setelah handler berhasil, ulangi handler yang gagal, dan saat shutdown tunggu pekerjaan yang sedang berjalan. Offset dan commit dari [[02 Lab - Message Broker]], graceful shutdown dari [[04 Lab - Watermill Basics]].

> [!hint]- Hint
> Kamu butuh dua context. Yang pertama menghentikan `PollFetches`. Yang kedua diberikan ke handler dan baru dibatalkan kalau waktu shutdown habis. Kalau handler berhenti karena context-nya dibatalkan, keluar tanpa commit. Subtest kedua juga memeriksa dead letter topic: kalau `Handle` di step 6 membuang message saat `ctx` dibatalkan, perbaiki di sana.

## Solution

> [!success]- Solution
> Kode lengkap ada di `proyek/_solusi/`, dengan path yang sama dengan file yang kamu ubah. Enam keputusan yang membuatnya benar:
>
> **Inbox (`apply.go`).** Satu transaction memuat empat tulisan: catatan inbox, booking, pemakaian kamar, dan outbox. Urutannya `markReceived`, `LockAndGetBooking`, cek stale, `Release` lama, `Occupy` baru, `SaveBooking`, `outbox.Insert`, commit. Catatan inbox sengaja ikut rollback saat ada yang gagal.
>
> **Forwarder (`forwarder.go`).** Publish dulu, tandai kemudian, dalam transaction yang memegang lock baris. Kalau proses mati di antara keduanya, baris dikirim lagi. Urutan sebaliknya kehilangan event.
>
> **Projector (`projector.go`).** Keputusan "terapkan atau abaikan" hanya dari nomor revision. Itu sekaligus menangani duplicate dan event yang terlambat. Ini mungkin karena event membawa state lengkap booking, bukan selisih.
>
> **Email (`consumer.go`).** Tiga jalan keluar yang berbeda: malformed langsung ke dead letter topic, gagal kirim dicoba ulang lalu ke dead letter topic, dan context dibatalkan mengembalikan error tanpa dead letter.
>
> **Observability (`obs.go`).** Label metric hanya `consumer` dan `outcome`. Booking ID dan correlation ID masuk log. Backlog diukur dengan dua angka: jumlah baris pending dan umur baris tertua.
>
> **Consumer loop (`consume.go`).** Intinya dua context dan satu aturan:
>
> ```go
> func (c *Consumer) process(r *kgo.Record) bool {
> 	for {
> 		err := c.handler(c.handlerCtx, toRecord(r))
> 		if err == nil {
> 			break
> 		}
> 		select { // gagal: jangan commit, jangan lompat ke record berikutnya
> 		case <-c.pollCtx.Done():
> 			return false
> 		case <-c.handlerCtx.Done():
> 			return false
> 		case <-time.After(c.backoff):
> 		}
> 	}
> 	// commit hanya sampai di sini kalau handler berhasil
> 	...
> }
> ```
>
> `Shutdown` membatalkan `pollCtx` lebih dulu, menunggu `Run` selesai, dan baru membatalkan `handlerCtx` kalau waktu yang diberikan habis.

## Design Decisions and Trade-offs

**Key Kafka = booking ID.** Semua event satu booking masuk partition yang sama, sehingga `BookingCreated` tidak didahului `BookingModified` di jalur normal. Harganya: satu booking yang sangat ramai tidak bisa disebar ke banyak consumer, dan satu message yang macet menahan semua booking lain di partition itu. Key ini mengurangi kekacauan urutan, tetapi tidak menjaminnya. Karena itu projector tetap memeriksa nomor revision.

**Event membawa state lengkap.** `BookingModified` berisi seluruh booking pada revision itu, bukan "kamar bertambah satu". Consumer bisa melompati event yang hilang atau terlambat dan tetap sampai di state yang benar. Harganya: payload lebih besar, dan consumer tidak tahu persis apa yang berubah tanpa membandingkan dengan state lamanya.

**Forwarder dengan polling, bukan change data capture.** Polling hanya butuh satu query dan satu goroutine, dan cara kerjanya bisa kamu baca dalam satu layar. Harganya: jeda sampai satu interval, query berulang walau outbox kosong, dan tabel outbox yang terus tumbuh. CDC (misalnya Debezium membaca write-ahead log) menghilangkan polling, tetapi menambah satu komponen infrastruktur yang harus dioperasikan. Untuk satu service dengan volume kecil, polling pilihan yang lebih jujur.

**`SKIP LOCKED` dan urutan.** `FOR UPDATE SKIP LOCKED` membuat dua forwarder tidak mengirim baris yang sama dan tidak saling menunggu. Dengan dua forwarder, urutan antarbaris tidak lagi terjamin: forwarder kedua bisa mengirim baris 11–20 sebelum forwarder pertama selesai dengan baris 1–10. Proyek ini hanya menjalankan satu forwarder per proses, dan perilaku dengan beberapa instance belum diuji.

**franz-go langsung, tanpa Watermill.** Step 8 adalah tentang kapan offset di-commit dan apa yang terjadi pada handler saat shutdown. Dua hal itu adalah isi dari router Watermill, jadi memakainya di sini berarti menyembunyikan pelajarannya. Harganya: loop consumer, retry, dan pembungkus handler ditulis sendiri, dan kode seperti itu lebih mungkin punya bug di sudut yang jarang dilalui dibanding library yang sudah dipakai banyak orang. Untuk service sungguhan, router yang sudah teruji adalah pilihan awal yang lebih masuk akal.

**Email dicatat di tabel, bukan dijamin tepat sekali.** `email_log` mencegah email kedua ketika event yang sama datang lagi. Ia tidak mencegah email kedua kalau proses mati di antara "SMTP menerima" dan "baris tercatat". Email tidak bisa ikut transaction database, jadi jaminan terbaik yang ada adalah at-least-once dengan jendela dobel yang sempit.

**Advisory lock per booking.** Lock ini menutup celah dua revision pertama dari booking yang sama. Kuncinya `hashtext` dari ID, jadi dua booking berbeda bisa kebetulan berbagi kunci dan saling menunggu sebentar. Itu memperlambat, tidak merusak.

## What Production Would Still Need

Kode ini ditulis oleh AI. Belum ada engineer berpengalaman yang memeriksanya, dan ia belum pernah dijalankan di bawah beban production. Test-nya membuktikan delapan perilaku yang disebut di atas pada satu laptop, tidak lebih. Yang belum ada:

- **Authentication yang layak.** Hanya ada shared secret di header. Belum ada tanda tangan HMAC atas body, rotasi secret, pembatasan IP, atau rate limit.
- **Perlindungan overbooking.** Service ini mencatat apa pun yang dikirim channel manager. Ia tidak menolak booking yang melebihi jumlah kamar.
- **Migration tooling.** Migration berupa daftar `CREATE TABLE IF NOT EXISTS` yang dijalankan saat start. Belum ada versi, rollback, atau cara mengubah kolom dengan aman.
- **Pembersihan outbox, inbox, dan `email_log`.** Ketiga tabel tumbuh selamanya.
- **Baris outbox yang tidak pernah bisa terkirim.** Belum ada kolom attempts atau status `DEAD` seperti di outbox billing di kantormu. Satu baris yang selalu ditolak broker akan menahan semua baris di belakangnya.
- **Pengolahan dead letter topic.** Message hanya dipindahkan. Belum ada alat untuk melihat, memperbaiki, dan mengirim ulang, dan belum ada cron seperti yang mengurus `emails.DLT`.
- **Dead letter untuk projector.** Projector melewati message malformed hanya dengan menulis log.
- **Rebalance.** Perilaku saat consumer bertambah atau berkurang di tengah beban belum diuji. Commit dilakukan per record, yang lambat.
- **Beberapa instance.** Dua instance service berjalan bersamaan (dua forwarder, dua consumer per group) belum pernah diuji.
- **Topic dengan satu replica.** Kafka lokal hanya satu broker. Production butuh replication factor 3 dan `min.insync.replicas` yang sesuai.
- **Schema event yang berevolusi.** Tidak ada versi di nama event dan tidak ada upcaster ([[15 Lab - Versioned Events]]).
- **Alert rule dan dashboard.** Metric-nya ada, aturan alert-nya belum ([[17 Lab - Metrics and Alerting]]).
- **Tracing.** Hanya correlation ID di log, belum ada span ([[18 Lab - Tracing]]).
- **Timeout dan circuit breaker untuk SMTP.** `EmailSender` dipanggil tanpa batas waktu sendiri ([[19 Lab - Fault Tolerance]]).
- **Load test dan security review.** Belum pernah dilakukan.

Satu hal yang teramati tetapi belum dipahami: dengan pengaturan default franz-go, record pertama setelah consumer start kadang baru diterima 5 detik kemudian. Kode ini menurunkan `FetchMaxWait` menjadi 500 ms dan jeda itu menghilang di test. Penyebab pastinya belum ditelusuri.

## Extensions

Lab yang belum terpakai bisa menjadi latihan lanjutan di proyek ini, tanpa test yang menjaga:

- [[14 Lab - Data Lake]]: consumer group ketiga yang menyimpan semua event mentah.
- [[16 Lab - Migrating Read Models]]: tape chart per rate plan, dibangun ulang dari data lake.
- [[15 Lab - Versioned Events]]: `BookingCreated_v2` dengan field baru, dan projector yang menerima keduanya.
- [[18 Lab - Tracing]]: span dari webhook sampai email.
- [[20 Lab - Sagas and Process Managers]]: booking yang butuh payment sebelum dikonfirmasi.
- [[08 Lab - Events with CQRS]] dan [[11 Lab - Commands]]: ganti loop consumer buatan sendiri dengan router dan bus Watermill, lalu bandingkan jumlah kodenya.

## Reflection Questions

1. Di step 3, kenapa catatan inbox harus berada di transaction yang sama dengan booking? Ceritakan urutan kejadian yang membuat sebuah booking hilang kalau tidak.
2. Forwarder melakukan publish di dalam transaction yang memegang lock baris outbox. Apa yang terjadi kalau broker butuh 30 detik untuk menjawab? Apa alternatifnya dan apa harganya?
3. Projector tidak punya tabel inbox, tetapi tetap idempotent. Sifat apa pada event yang membuat itu mungkin? Beri contoh event yang tidak punya sifat itu.
4. Outbox billing di kantormu diproses cron tiap 2 menit dan punya status `DEAD`. Bandingkan dengan forwarder di proyek ini: apa yang dimiliki outbox itu dan tidak dimiliki proyek ini? Kapan sebuah baris `DEAD` aman di-reset ke pending?
5. `email-service` memakai alamat penerima sebagai key record, proyek ini memakai booking ID. Urutan apa yang dijaga oleh masing-masing pilihan, dan apa yang terjadi pada message lain di partition yang sama ketika satu message gagal terus?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

Catatan di `../backend-engineering-vault`:

- [[Webhooks and How to Secure Them]]
- [[The Transactional Outbox Pattern]]
- [[Dead Letter Queues]]
- [[Graceful Shutdown]]
- [[Idempotency Keys]]
- [[Exactly-Once Delivery as an Illusion]]
- [[Dual Writes and Their Dangers]]
- [[Change Data Capture]]
- [[CQRS]]
- [[Case - Choosing Between Kafka and a Database Outbox Table for One Consumer]]
- [[Kafka]]
