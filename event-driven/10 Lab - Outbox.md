# 10 Lab - Outbox

[[09 Lab - At-Least-Once Delivery]] · [[00 Start Here]] · [[11 Lab - Commands]]

- **Goal:** menghilangkan dual write dengan menulis event ke tabel outbox dalam transaction yang sama dengan perubahan data, lalu membangun forwarder yang memindahkannya ke broker.
- **Covers:** dual write, outbox pattern, publish di dalam transaction, forwarder, `FOR UPDATE SKIP LOCKED`, kolom status dan attempts, baris dead.
- **Prerequisites:** [[09 Lab - At-Least-Once Delivery]]. Paham `BEGIN`/`COMMIT` di SQL.
- **Infrastructure:** Postgres (`make up` di root kursus).
- **Time:** 2,5–3 jam.

## Why This Exists

Service booking melakukan dua hal setiap kali tamu memesan: menyimpan booking ke database, dan mengabarkan `BookingCreated` ke broker supaya channel manager menurunkan availability di semua OTA. Keduanya menulis ke dua sistem berbeda. Tidak ada transaction yang mencakup keduanya.

Menulis ke dua sistem tanpa transaction bersama disebut **dual write**. Selalu ada celah di antara dua tulisan itu, dan proses bisa mati, jaringan bisa putus, atau broker bisa menolak tepat di celah tersebut.

```
Urutan A: commit dulu, publish kemudian
  INSERT booking ✔ COMMIT ✔ ──✖ mati di sini── publish (tidak pernah terjadi)
  Akibat: booking ada, event hilang. OTA masih menjual kamar itu. Double booking.

Urutan B: publish dulu, commit kemudian
  publish ✔ ──✖ mati di sini── COMMIT (tidak pernah terjadi)
  Akibat: event ada, booking tidak ada. OTA menutup kamar yang masih kosong.
```

Menukar urutan hanya menukar jenis kerusakannya. Retry di dalam request juga tidak menolong: kalau prosesnya mati, tidak ada lagi yang melakukan retry.

Yang membuat masalah ini berbahaya adalah kesunyiannya. Tidak ada error di log dan tidak ada request yang gagal dari sudut pandang tamu. Datanya hanya berbeda di dua tempat, dan baru ketahuan ketika dua tamu datang untuk kamar yang sama.

## Concepts

### Satu tulisan, bukan dua

Database sudah punya alat untuk "dua hal terjadi bersama atau tidak sama sekali", yaitu transaction. Outbox pattern memakai alat itu dengan satu trik: event tidak dikirim ke broker, melainkan **ditulis sebagai baris di tabel `outbox` pada database yang sama**, dalam transaction yang sama dengan booking.

```
BEGIN
  INSERT INTO bookings ...        ┐ satu transaction:
  INSERT INTO outbox (payload)... ┘ dua-duanya atau tidak sama sekali
COMMIT

            (nanti, oleh proses lain: forwarder)
outbox PENDING ──► publish ke broker ──► UPDATE outbox SET status = 'SENT'
```

Dual write hilang dari jalur request. Membuat booking sekarang hanya butuh database. Broker boleh mati satu jam: booking tetap diterima, dan event-nya menunggu di tabel.

Padanan terdekatnya adalah kotak surat keluar di kantor. Kamu menulis surat dan menaruhnya di kotak di mejamu sendiri; itu selalu berhasil karena kotaknya ada di ruanganmu. Kurir datang berkala, mengambil isi kotak, dan mengantarkannya. Kalau kurir sakit, surat menumpuk tapi tidak hilang. Yang tidak ditangkap analogi ini: kurir kantor pos tahu pasti surat mana yang sudah ia antar, sedangkan forwarder bisa mati setelah mengantar tetapi sebelum mencatatnya.

### Forwarder masih dual write, tetapi yang aman

Forwarder melakukan dua hal: publish ke broker dan menandai baris `SENT`. Itu dual write lagi. Bedanya ada pada arah kegagalannya.

```
publish ✔ ──✖ mati di sini── UPDATE status = 'SENT' (tidak terjadi)
Akibat: baris masih PENDING, jadi dikirim lagi. Event sampai dua kali.
```

Forwarder yang selalu melakukan publish dulu baru menandai tidak akan pernah kehilangan event. Ia hanya bisa menduplikasinya. Dengan kata lain, forwarder adalah pengirim at-least-once, dan consumer di ujung sana harus idempotent seperti di [[09 Lab - At-Least-Once Delivery]]. Outbox tidak menghapus kebutuhan itu. Ia mengubah "event bisa hilang" menjadi "event bisa dobel", dan dobel adalah masalah yang sudah kamu tahu cara menanganinya.

Supaya consumer bisa mengenali duplikat, kedua kiriman harus membawa ID yang sama. ID itu harus berasal dari baris outbox, bukan dibuat saat publish.

### Baris yang tidak mau terkirim

Sebagian kegagalan publish bersifat sementara: broker sedang restart, jaringan putus sebentar. Sebagian lagi permanen untuk baris itu: payload terlalu besar, topic tidak ada. Forwarder tidak selalu bisa membedakannya, jadi ia menghitung. Kolom `attempts` naik setiap kali gagal. Setelah melewati batas, status menjadi `DEAD` dan forwarder berhenti mencoba.

Baris dead bukan sampah. Ia pernyataan: "event ini belum sampai ke siapa pun, dan mesin sudah menyerah". Seseorang harus melihat `last_error`, memperbaiki sebabnya, lalu mengembalikan statusnya ke `PENDING`. Tabel outbox dengan kolom status sekaligus menjadi alat diagnosis: satu query menjawab "event mana yang tertahan, sejak kapan, dan kenapa".

Outbox billing di kantormu yang kamu pegang bekerja dengan bentuk yang sama: baris berstatus seperti `PENDING` diproses cron tiap 2 menit, dan baris yang terus gagal berakhir `DEAD`. Cron itu adalah forwarder; hanya saja tujuannya sistem billing, bukan broker.

### Dua forwarder dan `SKIP LOCKED`

Service yang berjalan dua replica punya dua forwarder. Kalau keduanya menjalankan `SELECT ... WHERE status = 'PENDING'` pada saat yang sama, keduanya mendapat baris yang sama dan mengirimnya dua kali.

`SELECT ... FOR UPDATE` mengunci baris yang terpilih sampai transaction selesai. Forwarder kedua akan menunggu, jadi duplikat hilang, tetapi kedua forwarder bekerja bergantian. `FOR UPDATE SKIP LOCKED` menambahkan satu perilaku: baris yang sedang dikunci dilewati. Forwarder kedua langsung mengambil batch berikutnya, dan keduanya bekerja paralel pada baris yang berbeda.

## The Exercise

Kode ada di `labs/10-outbox/`. Nyalakan Postgres dulu dengan `make up` di root kursus.

| File | Isi | Diubah? |
|---|---|---|
| `schema.go` | Tabel `bookings` dan `outbox`, konstanta status | Tidak, baca dulu |
| `publisher.go` | `FakePublisher` pengganti broker, bisa disuruh gagal | Tidak |
| `service.go` | `BookingService.CreateBooking` | Ya, Step 1–2 |
| `forwarder.go` | `Forwarder.RunOnce` | Ya, Step 3–5 |
| `lab_test.go` | Skenario; tiap test membuat schema Postgres sendiri | Tidak |

Broker di lab ini tiruan supaya kegagalannya bisa diatur persis. Database-nya Postgres sungguhan, karena transaction dan row lock tidak bisa ditiru dengan jujur.

### Step 0: The happy path

```bash
cd labs/10-outbox
go test -run Step0 -v
```

`PASS`. Kode awal melakukan dual write urutan A, dan selama broker sehat tidak ada yang terlihat salah. Baca `CreateBooking` di `service.go` dan tunjuk baris tempat proses tidak boleh mati.

### Step 1: The broker is down during a booking

```bash
go test -run Step1 -v
```

Test mematikan broker untuk satu panggilan `Publish`. Tebak dulu: booking-nya tersimpan atau tidak? `CreateBooking` return error atau tidak?

Test gagal dengan `CreateBooking gagal karena broker mati`. Lebih buruk dari itu: booking sudah di-commit, jadi pemanggil menerima error untuk booking yang sebenarnya tersimpan.

**Tugas:** ubah `CreateBooking` supaya tidak memanggil `Publish` sama sekali. Sebagai gantinya, tulis satu baris ke `outbox` dengan `topic = TopicBookingCreated` dan `payload` berisi JSON `BookingCreated`.

> [!hint]- Hint
> `pool.Begin(ctx)` memberi `pgx.Tx`. Jalankan kedua `INSERT` lewat `tx.Exec`, lalu `tx.Commit(ctx)`. Pasang `defer tx.Rollback(ctx)` segera setelah `Begin`; rollback setelah commit berhasil tidak melakukan apa pun. Hasil `json.Marshal` (`[]byte`) bisa langsung dikirim sebagai parameter kolom `jsonb`.

### Step 2: All or nothing

```bash
go test -run Step2 -v
```

Test ini memeriksa dua arah. Booking yang ditolak primary key tidak boleh meninggalkan baris outbox. Baris outbox yang gagal ditulis (test memasang trigger yang melempar exception) tidak boleh meninggalkan booking.

Kalau Step 1 sudah kamu kerjakan dengan satu transaction, test ini langsung lulus. Kalau kamu memakai dua `pool.Exec` terpisah, salah satu arah gagal, dan pesannya menunjukkan arah yang mana.

> [!hint]- Hint
> Dua `pool.Exec` berarti dua transaction kecil yang masing-masing langsung commit. Itu dual write lagi, hanya kali ini ke dua tabel.

### Step 3: The forwarder

```bash
go test -run Step3 -v
```

Booking sekarang aman, tetapi tidak ada event yang sampai ke broker. Test gagal dengan `RunOnce melaporkan 0 message, seharusnya 3`.

**Tugas:** implementasikan `Forwarder.RunOnce` di `forwarder.go`. Kontrak lengkapnya ada di komentar fungsi itu. Untuk step ini cukup: ambil baris `PENDING` urut `id` sebanyak `BatchSize`, publish satu per satu dengan `Message.ID` = id baris, tandai `SENT` dan isi `sent_at`.

> [!hint]- Hint
> Buka transaction, `SELECT id, topic, payload FROM outbox WHERE status = 'PENDING' ORDER BY id LIMIT $1`, kumpulkan hasilnya ke slice dulu (misalnya dengan `pgx.CollectRows`), baru lakukan publish dan `UPDATE` di dalam loop. Satu koneksi tidak bisa menjalankan `UPDATE` selagi `rows` dari `SELECT` belum habis dibaca.

### Step 4: Failures, attempts, and dead rows

```bash
go test -run Step4 -v
```

Ada dua test di step ini.

`FailedPublishCountsAttemptsThenDead` membuat tiga booking, dan broker selalu menolak yang di tengah. Tuntutannya: dua baris lain tetap terkirim, baris yang gagal mencatat `attempts` dan `last_error`, menjadi `DEAD` setelah `MaxAttempts`, lalu tidak dicoba lagi.

`LostAckCausesDuplicateWithSameID` membuat `Publish` return error padahal message sudah sampai. Forwarder yang benar akan mengirimnya lagi. Test ini **mengharapkan** dua message di broker, dan memeriksa bahwa keduanya membawa ID yang sama.

**Tugas:** tangani error dari `Publish` per baris. Error itu bukan alasan untuk menghentikan `RunOnce`.

> [!hint]- Hint
> Di cabang gagal: `UPDATE outbox SET attempts = attempts + 1, last_error = $2, status = CASE WHEN attempts + 1 >= $3 THEN 'DEAD' ELSE status END WHERE id = $1`, lalu `continue`. Di sisi kanan `SET`, `attempts` masih bernilai lama, makanya `attempts + 1`.

### Step 5: Two forwarders

```bash
go test -run Step5 -v
```

Dua forwarder berjalan bersamaan pada 20 baris, dengan broker yang butuh 5 ms per message. Tanpa penguncian, keduanya membaca baris `PENDING` yang sama dan test gagal dengan `sampai di broker 2 kali`.

**Tugas:** pastikan satu baris hanya dipegang satu forwarder.

> [!hint]- Hint
> Tambahkan `FOR UPDATE SKIP LOCKED` di akhir `SELECT`. Kunci hanya bertahan selama transaction terbuka, jadi `SELECT`, publish, dan `UPDATE` harus berada di transaction yang sama.

Setelah lulus, lihat baris log `forwarder 1 mengirim ..., forwarder 2 mengirim ...` dan durasi test. Lalu ganti `SKIP LOCKED` dengan `FOR UPDATE` saja dan jalankan beberapa kali. Test tetap lulus. Apa yang berubah, dan kenapa?

## Solution

> [!success]- Solution
> `CreateBooking` menjadi satu transaction dengan dua `INSERT`, tanpa menyentuh publisher:
>
> ```go
> tx, err := s.pool.Begin(ctx)
> if err != nil {
> 	return fmt.Errorf("begin: %w", err)
> }
> defer func() { _ = tx.Rollback(ctx) }()
>
> // INSERT INTO bookings ...
> // INSERT INTO outbox (topic, payload) VALUES ($1, $2)
> return tx.Commit(ctx)
> ```
>
> Inti `RunOnce` adalah query pengambilnya dan urutan "publish dulu, tandai kemudian":
>
> ```sql
> SELECT id, topic, payload FROM outbox
> WHERE status = 'PENDING'
> ORDER BY id LIMIT $1
> FOR UPDATE SKIP LOCKED
> ```
>
> Jawaban pertanyaan Step 5: dengan `FOR UPDATE` biasa, hasilnya tetap benar dan pembagiannya tetap 10 dan 10, tetapi durasinya naik. Forwarder kedua berhenti di baris pertama yang terkunci dan menunggu sampai forwarder pertama commit. Setelah itu Postgres memeriksa ulang baris tersebut, melihat statusnya bukan lagi `PENDING`, dan melanjutkan ke baris berikutnya. Kedua forwarder bekerja bergantian, bukan paralel. Dengan `SKIP LOCKED`, forwarder kedua tidak menunggu sama sekali.
>
> Kode lengkap: `labs/10-outbox/_solusi/`.

## When This Is the Wrong Choice

Outbox menambah satu tabel, satu proses, dan satu jeda waktu. Ada keadaan di mana itu tidak sepadan.

Kalau event-nya boleh hilang, dual write urutan A sudah cukup. Notifikasi "ada yang sedang melihat kamar ini" tidak perlu outbox.

Kalau perubahan data dan event tidak berada di database yang sama, outbox tidak bisa dipakai apa adanya. Seluruh triknya bergantung pada satu transaction lokal.

Kalau consumer-nya hanya satu dan ia berada di service yang sama, tabel outbox itu sendiri sudah menjadi antreannya. Menambah broker di belakangnya berarti dua antrean untuk satu pekerjaan. Vault punya case study tentang keputusan ini.

Kalau volume event sangat tinggi, polling dengan transaction yang terbuka selama publish menjadi leher botol. Koneksi database tertahan selama broker lambat, dan baris `SENT` yang menumpuk memperlambat tabel. Pilihan pada skala itu adalah membaca log database dengan change data capture, dan itu di luar lab ini.

Forwarder di lab ini juga tidak menjaga urutan ketika ada kegagalan: baris yang gagal dicoba lagi setelah baris sesudahnya terkirim. Kalau urutan penting, lihat [[13 Lab - Message Ordering]].

## What Is Still Broken

1. **Consumer belum idempotent.** Step 4 membuktikan duplikat terjadi. Lab ini berhenti di broker; penerimanya harus memakai teknik lab 09 dengan `Message.ID` atau business key.
2. **Tidak ada yang melihat baris dead.** Baris `DEAD` yang tidak memicu alert sama saja dengan event yang hilang, hanya lebih lambat ketahuannya. Itu [[17 Lab - Metrics and Alerting]].
3. **Tidak ada jeda antar percobaan.** Baris yang gagal dicoba lagi di `RunOnce` berikutnya. Tiga percobaan bisa habis dalam beberapa detik, padahal broker mungkin butuh satu menit untuk pulih. Backoff dan circuit breaker ada di [[19 Lab - Fault Tolerance]].
4. **Baris `SENT` tidak pernah dihapus.** Tabel tumbuh selamanya. Sistem nyata butuh pembersihan berkala atau partitioning.
5. **Transaction terbuka selama publish.** Sederhana dan benar, tetapi satu koneksi database tertahan selama broker lambat.

## Reflection Questions

1. Jelaskan dengan kata-katamu sendiri kenapa menukar urutan "commit lalu publish" menjadi "publish lalu commit" tidak menyelesaikan dual write.
2. Forwarder melakukan publish dulu baru menandai `SENT`. Apa yang terjadi kalau urutannya dibalik, dan kenapa itu lebih buruk?
3. Di outbox billing di kantormu, sebuah baris berstatus `DEAD`. Sebelum mengembalikannya ke status pending, dua hal apa yang harus kamu pastikan tentang sistem tujuannya supaya tidak ada tagihan ganda?
4. Cron outbox billing itu berjalan tiap 2 menit. Kalau satu putaran butuh lebih dari 2 menit dan putaran berikutnya mulai sebelum yang pertama selesai, apa yang bisa terjadi tanpa row lock? Bagian mana dari lab ini yang mencegahnya?
5. Spesifikasi channel manager meminta perubahan availability ditangkap lewat outbox dalam transaction yang sama, bukan dengan polling tabel booking. Apa yang bisa terlewat oleh polling tabel booking yang tidak terlewat oleh outbox?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Dual Writes and Their Dangers]]
- [[The Transactional Outbox Pattern]]
- [[Database Transactions]]
- [[Case - Choosing Between Kafka and a Database Outbox Table for One Consumer]]
- [[Change Data Capture]]
