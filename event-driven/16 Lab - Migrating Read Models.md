# 16 Lab - Migrating Read Models

[[15 Lab - Versioned Events]] · [[00 Start Here]] · [[17 Lab - Metrics and Alerting]]

---

**Goal:** mengganti bentuk sebuah read model tanpa mematikan sistem: membangun versi baru dari sejarah event, menjaganya tetap sinkron dengan event yang terus datang, membuktikan hasilnya cocok, lalu memindahkan pembacaan.
**Covers:** read model migration, replay dari data lake, idempotent replay, dual write selama migrasi, compare, cutover.
**Prerequisites:** [[12 Lab - Read Models]] (read model dibangun dari event), [[14 Lab - Data Lake]] (tabel `events`), [[09 Lab - At-Least-Once Delivery]] (idempotent handler).
**Infrastructure:** Postgres.
**Time:** 2–3 jam.

---

## Why This Exists

Read model availability menyimpan satu angka per tipe kamar dan tanggal: berapa kamar terjual. Bentuk itu cukup selama setahun. Lalu revenue manager meminta angka yang sama dipecah per rate plan, karena kamar nonrefundable dan flexible dijual dengan strategi berbeda.

Refleks seorang developer SQL adalah `ALTER TABLE ... ADD COLUMN rate_plan`, lalu backfill. Coba tulis backfill itu. Baris `deluxe, 24 Desember, 5 kamar` harus dipecah menjadi beberapa baris per rate plan. Berapa yang flexible, berapa yang nonrefundable?

Tabel itu tidak tahu. Ia tidak pernah menyimpan rate plan, karena dulu tidak ada yang bertanya. Informasinya dibuang pada saat event diringkas menjadi satu angka. Peringkasan itu memang tujuan sebuah read model, dan sekaligus batasnya: read model hanya bisa menjawab pertanyaan yang sudah diketahui saat ia dibuat.

Informasi itu masih ada di satu tempat. Setiap `BookingCreated` membawa `rate_plan` di payload, dan data lake menyimpan payload itu mentah. Jadi read model baru tidak diturunkan dari read model lama. Ia dibangun dari sumber yang sama dengan read model lama, yaitu event.

## Concepts

### Read model boleh dibuang

Source of truth sistem ini adalah tabel booking milik PMS dan event yang ia terbitkan. Read model hanyalah salah satu cara memandang event tersebut. Kalau cara pandang itu tidak cocok lagi, buat yang baru dari event yang sama dan buang yang lama.

Cara paling mudah memahaminya: read model adalah rekap bulanan yang dibuat dari buku kas. Kalau bos meminta rekap per kategori padahal rekap lama hanya per bulan, kamu tidak bisa memecah angka rekap lama. Kamu membuka buku kas lagi dan menghitung ulang dengan kolom yang berbeda. Perbedaan pentingnya: buku kas tidak bertambah selagi kamu menghitung, sedangkan hotel terus menerima booking selama replay berjalan.

### Empat langkah migrasi

```
langkah 1  dual write   event live ──► v1 dan v2 (satu transaction)
langkah 2  replay       data lake  ──► v2 (seluruh sejarah, idempotent)
langkah 3  compare      v1 vs v2 pada butir yang sama ──► harus tanpa selisih
langkah 4  switch       aplikasi membaca v2; v1 dihapus belakangan
```

Aplikasi tetap membaca v1 sampai langkah 4. Sebelum itu, v2 boleh salah, boleh setengah terisi, dan boleh dikosongkan lalu dibangun ulang. Tidak ada pengguna yang melihatnya. Sifat inilah yang membuat pola ini aman: setiap langkah bisa diulang, dan satu-satunya langkah yang terlihat pengguna adalah mengganti satu angka versi.

### Kenapa replay harus idempotent

Replay sejarah satu tahun bisa berjalan berjam-jam. Ia akan terhenti di tengah oleh deploy, koneksi putus, atau laptop yang ditutup. Kalau handler v2 menambah counter tanpa mengingat event mana yang sudah dihitung, menjalankan ulang replay menggandakan semua angka yang sudah masuk.

Obatnya sama dengan lab 09, dengan database sebagai ingatan. Tabel `applied_v2` mencatat ID setiap event yang sudah diterapkan. Pencatatan dan perubahan angka berada dalam satu transaction, sehingga keduanya tersimpan bersama atau tidak sama sekali.

### Event live selama replay

Selama replay berjalan, booking baru terus masuk. Ada dua cara event live bisa salah terhitung di v2.

**Terlewat.** Event datang setelah replay selesai membaca data lake, dan handler live hanya menulis ke v1. Tidak ada yang pernah menerapkannya ke v2.

**Terhitung dua kali.** Handler live menulis ke v2, lalu replay menemukan event yang sama di data lake dan menerapkannya lagi.

Dual write menutup yang pertama, dan `applied_v2` menutup yang kedua. Jalur live dan jalur replay memakai fungsi yang sama dan tabel pencatat yang sama. Siapa pun yang lebih dulu sampai menerapkan event itu, dan yang datang belakangan melewatinya.

```
                     ┌──► ApplyV1 ──► availability_v1
event live ──► tx ───┤
                     └──► ApplyV2 ──┐
                                    ├──► applied_v2 (event_id) ──► availability_v2
data lake ──► replay ──► ApplyV2 ───┘
```

### Compare sebelum switch

Kepercayaan pada v2 tidak datang dari membaca kodenya. Ia datang dari membandingkan hasilnya dengan sesuatu yang sudah dipercaya, yaitu v1.

v1 dan v2 tidak berbentuk sama, tetapi v2 bisa diringkas ke bentuk v1: jumlahkan semua rate plan per tipe kamar dan tanggal. Setiap baris harus sama persis. Membandingkan total keseluruhan saja tidak cukup, karena dua kesalahan yang berlawanan saling menutupi di total.

## The Exercise

Kode ada di `labs/16-migrating-read-models/`. Jalankan `make up` di root kursus kalau Postgres belum hidup.

| File | Isi | Diubah? |
|---|---|---|
| `schema.go` | Tabel, `Envelope`, `Decode`, `ApplyV1`, `ResetV2`, fungsi baca | Tidak |
| `migrate.go` | `ApplyV2`, `Rebuild`, `ApplyLive`, `Compare`, `SwitchToV2` | Ya |
| `migrate_test.go` | Lima test dan helper | Tidak |

Setiap test mulai dari keadaan yang sama, dibuat oleh `seedHistory`: lima event sudah ada di data lake dan sudah diterapkan ke v1. Baca fungsi itu dulu. Hitung sendiri di kertas isi v1 dan isi v2 yang benar, lalu cocokkan dengan `historyV1` dan `historyV2` di file test.

### Step 0: Sebelum migrasi

```bash
cd labs/16-migrating-read-models
go test -run Step0 -v
```

Test ini lulus. v1 melayani pembacaan, dan event live memperbaruinya lewat `ApplyLive`. Baca `ApplyV1` di `schema.go`: polanya akan kamu pakai lagi di Step 2.

### Step 1: Jalan pintas yang tidak ada

```bash
go test -run Step1 -v
```

Baca `Rebuild` di `migrate.go`. Ia menyalin angka dari v1 dan mengisi rate plan dengan `"standard"`. Tebak dulu isi v2 setelahnya.

Test gagal:

```
read model v2:
   isi: map[{deluxe standard 2026-12-24}:5 {suite standard 2026-12-25}:1]
 harus: map[{deluxe flexible 2026-12-24}:2 {deluxe nonrefundable 2026-12-24}:3 {suite flexible 2026-12-25}:1]
```

Angka 5 tidak bisa dipecah menjadi 2 dan 3 dari tabel v1. Rate plan `"standard"` adalah karangan.

**Tugas:** tulis ulang `Rebuild` supaya membaca semua event dari tabel `events`, urut dari yang paling lama, dan menerapkan masing-masing dengan `ApplyV2`.

> [!hint]- Hint
> `SELECT event_id, name, occurred_at, payload::text FROM events ORDER BY occurred_at, event_id`. Kumpulkan hasilnya ke slice dulu, baru terapkan satu per satu. `pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return ApplyV2(ctx, tx, ev) })` membungkus satu event dalam satu transaction. Mengumpulkan dulu membuat koneksi pembaca tidak tertahan selama replay berjalan.

### Step 2: Rebuild dijalankan dua kali

```bash
go test -run Step2 -v
```

Test menjalankan `Rebuild` dua kali, seperti operator yang mengulang proses yang terhenti di tengah.

Test gagal dengan setiap angka menjadi dua kali lipat: `{deluxe flexible 2026-12-24}:4` dan seterusnya.

**Tugas:** buat `ApplyV2` idempotent.

> [!hint]- Hint
> Tiru `ApplyV1`. `INSERT INTO applied_v2 (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, lalu periksa `tag.RowsAffected()`. Kalau 0, event sudah pernah diterapkan dan fungsi selesai. Karena `Rebuild` sudah membungkus tiap event dalam transaction, pencatatan dan perubahan angka otomatis atomik.

### Step 3: Booking tidak berhenti selama migrasi

```bash
go test -run Step3 -v
```

Urutan kejadian di test: satu booking baru masuk, lalu `Rebuild` berjalan, lalu satu pembatalan masuk.

Test gagal. v2 masih mencatat `{deluxe flexible 2026-12-24}:2`, padahal booking itu sudah dibatalkan. Pembatalan datang setelah replay selesai, dan `ApplyLive` hanya menulis ke v1.

**Tugas:** buat `ApplyLive` menerapkan event ke v1 dan v2 dalam satu transaction.

Sebelum memperbaiki, jawab ini: booking baru yang masuk sebelum `Rebuild` akan diterapkan `ApplyLive` ke v2, lalu ditemukan lagi oleh replay di data lake. Kenapa ia tidak terhitung dua kali?

> [!hint]- Hint
> Di dalam `pgx.BeginFunc` yang sudah ada, panggil `ApplyV1` lalu `ApplyV2` dengan `tx` yang sama. Kalau salah satu gagal, transaction dibatalkan dan tidak ada yang berubah di keduanya.

### Step 4: Buktikan, baru pindah

```bash
go test -run Step4 -v
```

Test merusak v2 dengan sengaja: kamar suite dipindah ke tanggal lain. Total keseluruhan tidak berubah.

Test gagal: `Compare pada v2 yang rusak: isi: []`. `Compare` hanya membandingkan total, tidak melihat apa pun, dan `SwitchToV2` akan memindahkan pengguna ke data yang salah.

**Tugas:** tulis ulang `Compare` supaya membandingkan per tipe kamar dan tanggal. Kembalikan satu `Diff` untuk setiap pasangan yang angkanya berbeda, urut tipe kamar lalu tanggal. Baris yang hanya ada di salah satu sisi juga selisih, dengan sisi yang kosong dihitung 0.

> [!hint]- Hint
> Ringkas v2 dulu dengan `GROUP BY room_type, stay_date`, lalu `FULL OUTER JOIN` ke v1. `coalesce(..., 0)` mengisi sisi yang tidak punya baris. `sum()` atas kolom `int` menghasilkan `bigint`; `::int` menyamakan tipenya dengan kolom di v1. `SwitchToV2` sudah memanggil `Compare` dan tidak perlu diubah.

Bagian kedua test menunjukkan cara memperbaiki read model yang salah: `ResetV2`, lalu `Rebuild` lagi. Baris yang rusak tidak ditambal satu per satu.

```bash
go test -v
```

## Solution

> [!success]- Solution
> `ApplyV2` yang idempotent dimulai dengan mencatat event:
>
> ```go
> tag, err := db.Exec(ctx, `INSERT INTO applied_v2 (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, ev.ID)
> if err != nil {
> 	return fmt.Errorf("v2: mencatat event %s: %w", ev.ID, err)
> }
> if tag.RowsAffected() == 0 {
> 	return nil // sudah diterapkan, oleh replay sebelumnya atau oleh jalur live
> }
> ```
>
> `ApplyLive` menulis ke dua read model dalam satu transaction:
>
> ```go
> return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
> 	if err := ApplyV1(ctx, tx, ev); err != nil {
> 		return err
> 	}
> 	return ApplyV2(ctx, tx, ev)
> })
> ```
>
> `Compare` membandingkan pada butir terhalus yang dimiliki keduanya:
>
> ```sql
> SELECT coalesce(a.room_type, b.room_type), coalesce(a.stay_date, b.stay_date),
>        coalesce(a.rooms_sold, 0), coalesce(b.rooms_sold, 0)
>   FROM availability_v1 a
>   FULL OUTER JOIN (
>         SELECT room_type, stay_date, sum(rooms_sold)::int AS rooms_sold
>           FROM availability_v2 GROUP BY room_type, stay_date
>        ) b ON a.room_type = b.room_type AND a.stay_date = b.stay_date
>  WHERE coalesce(a.rooms_sold, 0) <> coalesce(b.rooms_sold, 0)
>  ORDER BY 1, 2
> ```
>
> Kode lengkap, termasuk `Rebuild`, ada di `_solusi/migrate.go`.

## When This Is the Wrong Choice

**Kalau data baru bisa diturunkan dari read model lama.** Menambah kolom `rooms_available = capacity - rooms_sold` tidak butuh replay. `ALTER TABLE` dan satu `UPDATE` selesai dalam hitungan menit. Replay hanya untuk informasi yang memang tidak ada di tabel lama.

**Kalau sejarahnya tidak lengkap.** Replay hanya sebaik data lake. Kalau data lake baru dipasang enam bulan lalu, read model baru hanya tahu enam bulan. Kalau payload lama tidak membawa field yang dibutuhkan, tidak ada replay yang bisa mengarangnya. Dalam dua kasus itu sumbernya harus tabel milik PMS sendiri, lewat backfill biasa.

**Kalau read model kecil dan downtime boleh.** Untuk tabel yang bisa dibangun ulang dalam sepuluh detik pada jam sepi, kosongkan dan isi ulang. Dual write, compare, dan switch adalah harga untuk tidak pernah berhenti. Jangan bayar harga itu kalau berhenti sebentar tidak merugikan siapa pun.

**Dual write hanya aman di sini karena satu database.** v1 dan v2 berada di Postgres yang sama, jadi satu transaction mencakup keduanya. Kalau v2 pindah ke Elasticsearch atau database lain, tidak ada transaction bersama, dan menulis ke dua tempat kembali menjadi masalah dual write dari lab 10. Pada kasus itu v2 diberi consumer sendiri dengan consumer group sendiri.

## What Is Still Broken

1. **Replay dan live boleh menerapkan event dalam urutan berbeda.** Itu aman di lab ini karena penjumlahan tidak peduli urutan. Read model yang menyimpan status terakhir akan salah kalau event lama dari replay menimpa event baru dari jalur live. Pertahanannya adalah revision per entity dari [[13 Lab - Message Ordering]].
2. **`Rebuild` memuat seluruh sejarah ke memory.** Kode solusi menandainya dengan komentar `shortcut:`. Data lake yang besar harus dibaca per halaman dengan keyset pagination, dan progresnya dipantau.
3. **Compare dan switch tidak atomik.** Event live bisa masuk di antara keduanya. Karena dual write menjaga v1 dan v2 bergerak bersama, celah itu tidak berbahaya di sini. Pada sistem tanpa dual write yang transactional, compare harus menoleransi selisih sesaat atau menghentikan penulisan sejenak.
4. **v1 belum dihapus dan dual write belum dicabut.** Setelah switch, v1 sebaiknya dibiarkan beberapa hari sebagai jalan pulang, lalu dihapus bersama kode dual write-nya. Migrasi yang tidak pernah dibereskan meninggalkan dua read model untuk dipelihara selamanya.
5. **Replay yang berjalan lama tidak terlihat.** Tidak ada metrik progres atau alert kalau ia berhenti. Lihat [[17 Lab - Metrics and Alerting]].

## Reflection Questions

1. Jelaskan kepada rekan yang terbiasa dengan `ALTER TABLE` kenapa kolom `rate_plan` tidak bisa di-backfill dari `availability_v1`.
2. Di Step 3, satu event diterapkan ke v2 oleh `ApplyLive`, lalu ditemukan lagi oleh `Rebuild`. Telusuri baris demi baris apa yang terjadi di `ApplyV2` pada kedatangan kedua.
3. `Compare` versi awal hanya membandingkan total keseluruhan. Buat satu contoh kesalahan selain yang ada di test yang lolos dari pengecekan itu.
4. Urutan di lab ini: dual write dinyalakan dulu, baru replay. Apa yang salah kalau urutannya dibalik dan `applied_v2` tidak ada?
5. Pilih satu laporan atau halaman rekap di sistem yang kamu pegang yang angkanya dihitung dari tabel transaksi. Kalau besok bentuk rekap itu harus berubah, dari mana kamu membangun ulangnya? Apa padanan "data lake" di sistem itu, dan apakah lengkap?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Backfilling Large Datasets Safely]]
- [[Expand-Contract Schema Migration]]
- [[CQRS]]
- [[Zero-Downtime Database Migrations]]
- [[Zero-Downtime Database Migration Using CDC]]
- [[Materialised Views]]
