# 12 Lab - Read Models

[[11 Lab - Commands]] · [[00 Start Here]] · [[13 Lab - Message Ordering]]

- **Goal:** membangun dua read model dari event di Postgres, menjaganya tetap benar saat event datang dobel, dan menyajikannya lewat HTTP.
- **Covers:** read model, projection, memperbarui read model dari event, read model untuk kebutuhan query yang berbeda, menyajikan read model lewat API, eventual consistency.
- **Prerequisites:** [[08 Lab - Events with CQRS]], [[09 Lab - At-Least-Once Delivery]], [[10 Lab - Outbox]] (transaction dengan pgx).
- **Infrastructure:** Postgres (`make up` di root kursus).
- **Time:** 2,5–3 jam.

## Why This Exists

Tape chart adalah layar yang paling sering dibuka di hotel: satu baris per room type, satu kolom per malam, tiap sel berisi sisa kamar. Resepsionis membukanya puluhan kali sehari, dan channel manager menanyakan hal yang sama untuk setiap OTA.

Tabel `bookings` di write model dirancang untuk hal lain. Bentuknya satu baris per booking dengan `check_in` dan `check_out`, karena itu bentuk yang membuat penulisan aman: satu booking, satu baris, satu transaction. Untuk menjawab pertanyaan tape chart dari tabel itu, database harus bekerja keras pada setiap request:

```sql
SELECT d::date, rt.capacity - COALESCE(SUM(b.rooms), 0)
FROM room_types rt
CROSS JOIN generate_series('2026-12-01'::date, '2026-12-14', '1 day') d
LEFT JOIN bookings b
       ON b.room_type = rt.room_type AND b.status = 'confirmed'
      AND b.check_in <= d AND b.check_out > d
WHERE rt.room_type = 'deluxe'
GROUP BY d, rt.capacity;
```

Query ini membentangkan setiap booking menjadi malam-malamnya, lalu menjumlahkan ulang, setiap kali layar dibuka. Dengan seratus ribu booking, pekerjaan itu mahal dan diulang tanpa henti untuk jawaban yang jarang berubah.

Index bisa menunda masalahnya, tetapi akarnya tetap: bentuk yang baik untuk menulis berbeda dari bentuk yang baik untuk membaca. Memaksa satu tabel melayani keduanya berarti salah satunya selalu dikorbankan.

## Concepts

### Jawaban yang disiapkan lebih dulu

**Read model** adalah tabel yang bentuknya mengikuti satu pertanyaan, bukan mengikuti aturan bisnis. Untuk tape chart, bentuknya `availability_view (room_type, day, rooms_sold)`: satu baris per sel layar. Membacanya tidak butuh `JOIN` terhadap booking dan tidak butuh penjumlahan.

Pekerjaan menjumlahkan tidak hilang. Ia dipindahkan dari saat membaca ke saat menulis: setiap kali event `BookingCreated` datang, sebuah **projector** menambah angka di baris-baris yang terkena. Proses mengubah event menjadi baris read model disebut projection.

```
write side                                read side
──────────                                ─────────
CreateBooking ──► bookings                booking_list_view  ◄── layar resepsionis
                    │                     availability_view  ◄── GET /availability
                    └─► BookingCreated ──► projector ──┘
                        (lewat outbox dan broker)
```

Cara termudah membayangkannya: write model adalah buku kas tempat setiap transaksi dicatat, dan read model adalah papan skor di dinding. Papan skor tidak menyimpan apa pun yang tidak ada di buku kas. Ia ada supaya orang tidak perlu menjumlahkan buku kas setiap kali ingin tahu skornya. Analogi ini berhenti bekerja pada satu hal: papan skor di stadion diperbarui oleh orang yang melihat langsung kejadiannya, sedangkan projector hanya tahu apa yang diberitahukan event, dan event bisa datang terlambat atau dua kali.

### Satu sumber, banyak bentuk

Satu aliran event bisa mengisi banyak read model, masing-masing untuk satu kebutuhan. Lab ini punya dua:

| Read model | Pertanyaan yang dijawab | Bentuk |
|---|---|---|
| `booking_list_view` | Siapa yang menginap malam ini? | Satu baris per booking |
| `availability_view` | Berapa kamar tersisa per malam? | Satu baris per room type per malam |

Read model boleh berlebih dan boleh mengulang data. Ia tidak perlu dinormalisasi, karena ia bukan sumber kebenaran. Kalau rusak atau bentuknya perlu diganti, ia dibuang dan dibangun ulang dari event. Itu topik [[16 Lab - Migrating Read Models]].

### Projector harus idempotent

Event sampai ke projector dengan jaminan at-least-once, jadi semua yang dibahas di [[09 Lab - At-Least-Once Delivery]] berlaku di sini. Kedua read model di lab ini bereaksi berbeda terhadap duplikat:

- `booking_list_view` diisi dengan upsert. Memproyeksikan booking yang sama dua kali menghasilkan baris yang sama. Ia idempotent secara alami.
- `availability_view` diisi dengan `rooms_sold = rooms_sold + n`. Itu selisih, bukan fakta. Dua kali diproyeksikan berarti dua kali ditambah.

Untuk yang kedua, projector mencatat event ID di tabel `processed_events` **dalam transaction yang sama** dengan perubahan read model. Itu versi tahan-restart dari map `processed` di lab 09: tanda dan efeknya tersimpan bersama atau tidak sama sekali.

### Eventual consistency dari kursi pengguna

Projector berjalan terpisah dari request yang menulis booking. Di antara "booking tersimpan" dan "tape chart berubah" ada jeda. Biasanya beberapa milidetik. Saat consumer tertinggal atau mati, jedanya bisa menit atau jam. Sifat ini disebut eventual consistency: read model pasti menyusul, tetapi tidak ada janji kapan.

Yang dialami pengguna adalah ini: resepsionis membuat booking, layar kembali ke tape chart, dan kamarnya masih terlihat kosong. Ia menekan refresh dan angkanya berubah. Sistemnya tidak salah, tetapi bagi resepsionis itu terlihat seperti bug.

Tiga hal yang bisa dilakukan, dari yang paling murah:

1. **Jujur di layar.** Tampilkan "data per 14.02.31", atau tandai baris yang baru dibuat sebagai "sedang diproses". Pengguna menerima jeda yang diberitahukan; yang membuat mereka tidak percaya adalah jeda yang disembunyikan.
2. **Baca tulisanmu sendiri.** Setelah menulis, layar menampilkan hasil dari respons penulisan itu, bukan dari read model. Orang lain boleh melihat data lama sebentar; penulisnya tidak.
3. **Jangan pakai read model untuk keputusan.** Tape chart boleh tertinggal. Pemeriksaan "masih ada kamar?" saat membuat booking harus membaca write model di dalam transaction, karena di situlah double booking dicegah.

Cara membicarakannya dengan orang non-teknis: jangan bilang "eventually consistent". Sebut angkanya. "Tape chart bisa tertinggal sampai dua detik dari booking terakhir, dan kami memasang alert kalau lebih dari tiga puluh detik."

## The Exercise

Kode ada di `labs/12-read-models/`. Nyalakan Postgres dulu dengan `make up` di root kursus.

| File | Isi | Diubah? |
|---|---|---|
| `events.go` | `Envelope`, `BookingCreated`, `BookingCancelled` | Tidak, baca dulu |
| `schema.go` | Dua read model, `room_types`, `processed_events` | Tidak, baca dulu |
| `run.go` | Consumer: channel ke projector | Tidak |
| `projector.go` | `Projector.Handle` | Ya, Step 1–3 |
| `api.go` | `GET /availability` | Ya, Step 4 |
| `lab_test.go` | Skenario; tiap test membuat schema Postgres sendiri | Tidak |

Tanggal di event berupa string `YYYY-MM-DD`, dan `CheckOut` bukan malam menginap. Booking 1 Des sampai 3 Des menempati malam 1 dan malam 2.

### Step 0: The booking list

```bash
cd labs/12-read-models
go test -run Step0 -v
```

`PASS`. Read model pertama sudah jadi. Baca `Handle` di `projector.go`: `BookingCreated` menjadi satu baris `booking_list_view` lewat upsert. Test lalu menanyakan "siapa yang menginap malam 2 Des" dengan satu `SELECT` tanpa `JOIN`.

### Step 1: The availability view

```bash
go test -run Step1 -v
```

Test gagal dengan `rooms_sold = 0, seharusnya 2`. Tabel `availability_view` masih kosong.

**Tugas:** saat `BookingCreated` datang, tambahkan `Rooms` ke `rooms_sold` untuk setiap malam menginap. Baris yang belum ada harus dibuat.

> [!hint]- Hint
> Satu statement cukup. `generate_series($2::date, $3::date - 1, interval '1 day')` menghasilkan satu baris per malam, dan `INSERT ... SELECT ... ON CONFLICT (room_type, day) DO UPDATE SET rooms_sold = availability_view.rooms_sold + EXCLUDED.rooms_sold` menambah atau membuat barisnya. Di dalam `SELECT`, beri cast pada parameter: `$1::text` dan `$4::int`. Tanpa cast, Postgres menganggap parameter itu teks dan menolak memasukkannya ke kolom `int`.

### Step 2: Duplicates

```bash
go test -run Step2 -v
```

Event `e-1` dikirim tiga kali. Tebak dulu angka `rooms_sold` untuk 1 Des.

Test gagal, misalnya dengan `rooms_sold = 6, seharusnya 2` untuk 1 Des. Perhatikan bahwa `booking_list_view` tidak ikut rusak. Kenapa?

**Tugas:** bungkus `Handle` dalam satu transaction. Langkah pertamanya mencatat `env.EventID` di `processed_events`. Kalau ID itu sudah ada, jangan ubah apa pun.

> [!hint]- Hint
> `tag, err := tx.Exec(ctx, "INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT (event_id) DO NOTHING", env.EventID)`, lalu `if tag.RowsAffected() == 0 { return nil }`. Semua `Exec` berikutnya memakai `tx`, bukan `p.pool`. Kalau sebagian masih lewat pool, tanda dan efeknya bisa tersimpan terpisah.

### Step 3: Cancellation

```bash
go test -run Step3 -v
```

`BookingCancelled` hanya membawa `BookingID`. Projector harus mengubah status di `booking_list_view` menjadi `cancelled` dan mengurangi `rooms_sold` untuk malam-malam booking itu.

Test ini punya tiga bagian. Pembatalan pertama harus melepas kamar. Redelivery event yang sama tidak boleh melepas lagi; `processed_events` dari Step 2 menangani ini. Bagian ketiga mengirim pembatalan yang sama dengan **event ID baru**, seperti pengirim yang melakukan retry. `processed_events` tidak mengenalinya.

**Tugas:** lepas kamar hanya kalau status booking berubah dari `confirmed` ke `cancelled` pada saat itu.

> [!hint]- Hint
> `UPDATE booking_list_view SET status = 'cancelled' WHERE booking_id = $1 AND status = 'confirmed' RETURNING room_type, check_in, check_out, rooms`. Kalau booking sudah `cancelled`, `UPDATE` tidak menemukan baris dan tidak mengembalikan apa pun, jadi tidak ada yang dikurangi. Hasil `RETURNING` bisa di-scan lalu dipakai di `UPDATE` kedua, atau kedua `UPDATE` digabung dalam satu statement dengan `WITH cancelled AS (...)`.

Perhatikan dari mana projector mengetahui tanggal dan jumlah kamar: dari `booking_list_view`, read model yang ia bangun sendiri. Event pembatalan tidak membawanya.

### Step 4: Serving the view over HTTP

```bash
go test -run Step4 -v
```

Test gagal dengan `GET /availability = 501, seharusnya 200`.

**Tugas:** implementasikan handler di `api.go` sesuai kontrak di komentar `NewAPI`. Handler hanya membaca `availability_view` dan `room_types`. Malam tanpa baris harus tetap muncul dengan `rooms_sold` 0.

> [!hint]- Hint
> Validasi dulu: `time.Parse(time.DateOnly, ...)` untuk kedua tanggal, selisih malam antara 1 dan 366. Lalu satu query: `room_types` di-`CROSS JOIN` dengan `generate_series` tanggal, `LEFT JOIN` ke `availability_view`, dan `COALESCE(v.rooms_sold, 0)`. Kalau hasilnya nol baris, room type-nya tidak ada: jawab 404.

Setelah lulus, baca dua baris log test:

```
GET pertama setelah event dikirim masih melihat data lama: [...]
read model menyusul setelah N kali GET
```

Test sengaja memperlambat consumer 20 ms per event. Pada GET pertama, semua event sudah dikirim, tetapi API masih menjawab dengan keadaan sebelumnya. Jawaban itu valid dan berstatus 200. Itu eventual consistency yang terlihat langsung.

## Solution

> [!success]- Solution
> Kerangka `Handle`: satu transaction, dengan pencatatan event ID sebagai langkah pertama.
>
> ```go
> tag, err := tx.Exec(ctx,
> 	`INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT (event_id) DO NOTHING`,
> 	env.EventID)
> if err != nil {
> 	return fmt.Errorf("catat event %s: %w", env.EventID, err)
> }
> if tag.RowsAffected() == 0 {
> 	return nil // duplikat
> }
> ```
>
> Pembatalan dijaga oleh perubahan status, bukan oleh event ID:
>
> ```sql
> WITH cancelled AS (
> 	UPDATE booking_list_view SET status = 'cancelled'
> 	WHERE booking_id = $1 AND status = 'confirmed'
> 	RETURNING room_type, check_in, check_out, rooms
> )
> UPDATE availability_view v
> SET rooms_sold = v.rooms_sold - c.rooms
> FROM cancelled c
> WHERE v.room_type = c.room_type AND v.day >= c.check_in AND v.day < c.check_out
> ```
>
> Jawaban pertanyaan Step 2: `booking_list_view` diisi dengan upsert, jadi tiga kali memproyeksikan `B-1` tetap menghasilkan satu baris yang sama.
>
> Kode lengkap: `labs/12-read-models/_solusi/`.

## When This Is the Wrong Choice

Read model yang dibangun dari event menambah satu proses, satu sumber jeda, dan satu hal yang bisa tertinggal diam-diam. Sebelum memakainya, coba dulu yang lebih murah.

Index yang tepat sering sudah cukup. Kalau query lambat karena full scan, itu masalah index, bukan masalah bentuk.

Materialized view di Postgres memberi tabel hasil query yang disegarkan berkala, tanpa event dan tanpa projector. Untuk laporan yang boleh tertinggal lima menit, itu jauh lebih sederhana.

Read replica menjawab masalah beban baca tanpa mengubah bentuk data sama sekali.

Read model dari event adalah pilihan yang salah kalau pembacanya butuh jawaban yang pasti terbaru untuk mengambil keputusan, seperti pemeriksaan stok saat checkout. Ia juga salah kalau sistemnya belum punya event yang bisa dipercaya: projector yang kehilangan satu event menghasilkan angka yang salah selamanya, sampai read model dibangun ulang.

Ia sepadan ketika bentuk baca dan bentuk tulis memang jauh berbeda, ketika pembacanya ada di service lain yang tidak boleh menyentuh database penulis, atau ketika satu data harus disajikan dalam beberapa bentuk sekaligus.

## What Is Still Broken

1. **Urutan event diandaikan benar.** Kalau `BookingCancelled` tiba sebelum `BookingCreated`, pembatalan tidak menemukan baris, lalu `BookingCreated` datang dan booking itu tercatat `confirmed` selamanya. Itu [[13 Lab - Message Ordering]].
2. **`BookingCreated` yang dikirim ulang dengan event ID baru masih terhitung dua kali.** Step 3 menutup lubang ini untuk pembatalan dengan penjaga status. Penjaga serupa untuk pembuatan (tambah `rooms_sold` hanya kalau baris `booking_list_view` benar-benar baru) kamu kerjakan sendiri sebagai latihan tambahan.
3. **Tidak ada cara membangun ulang.** Kalau projector punya bug dan angka sudah salah, satu-satunya obat adalah memutar ulang semua event. Itu butuh tempat penyimpanan event ([[14 Lab - Data Lake]]) dan prosedur pindah tanpa downtime ([[16 Lab - Migrating Read Models]]).
4. **Tidak ada yang mengukur jedanya.** Read model yang tertinggal satu jam menjawab 200 dengan percaya diri. Metrik lag dan alert-nya ada di [[17 Lab - Metrics and Alerting]].
5. **`processed_events` tumbuh selamanya**, sama seperti baris `SENT` di outbox.

## Reflection Questions

1. Kenapa `booking_list_view` aman terhadap duplikat tanpa `processed_events`, sedangkan `availability_view` tidak? Jelaskan dengan istilah "fakta" dan "selisih".
2. Di Step 2, apa yang bisa terjadi kalau `INSERT INTO processed_events` berhasil di-commit tetapi perubahan `availability_view` gagal, karena keduanya tidak berada di satu transaction?
3. Integrasi channel manager yang kamu pegang mewajibkan availability dikirim ke OTA segera setelah booking mana pun di-commit. Availability itu sebaiknya dihitung dari write model atau dari read model seperti `availability_view`? Apa risikonya kalau dari read model yang sedang tertinggal?
4. Resepsionis melapor: "Saya baru input booking, tapi tape chart belum berubah." Tulis dua kalimat penjelasan untuknya tanpa istilah teknis, dan satu perubahan di layar yang membuat laporan seperti itu berhenti datang.
5. Sebutkan satu layar atau laporan di aplikasi yang kamu pelihara sekarang yang lambat karena menghitung ulang dari tabel transaksi setiap kali dibuka. Seperti apa bentuk read model-nya, dan event apa yang mengisinya?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[CQRS]]
- [[Materialised Views]]
- [[Defensible Eventual Consistency]]
- [[Consistency Models]]
- [[Idempotent Consumers]]
