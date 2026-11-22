# 18 Lab - Tracing

[[17 Lab - Metrics and Alerting]] · [[00 Start Here]] · [[19 Lab - Fault Tolerance]]

**Goal:** membuat satu booking terlihat sebagai satu trace, dari HTTP request sampai dua handler yang berjalan di proses berbeda.
**Covers:** tracing: trace, span, trace context, propagation lewat metadata message, span kind, attribute untuk messaging, hubungan dengan correlation ID.
**Prerequisites:** [[05 Lab - Middlewares]] (middleware dan correlation ID), [[08 Lab - Events with CQRS]] (satu event, beberapa handler).
**Infrastructure:** none. Span disimpan di memory oleh exporter untuk test; Jaeger atau Tempo tidak dijalankan.
**Time:** 2–3 jam.

---

## Why This Exists

Tamu menelepon: "Saya booking pukul 14.02, sampai sekarang belum ada email." Booking itu melewati empat tempat:

```
proses API                     broker                 proses email worker
POST /bookings ─► simpan ─► publish BookingCreated ─► handler send_confirmation_email ─► SMTP
                                    │
                                    │                  proses channel worker
                                    └────────────────► handler sync_channel_manager ─► HTTP ke channel manager
```

Metric dari [[17 Lab - Metrics and Alerting]] memberi tahu bahwa handler email gagal 3% hari ini. Ia tidak memberi tahu apa yang terjadi pada booking milik tamu ini.

Log bisa menjawab, kalau kamu mau membuka tiga file log di tiga proses, mencocokkan jam, dan berharap tidak ada booking lain pada detik yang sama. Correlation ID dari [[05 Lab - Middlewares]] memperbaiki pencocokan: semua baris log untuk satu request membawa ID yang sama, jadi satu pencarian mengumpulkan semuanya.

Yang masih hilang adalah bentuk. Daftar baris log tidak menunjukkan siapa memanggil siapa, pekerjaan mana yang berjalan paralel, dan di mana waktu habis. Dari 40 detik antara booking dan email, berapa detik message menunggu di broker dan berapa detik dihabiskan di SMTP? Tracing menjawab pertanyaan itu.

## Concepts

### Trace and span

**Span** adalah satu pekerjaan dengan waktu mulai dan selesai: satu HTTP request, satu publish, satu pemrosesan message, satu panggilan SMTP. Setiap span punya `span ID`, dan mencatat `span ID` milik parent-nya.

**Trace** adalah pohon span yang berbagi satu `trace ID`. Untuk booking di atas:

```
trace 4bf92f35...
│
└─ POST /bookings                       [server]    0 ms ──── 45 ms
   └─ publish pms.booking.created       [producer]  30 ms ─ 38 ms
      ├─ process pms.booking.created    [consumer]        2.100 ms ───────── 32.400 ms   ✗ error
      │  └─ smtp send                                     2.110 ms ───────── 32.400 ms
      └─ process pms.booking.created    [consumer]        2.050 ms ── 2.300 ms
```

Dari gambar ini langsung terbaca: message menunggu sekitar dua detik di broker, handler email menghabiskan 30 detik di SMTP lalu gagal, dan channel sync selesai cepat.

Pikirkan trace sebagai resi paket. Setiap gudang yang dilewati menempelkan stempel berisi jam masuk dan jam keluar, dan semua stempel memuat nomor resi yang sama. Yang tidak ditangkap analogi ini: paket hanya punya satu jalur, sedangkan trace bercabang. Satu event `BookingCreated` diproses dua handler, dan keduanya menjadi child dari span publish yang sama.

### Why context does not cross the broker

Di dalam satu proses Go, span aktif disimpan di `context.Context`. `tracer.Start(ctx, ...)` melihat span di `ctx`, menjadikannya parent, dan mengembalikan `ctx` baru berisi span yang baru.

`context.Context` adalah nilai di memory. Ia tidak ikut ketika message dikirim ke Kafka. Yang menyeberang hanya byte: payload dan header. Jadi identitas trace harus ditulis ke header message di sisi pengirim (**inject**) dan dibaca kembali di sisi penerima (**extract**).

Format header-nya distandarkan oleh W3C Trace Context. Satu header bernama `traceparent`:

```
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
             │  │                                │                │
             │  trace ID (32 hex)                span ID (16 hex) flags (01 = sampled)
             version
```

`span ID` di sini adalah span milik pengirim, yang akan menjadi parent di sisi penerima. Di OpenTelemetry, pekerjaan menulis dan membaca header ini dilakukan oleh sebuah `TextMapPropagator`. Ia bekerja terhadap `TextMapCarrier`, yaitu apa pun yang berbentuk map dari string ke string. `message.Metadata` milik Watermill adalah `map[string]string`, jadi cukup dibungkus dengan `propagation.MapCarrier(msg.Metadata)`.

### Span kind and attributes

Span kind memberi tahu backend tracing peran sebuah span. Untuk messaging ada dua yang penting: `producer` untuk span yang mengirim message, dan `consumer` untuk span yang memprosesnya. Backend memakai pasangan ini untuk menghitung berapa lama message berada di perjalanan.

Attribute adalah pasangan key dan value pada span. OpenTelemetry punya semantic conventions untuk messaging, sehingga nama attribute sama di semua aplikasi. Yang paling berguna:

| Attribute | Isi |
|---|---|
| `messaging.system` | jenis broker, misalnya `kafka` |
| `messaging.destination.name` | nama topic |
| `messaging.message.id` | ID message |
| `messaging.operation.type` | `publish` atau `process` |
| `messaging.consumer.group.name` | consumer group yang memproses |

Berbeda dengan label metric, attribute span boleh memuat nilai ber-cardinality tinggi seperti booking ID. Setiap span disimpan sebagai satu catatan tersendiri, bukan sebagai time series.

### Tracing and the correlation ID

Keduanya menyambung kejadian lintas proses, dan caranya menumpang di metadata sama. Bedanya ada pada apa yang disambung dan apakah ia selalu ada.

Correlation ID adalah satu string yang ditempel di setiap baris log. Ia murah, selalu ada, dan tidak punya struktur. Trace punya struktur (parent, durasi, status), tetapi biasanya di-sampling: pada lalu lintas tinggi hanya sebagian kecil trace yang disimpan. Booking milik tamu yang menelepon bisa saja tidak termasuk.

Praktik yang umum adalah memakai keduanya dan menghubungkannya: tulis `trace ID` di setiap baris log. Dari log kamu bisa melompat ke trace kalau trace-nya tersimpan, dan log tetap ada kalau tidak.

## The Exercise

Kode ada di `labs/18-tracing/`.

| File | Isi | Diubah? |
|---|---|---|
| `tracing.go` | `PublishBookingCreated` dan `Middleware` | Ya |
| `tracing_test.go` | Lima skenario, broker tiruan, exporter di memory | Tidak |

Dua hal tentang test perlu kamu tahu sebelum mulai. Exporter hanya menerima span yang sudah ditutup dengan `End()`, jadi span yang lupa ditutup tidak akan pernah terlihat. Function `deliver` di test meniru penyeberangan ke proses lain: ia membuat message baru yang hanya menyalin UUID, payload, dan metadata.

### Step 0: Tracing must be invisible

```bash
cd labs/18-tracing
go test -run Step0 -v
```

`PASS`. Booking `B-1` dipublikasikan dan sampai ke handler. Instrumentasi yang kamu tambahkan tidak boleh mengubah ini.

### Step 1: A span for the publish

```bash
go test -run Step1 -v
```

Test membuat span `POST /bookings`, memanggil `PublishBookingCreated` dengan `ctx` berisi span itu, lalu mencari span ber-kind `producer`. Belum ada.

**Task:** di `PublishBookingCreated`, mulai span dengan `t.Tracer.Start`, beri kind `producer`, dan beri attribute `messaging.destination.name` serta `messaging.message.id`.

> [!hint]- Hint
> `ctx, span := t.Tracer.Start(ctx, "publish "+BookingCreatedTopic, trace.WithSpanKind(trace.SpanKindProducer), trace.WithAttributes(attribute.String("messaging.destination.name", BookingCreatedTopic), ...))`, lalu `defer span.End()`. Buat message sebelum `Start` supaya `msg.UUID` sudah tersedia untuk attribute. Kalau `Start` dipanggil dengan `context.Background()`, span itu menjadi root di trace baru dan test akan mengatakannya.

### Step 2: Put the trace context in the metadata

```bash
go test -run Step2 -v
```

Tebak dulu: setelah Step 1, apakah `traceparent` sudah ada di metadata? Belum. Span sudah dibuat, tetapi ia hanya hidup di `ctx` milik proses API.

**Task:** inject trace context ke `msg.Metadata` sebelum publish.

Test memeriksa satu detail: `span ID` di `traceparent` harus milik producer span, bukan milik `POST /bookings`.

> [!hint]- Hint
> `t.Propagator.Inject(ctx, propagation.MapCarrier(msg.Metadata))`. `ctx` yang dipakai harus `ctx` yang dikembalikan `Tracer.Start`. Kalau kamu memakai `ctx` yang masuk sebagai parameter, yang tertulis adalah span HTTP, dan span publish akan hilang dari rantai parent di sisi consumer.

### Step 3: Continue the trace in the consumer

```bash
go test -run Step3 -v
```

Message sudah menyeberang. Handler di test membuat span `smtp send` dari `msg.Context()`.

**Task:** di `Middleware`, extract trace context dari metadata, mulai span ber-kind `consumer` dari context hasil extract, lalu pasang context berisi span itu ke message.

Test menuntut tiga hal: consumer span berada di trace yang sama, parent-nya adalah producer span dan bertanda remote, dan `smtp send` menjadi child dari consumer span.

> [!hint]- Hint
> Urutannya: `ctx := t.Propagator.Extract(msg.Context(), propagation.MapCarrier(msg.Metadata))`, lalu `ctx, span := t.Tracer.Start(ctx, "process "+topic, trace.WithSpanKind(trace.SpanKindConsumer), ...)`, `defer span.End()`, lalu `msg.SetContext(ctx)` sebelum `next(msg)`. Tanpa `SetContext`, handler tidak tahu ada span aktif dan `smtp send` menjadi trace baru.

### Step 4: Two handlers, one failure

```bash
go test -run Step4 -v
```

Satu event diproses dua handler. Handler email gagal dengan `SMTP timeout`.

**Task:** beri setiap consumer span attribute `messaging.consumer.group.name` berisi `handlerName`. Kalau handler mengembalikan error, catat error itu pada span dan beri status error. Error tetap harus dikembalikan ke pemanggil.

> [!hint]- Hint
> `span.RecordError(err)` menambah event bernama `exception`, tetapi tidak mengubah status. Status diisi terpisah dengan `span.SetStatus(codes.Error, err.Error())`. Jangan menelan error-nya: middleware di atasmu (retry, nack) masih membutuhkannya.

## Solution

> [!success]- Solution
> Sisi pengirim: mulai span, inject context hasil `Start`, baru publish.
>
> ```go
> ctx, span := t.Tracer.Start(ctx, "publish "+BookingCreatedTopic,
> 	trace.WithSpanKind(trace.SpanKindProducer),
> 	trace.WithAttributes(messagingAttributes(BookingCreatedTopic, msg.UUID, "publish")...),
> )
> defer span.End()
>
> t.Propagator.Inject(ctx, propagation.MapCarrier(msg.Metadata))
> ```
>
> Sisi penerima: extract, mulai span, pasang ke message, jalankan handler, catat error.
>
> ```go
> ctx := t.Propagator.Extract(msg.Context(), propagation.MapCarrier(msg.Metadata))
> ctx, span := t.Tracer.Start(ctx, "process "+topic,
> 	trace.WithSpanKind(trace.SpanKindConsumer),
> 	trace.WithAttributes(attrs...),
> )
> defer span.End()
> msg.SetContext(ctx)
>
> produced, err := next(msg)
> if err != nil {
> 	span.RecordError(err)
> 	span.SetStatus(codes.Error, err.Error())
> }
> return produced, err
> ```
>
> Kode lengkap ada di `_solusi/tracing.go`.

## When This Is the Wrong Choice

Tracing butuh tempat menyimpan dan menampilkan span: collector, storage, dan UI. Untuk satu aplikasi dengan satu worker, correlation ID di log yang terstruktur menjawab hampir semua pertanyaan yang sama dengan biaya operasional jauh lebih kecil.

Trace juga bukan alat untuk alerting. Karena di-sampling, ia tidak bisa dipercaya untuk menghitung. "Berapa persen booking gagal" adalah pertanyaan untuk metric.

Model parent dan child kurang cocok untuk batch. Kalau consumer mengambil 500 message sekaligus dan memprosesnya dalam satu transaction, span pemrosesan itu tidak punya satu parent. OpenTelemetry menyediakan span link untuk hubungan seperti ini: satu span merujuk ke banyak trace tanpa menjadi child salah satunya.

Jangan menaruh data pribadi di attribute. Alamat email tamu di `messaging.message.id` atau di attribute lain akan tersimpan di backend tracing yang biasanya dibuka lebih banyak orang daripada database.

## What Is Still Broken

1. **Message yang dihasilkan handler tidak membawa trace context.** Middleware di lab ini tidak menyentuh `produced`. Kalau handler mempublikasikan event lanjutan, rantainya putus di sana. Perbaikannya: inject context consumer span ke metadata setiap message di `produced`.
2. **Outbox memutus rantai.** Di [[10 Lab - Outbox]], publish dilakukan oleh forwarder beberapa detik kemudian, di proses lain, tanpa `ctx` dari request. Supaya trace tersambung, `traceparent` harus disimpan di baris outbox bersama payload, lalu ditulis ke header saat forwarder mengirim.
3. **Retry membuat span ganda.** Message yang dikirim ulang menghasilkan consumer span baru dengan parent yang sama. Itu benar, tetapi tanpa attribute nomor percobaan, kamu tidak bisa membedakan redelivery dari handler ganda.
4. **Sampling belum dibahas.** Test menyimpan semua span. Di production, keputusan sampling biasanya diambil di awal trace dan ikut dibawa di flags `traceparent`.

## Reflection Questions

1. Kenapa `ctx` dari HTTP request tidak bisa begitu saja dipakai oleh handler di proses lain? Apa yang sebenarnya menyeberang lewat broker?
2. Di Step 2, apa akibatnya di gambar trace kalau yang di-inject adalah `ctx` yang masuk, bukan `ctx` hasil `Tracer.Start`?
3. Aplikasi di kantormu menaruh email di Kafka dan `email-service` mengirimnya. Seorang pengguna melapor email kode pembayaran tidak datang. Dengan tracing terpasang dari aplikasi sampai email service, span apa saja yang kamu harapkan terlihat, dan attribute mana yang kamu pakai untuk menemukan trace milik pengguna itu tanpa menaruh alamat emailnya di span?
4. Outbox billing di kantormu diproses cron tiap 2 menit. Di mana `traceparent` harus disimpan supaya trace tidak putus di antara request pengguna dan panggilan ke sistem billing?
5. Correlation ID dan trace ID sama-sama menumpang di metadata. Sebutkan satu situasi di mana correlation ID ada tetapi trace-nya tidak.

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Distributed Tracing]]
- [[Correlation IDs]]
- [[The Three Pillars of Observability]]
- [[Structured Logging and Log Levels]]
- [[OpenTelemetry]]
