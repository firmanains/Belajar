# 14 Lab - Data Lake

[[13 Lab - Message Ordering]] · [[00 Start Here]] · [[15 Lab - Versioned Events]]

---

**Goal:** membangun tempat penyimpanan semua event yang append-only di Postgres, mengisinya dari forwarder yang tahan duplikat, dan membacanya kembali berdasarkan nama event dan rentang waktu.
**Covers:** data lake, retention broker, forwarder, deduplication pada event ID, append-only, `occurred_at` vs `recorded_at`.
**Prerequisites:** [[03 Lab - Events]] (envelope dan event ID), [[09 Lab - At-Least-Once Delivery]] (kenapa event bisa datang dua kali), [[12 Lab - Read Models]] (apa itu read model).
**Infrastructure:** Postgres.
**Time:** 2 jam.

---

## Why This Exists

Tiga bulan setelah PMS berjalan, manajer hotel bertanya: berapa persen booking yang dibatalkan dalam 24 jam setelah dibuat, per rate plan?

Read model availability tidak bisa menjawabnya. Ia hanya menyimpan jumlah kamar terjual per tanggal, karena hanya itu yang dulu diminta. Tabel booking menyimpan status terakhir, bukan kapan status itu berubah. Semua informasi yang dibutuhkan pernah lewat sebagai event `BookingCreated` dan `BookingCancelled`. Sekarang event itu sudah tidak ada.

Event itu hilang karena broker bukan arsip. Kafka dengan pengaturan umum menghapus record setelah masa retention topic habis, dan queue biasa menghapus message begitu selesai dikonsumsi. Broker dirancang untuk mengantar, dan menyimpan hanya selama itu membantu pengantaran.

Setiap pertanyaan baru tentang masa lalu berakhir sama: "datanya tidak pernah disimpan". Data lake ada supaya jawaban itu tidak pernah diberikan lagi. Caranya sederhana: simpan semua event, apa adanya, selamanya, di satu tempat.

## Concepts

### Satu consumer yang tidak memilih

Data lake adalah consumer biasa dengan satu keistimewaan: ia berlangganan ke semua topic event dan tidak menafsirkan apa pun.

```
                      ┌─► availability handler ──► read model
PMS ──► broker ───────┼─► email handler
        (retention    │
         terbatas)    └─► lake forwarder ──► tabel events (selamanya)
                                                   │
                              ┌────────────────────┴───────────────┐
                              ▼                                    ▼
                     read model baru (lab 16)            analisis, audit, debugging
```

Pikirkan data lake sebagai kamera CCTV di lobi hotel. Resepsionis mencatat hal yang ia anggap penting hari ini. Kamera merekam semuanya tanpa menilai, sehingga pertanyaan yang baru muncul bulan depan masih bisa dijawab dari rekaman. Analogi ini berhenti bekerja di satu titik: rekaman CCTV ditimpa setelah beberapa minggu, sedangkan data lake tidak pernah menimpa.

Di production, forwarder berlangganan ke topic lewat pola nama dengan consumer group miliknya sendiri. Lab ini memakai channel Go sebagai pengganti broker. Bagian yang dipelajari adalah apa yang dilakukan forwarder terhadap event, dan itu sama di kedua kasus.

### Empat sifat yang membuatnya berguna

**Mentah.** Payload disimpan apa adanya, termasuk field yang tidak dikenal forwarder. Begitu forwarder merapikan payload ke struct yang ia kenal, setiap field baru dari pengirim hilang tanpa pesan error. Data lake yang pilih-pilih hanya menyimpan pertanyaan kemarin.

**Lengkap.** Event dengan nama yang belum pernah dilihat tetap disimpan. Forwarder tidak punya daftar event yang "didukung".

**Tanpa duplikat.** Forwarder menerima event at-least-once seperti consumer lain. Event ID menjadi primary key, dan `INSERT ... ON CONFLICT DO NOTHING` membuat penyimpanan idempotent. Ini idempotent consumer dari lab 09 dalam bentuk paling murah, karena database yang mengingat ID.

**Append-only.** Baris tidak pernah diubah atau dihapus. Kalau sebuah event ternyata salah, perbaikannya adalah event baru yang mengoreksi, sama seperti jurnal akuntansi yang dikoreksi dengan jurnal pembalik.

### Dua waktu

Setiap baris punya `occurred_at` dan `recorded_at`. `occurred_at` adalah waktu kejadian menurut pengirim, dan itu yang dipakai untuk pertanyaan bisnis. `recorded_at` adalah waktu data lake menerima event, dan itu yang dipakai untuk pertanyaan operasional seperti "berapa lama event tertunda di broker?".

Keduanya bisa berbeda jauh. Event yang tertahan di outbox selama satu jam punya `occurred_at` pukul 09.00 dan `recorded_at` pukul 10.00. Laporan "booking kemarin" yang memakai `recorded_at` akan salah setiap kali ada gangguan.

### Yang bukan data lake

Data lake bukan source of truth untuk penulisan. PMS memutuskan boleh tidaknya sebuah booking dari tabel booking miliknya, bukan dari data lake. Data lake selalu sedikit tertinggal dan tidak ikut dalam transaction siapa pun.

Data lake juga bukan event sourcing. Pada event sourcing, service menyimpan event sebagai satu-satunya bentuk state dan membangun ulang entity dari event itu sebelum mengambil keputusan. Di sini service tetap menyimpan state seperti biasa, dan data lake hanya salinan dari apa yang sudah diterbitkan.

## The Exercise

Kode ada di `labs/14-data-lake/`. Jalankan `make up` di root kursus kalau Postgres belum hidup.

| File | Isi | Diubah? |
|---|---|---|
| `event.go` | `Envelope` dan dua bentuk payload | Tidak |
| `lake.go` | `Migrate`, `Append`, `Forward`, `Query` | Ya |
| `lake_test.go` | Lima test dan helper Postgres | Tidak |

Setiap test membuat schema Postgres sendiri dengan nama acak dan menghapusnya di akhir. Kamu bisa menjalankan test berulang kali tanpa membersihkan apa pun.

### Step 0: Simpan dan baca

```bash
cd labs/14-data-lake
go test -run Step0 -v
```

Test ini lulus. Satu event disimpan, lalu dibaca kembali dengan payload yang sama. Baca `Migrate` dan perhatikan apa yang tidak ada di definisi tabelnya.

### Step 1: Event yang sama datang dua kali

```bash
go test -run Step1 -v
```

Tebak dulu: `Append` dipanggil dua kali dengan event yang sama. Error, atau dua baris?

Test gagal: `tabel events berisi 2 baris untuk satu event, seharusnya 1`. Setiap laporan di atas tabel ini akan menghitung booking itu dua kali, dan tidak ada yang tahu sampai angkanya dibandingkan dengan sumber lain.

**Tugas:** buat `Append` idempotent. Panggilan kedua harus berhasil tanpa error dan tanpa menambah baris.

> [!hint]- Hint
> Dua perubahan. Di `Migrate`, jadikan `event_id` primary key. Di `Append`, tambahkan `ON CONFLICT (event_id) DO NOTHING`. Tanpa yang kedua, panggilan kedua gagal dengan unique violation dan forwarder berhenti di event yang sebenarnya sudah tersimpan.

### Step 2: Forwarder yang terlalu pintar

```bash
go test -run Step2 -v
```

Tiga event dikirim lewat `Forward`: `BookingCreated` dengan field baru `rate_plan`, `PaymentCaptured` yang belum ada saat forwarder ditulis, dan `BookingCancelled` dengan field baru `reason`.

Test gagal: `tabel events berisi 2 baris, seharusnya 3`. Setelah kamu memperbaiki itu, test gagal lagi di tempat lain: `payload evt-1 berubah di data lake`, dengan `rate_plan` yang hilang dari versi tersimpan.

Baca `Forward`. Ia melakukan decode payload ke struct yang dikenalnya, lalu encode ulang, dan melewati nama yang tidak dikenal. Niatnya validasi. Akibatnya data lake kehilangan semua hal yang baru.

**Tugas:** buat `Forward` menyimpan setiap event apa adanya.

> [!hint]- Hint
> Jawabannya menghapus kode, bukan menambah. `Envelope.Payload` sudah bertipe `json.RawMessage`. Yang tersisa di dalam loop hanya pemanggilan `Append`.

### Step 3: Rentang waktu

```bash
go test -run Step3 -v
```

Test gagal: `Query(BookingCreated, 10:00, 15:00) = [evt-5 evt-2 evt-4 evt-1], seharusnya [evt-2 evt-4]`. `Query` mengabaikan `from` dan `to`, dan urutannya mengikuti urutan penyimpanan.

**Tugas:** saring pada rentang `[from, to)` dan urutkan dari yang paling lama. `from` termasuk, `to` tidak.

Rentang setengah terbuka dipilih supaya dua query yang bersebelahan tidak tumpang-tindih. `[10:00, 15:00)` lalu `[15:00, 20:00)` mencakup setiap event tepat sekali. Dengan `BETWEEN`, event pukul 15.00 tepat terhitung di keduanya.

> [!hint]- Hint
> `WHERE name = $1 AND occurred_at >= $2 AND occurred_at < $3 ORDER BY occurred_at, event_id`. `event_id` di `ORDER BY` membuat urutan tetap sama untuk dua event dengan waktu yang persis sama. Tambahkan juga index `(name, occurred_at)` di `Migrate`: test tidak memeriksanya, tetapi tabel ini akan menjadi tabel terbesar di sistem.

### Step 4: Sejarah tidak boleh ditulis ulang

```bash
go test -run Step4 -v
```

Test menjalankan `UPDATE` dan `DELETE` langsung ke tabel `events`, dan keduanya berhasil. "Append-only" sejauh ini hanya niat baik.

Test gagal: `UPDATE pada tabel events berhasil, seharusnya ditolak database`.

**Tugas:** buat database menolak `UPDATE` dan `DELETE` pada tabel `events`.

> [!hint]- Hint
> Trigger `BEFORE UPDATE OR DELETE ... FOR EACH ROW` yang menjalankan function PL/pgSQL berisi `RAISE EXCEPTION`. Tambahkan ke `Migrate`. `CREATE OR REPLACE TRIGGER` membuat `Migrate` tetap aman dijalankan berulang.

Lalu jalankan semuanya:

```bash
go test -v
```

## Solution

> [!success]- Solution
> Tiga dari empat perbaikan ada di SQL:
>
> ```sql
> CREATE TABLE IF NOT EXISTS events (
>     event_id    text        PRIMARY KEY,
>     name        text        NOT NULL,
>     occurred_at timestamptz NOT NULL,
>     payload     jsonb       NOT NULL,
>     recorded_at timestamptz NOT NULL DEFAULT now()
> );
> CREATE INDEX IF NOT EXISTS events_name_occurred_at ON events (name, occurred_at);
>
> CREATE OR REPLACE FUNCTION events_append_only() RETURNS trigger AS $$
> BEGIN
>     RAISE EXCEPTION 'tabel events append-only: % ditolak', TG_OP;
> END;
> $$ LANGUAGE plpgsql;
>
> CREATE OR REPLACE TRIGGER events_append_only
>     BEFORE UPDATE OR DELETE ON events
>     FOR EACH ROW EXECUTE FUNCTION events_append_only();
> ```
>
> ```sql
> INSERT INTO events (event_id, name, occurred_at, payload) VALUES ($1, $2, $3, $4::jsonb)
> ON CONFLICT (event_id) DO NOTHING
> ```
>
> Perbaikan keempat adalah `Forward` yang menjadi lebih pendek:
>
> ```go
> case e, ok := <-in:
> 	if !ok {
> 		return nil
> 	}
> 	if err := l.Append(ctx, e); err != nil {
> 		return err
> 	}
> ```
>
> Kode lengkap ada di `_solusi/lake.go`.

## When This Is the Wrong Choice

**Kalau tidak ada yang akan membacanya.** Data lake punya biaya nyata: storage yang hanya tumbuh, satu consumer lagi untuk dipantau, dan kewajiban menjaga data pribadi di dalamnya. Untuk sistem kecil dengan satu read model yang bentuknya stabil, backup database biasa sudah cukup.

**Kalau event berisi data yang wajib bisa dihapus.** Append-only bertabrakan dengan kewajiban menghapus data pribadi atas permintaan pemiliknya. Jalan keluar yang umum adalah menyimpan data pribadi di luar event dan hanya menaruh referensinya. Itu keputusan desain event yang harus diambil sebelum data lake diisi, bukan sesudahnya.

**Postgres bukan tempat yang tepat untuk volume besar.** Satu tabel Postgres nyaman untuk volume kecil sampai menengah, lalu menjadi beban bagi backup dan vacuum. Sistem dengan volume tinggi menyimpan event sebagai file di object storage dan membacanya dengan query engine terpisah. Lab ini memakai Postgres karena konsepnya sama dan alatnya sudah kamu kenal.

**Sebagai pengganti audit log.** Data lake mencatat event yang diterbitkan service. Ia tidak mencatat siapa yang membaca data atau siapa yang mencoba dan ditolak. Kebutuhan audit seperti itu punya jalurnya sendiri.

## What Is Still Broken

1. **Trigger tidak menghentikan `TRUNCATE` atau `DROP TABLE`.** Trigger baris hanya menangkap `UPDATE` dan `DELETE`. Perlindungan penuh butuh pembatasan privilege: role forwarder hanya mendapat `INSERT` dan `SELECT`.
2. **Tidak ada yang tahu kalau forwarder berhenti.** Kalau `Forward` mati selama seminggu dan retention broker hanya tiga hari, empat hari event hilang selamanya. Memantau jarak antara `occurred_at` dan `recorded_at` adalah bahan [[17 Lab - Metrics and Alerting]].
3. **Payload lama dan baru bercampur.** `BookingCreated` bulan ini punya `rate_plan`, yang bulan lalu tidak. Siapa pun yang membaca data lake harus siap dengan semua bentuk yang pernah ada. Itu topik [[15 Lab - Versioned Events]].
4. **Belum ada yang memakai datanya.** [[16 Lab - Migrating Read Models]] membangun read model baru dari tabel seperti ini.

## Reflection Questions

1. Jelaskan dengan kata-katamu sendiri kenapa broker tidak bisa menggantikan data lake, walau keduanya "menyimpan event".
2. Forwarder di Step 2 melakukan decode lalu encode ulang payload dengan niat validasi. Apa yang hilang, dan kenapa kehilangan itu tidak menghasilkan error?
3. Sebuah event tertahan dua jam di outbox sebelum terkirim. Laporan "jumlah booking per jam" seharusnya memakai `occurred_at` atau `recorded_at`? Pertanyaan apa yang justru lebih tepat dijawab dengan kolom satunya?
4. Outbox billing di kantormu menyimpan baris dengan status seperti `PENDING` dan `DEAD`, dan statusnya diubah saat diproses. Apa bedanya tabel itu dengan tabel `events` di lab ini? Bisakah satu tabel melayani kedua peran?
5. Seseorang mengusulkan agar PMS membaca data lake untuk memutuskan apakah kamar masih tersedia. Apa yang salah dengan usul itu?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Event Sourcing]] (untuk membedakannya dari data lake)
- [[Queue vs Log Semantics]]
- [[Idempotent Consumers]]
- [[Change Data Capture]]
- [[Backfilling Large Datasets Safely]]
