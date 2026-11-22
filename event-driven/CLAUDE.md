# Course Build Spec: Event-Driven Go

Kursus latihan event-driven dengan Go untuk satu pembaca. Teori panjang ada di
`../backend-engineering-vault`; kursus ini berisi lab yang bisa dijalankan.
Semua isi ditulis sendiri.

## Reader

Backend engineer, sekitar satu tahun pengalaman production. Stack harian
PHP (Yii) + MariaDB; ekosistem sekitar: Go, Kafka, Kubernetes. Pernah
mengoperasikan Kafka, dead letter topic, dan outbox bersama AI, tetapi belum
paham konsep di baliknya. Jangan mengandaikan pengetahuan event-driven apa pun.
Go: paham struct, interface, goroutine dasar, `go test`.

## Language

Aturan bahasa dan kalimat mengikuti `../backend-engineering-vault/CLAUDE.md`
bagian 3, 4, dan 4.1. Ringkasnya:

- Penjelasan dalam Bahasa Indonesia. Istilah teknis tetap bahasa Inggris dan
  tidak diterjemahkan: transaction, idempotent, partition, consumer group,
  offset, retry, handler, read model, dead letter queue.
- Nama file, nama folder, heading, nama test, dan identifier kode dalam bahasa
  Inggris. Komentar kode dan pesan gagal test dalam Bahasa Indonesia.
- Masalah dulu, baru mekanisme. Satu ide per kalimat. Analogi wajib untuk konsep
  abstrak, disertai titik di mana analogi itu berhenti berlaku.
- Setiap teknik punya bagian kapan ia pilihan yang salah.
- Jangan mengarang judul paper, URL, angka benchmark, atau perilaku versi.

## Layout

```
00 Start Here.md                     peta kursus, cara belajar, progress
NN Lab - <Title>.md                  satu catatan per lab (01..20)
21 Project - Hotel Channel Sync.md   proyek akhir
labs/NN-slug/                        kode lab, satu Go module per lab
labs/NN-slug/_solusi/                file jawaban, path sama dengan file yang diubah pembaca
proyek/                              kode proyek akhir, konvensi sama dengan lab
docker-compose.yaml, Makefile        Postgres 55432 dan Kafka 59092
verify.sh                            pemeriksa lab
```

Pembaca mengerjakan lab di salinan kerja `~/Developer/playground/event-driven/`, bukan di folder ini. `labs/` dan `proyek/` di sini adalah sumber yang tidak diubah; perubahan pada lab dibuat di sini lalu disalin ulang ke salinan kerja.

## Lab Note Template

Heading persis seperti ini, urutannya tetap:

1. `# NN Lab - Title`, lalu baris navigasi `[[sebelumnya]] · [[00 Start Here]] · [[berikutnya]]`
2. Blok ringkas: **Goal**, **Covers** (topik yang dipetakan), **Prerequisites**, **Infrastructure** (none / Postgres / Kafka), **Time**
3. `## Why This Exists`: masalah nyata yang melahirkan teknik ini
4. `## Concepts`: prosa, diagram ASCII, analogi
5. `## The Exercise`: tabel file (mana yang diubah), lalu `### Step N: ...` per test. Tiap step: command, minta pembaca menebak hasil, pesan gagal yang akan muncul, tugas, lalu petunjuk dalam callout terlipat `> [!hint]- Hint`
6. `## Solution`: callout terlipat `> [!success]- Solution` berisi ide kuncinya dan potongan kode terpenting; kode lengkap ada di `_solusi/`
7. `## When This Is the Wrong Choice`
8. `## What Is Still Broken`: lubang yang sengaja tersisa dan lab mana yang menutupnya
9. `## Reflection Questions`: 3–5 pertanyaan, minimal satu memakai sistem nyata pembaca, lalu tempat kosong `**My answers:**`
10. `## Read Next in the Vault`: nama catatan di `../backend-engineering-vault` yang benar-benar ada (cek dulu dengan `ls`/`find`)

## Code Conventions

- `go.mod`: `module belajar/eventdriven/labNN`, `go 1.26`.
- Kode awal harus bisa di-compile dan lulus `go vet`.
- Test bernama `TestStepN_Description`. `TestStep0_...` lulus di kode awal dan
  menunjukkan dunia ideal. Step lain gagal di kode awal karena assertion,
  bukan karena compile error, panic, atau hang. Setiap tunggu punya timeout.
- Pembaca hanya mengubah file yang disebut di catatan. File test dan file
  pendukung diberi komentar pembuka "File ini sudah jadi. Jangan diubah."
- `_solusi/` berisi versi jadi dari file yang diubah pembaca saja.
- Go idiomatis: `context.Context`, `fmt.Errorf("...: %w", err)`, semua error
  ditangani, tanpa data race (solusi dijalankan dengan `-race`).
- Hanya `testing` dari stdlib; tanpa testify.
- Dependency yang dipakai: `github.com/ThreeDotsLabs/watermill` (dan
  `message/router`, `pubsub/gochannel`, `components/cqrs`),
  `github.com/twmb/franz-go` untuk Kafka, `github.com/jackc/pgx/v5` untuk
  Postgres, `github.com/prometheus/client_golang`, `go.opentelemetry.io/otel`.
  Jangan menambah yang lain tanpa alasan kuat.
- Infrastruktur: `ED_POSTGRES_URL` (default
  `postgres://lab:lab@localhost:55432/lab`) dan `ED_KAFKA_BROKERS` (default
  `localhost:59092`). Kalau tidak terjangkau, test gagal dengan pesan
  "jalankan `make up` di root kursus". Topic dibuat oleh test (auto-create
  dimatikan). Setiap test memakai nama schema/tabel/topic/consumer group yang
  unik (akhiran acak) supaya bisa diulang dan tidak bertabrakan dengan lab lain.
- Lab selesai hanya kalau `./verify.sh labs/NN-slug` mencetak `OK`.

## Domain

Semua lab memakai satu domain: PMS hotel yang menyinkronkan booking dengan
channel manager. Entity: `Booking`, `RoomType`, availability per tanggal,
confirmation email, payment. Event memakai bentuk lampau (`BookingCreated`,
`BookingCancelled`, `PaymentCaptured`); command memakai bentuk perintah
(`SendConfirmationEmail`, `CapturePayment`).

Contoh dari sistem nyata pembaca boleh dipakai hanya sebatas fakta berikut,
tanpa IP, kredensial, atau nama orang:

- Aplikasi di kantormu menaruh email di Kafka; `email-service` (Spring Kafka)
  mengirimnya, punya dead letter topic `emails.DLT` yang dicoba ulang oleh
  cron, dan memakai alamat penerima sebagai key record.
- Cluster Kafka-nya 3 broker.
- Billing di kantormu memakai outbox dengan status seperti `PENDING`
  dan `DEAD`, diproses cron tiap 2 menit.
- Spesifikasi integrasi channel manager menuntut: perubahan ditangkap lewat
  outbox dalam transaction yang sama, revisi booking diterapkan idempotent dan
  berurutan, ack hanya setelah commit database, polling feed sebagai jaring
  pengaman webhook.
