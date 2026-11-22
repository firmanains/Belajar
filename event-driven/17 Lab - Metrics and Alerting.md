# 17 Lab - Metrics and Alerting

[[16 Lab - Migrating Read Models]] · [[00 Start Here]] · [[18 Lab - Tracing]]

**Goal:** membuat consumer yang bisa ditanya "apakah kamu sehat?" lewat angka, lalu menulis alert yang berbunyi ketika tamu hotel dirugikan, bukan ketika sebuah server sibuk.
**Covers:** metrics and alerting: counter per outcome, duration histogram, backlog, umur message tertua, label cardinality, alert rule berbasis gejala.
**Prerequisites:** [[05 Lab - Middlewares]] (middleware membungkus handler), [[06 Lab - Errors]] (handler bisa gagal).
**Infrastructure:** none. Prometheus tidak dijalankan; test membaca registry langsung.
**Time:** 2–3 jam.

---

## Why This Exists

Sistem sinkron mengeluh dengan keras. Kalau endpoint booking rusak, tamu melihat HTTP 500, front office menelepon, dan dalam lima menit semua orang tahu.

Sistem asinkron gagal dengan diam. Booking tersimpan, tamu melihat "Booking berhasil", lalu message `BookingCreated` menunggu di broker. Kalau consumer email mati malam itu, tidak ada yang melihat error apa pun. Tamu baru sadar besok pagi ketika confirmation email tidak pernah datang. Kalau consumer channel sync yang mati, akibatnya lebih mahal: OTA masih menjual kamar yang sudah habis.

Log tidak menolong di sini. Consumer yang mati tidak menulis log. Consumer yang lambat menulis log yang terlihat normal, hanya lebih jarang. Yang dibutuhkan adalah angka yang bisa dibandingkan dengan ambang batas oleh mesin, tanpa ada orang yang harus ingat untuk memeriksa.

Kamu sudah pernah berada di sisi lain masalah ini. Topic `emails.DLT` di `email-service` dicoba ulang oleh cron. Tanpa metric, satu-satunya cara tahu bahwa topic itu sedang menumpuk adalah seseorang membukanya, biasanya setelah ada tiket masuk.

## Concepts

### Four numbers per consumer

Sebuah consumer butuh empat angka. Dua yang pertama menjawab "apa yang terjadi pada message yang diproses". Dua yang terakhir menjawab "apa yang terjadi pada message yang belum diproses", dan dua inilah yang paling sering dilupakan.

| Metric | Tipe | Pertanyaan yang dijawab |
|---|---|---|
| `pms_messages_processed_total{handler, outcome}` | counter | Berapa yang berhasil, berapa yang gagal? |
| `pms_message_processing_duration_seconds{handler}` | histogram | Berapa lama satu message diproses? |
| `pms_queue_backlog_messages{queue}` | gauge | Berapa message yang menunggu? |
| `pms_queue_oldest_message_age_seconds{queue}` | gauge | Sudah berapa lama message tertua menunggu? |

**Counter** hanya bisa naik. Nilai mentahnya jarang berguna; yang dibaca adalah lajunya, misalnya `rate(pms_messages_processed_total[5m])`. **Gauge** bisa naik dan turun, dan nilainya saat ini sudah bermakna. **Histogram** membagi pengamatan ke dalam bucket, sehingga kamu bisa bertanya "99% message selesai dalam berapa detik" tanpa menyimpan setiap pengamatan.

### Why the counter is not enough

Counter `processed` punya titik buta. Laju nol berarti salah satu dari dua hal yang berlawanan: tidak ada booking malam ini, atau consumer mati dan seratus booking sedang menunggu. Dari sisi counter keduanya terlihat sama.

```
                    consumer sehat, sepi          consumer mati
processed rate              0                          0
backlog                     0                         100
oldest message age          0 s                      1.800 s   ◄── yang membedakan
```

Padanan terdekatnya di luar software adalah loket yang menghitung nasabah terlayani. Angka "nol nasabah dalam satu jam terakhir" tidak memberi tahu apakah ruang tunggu kosong atau petugasnya pulang. Yang memberi tahu adalah melihat ruang tunggu: berapa orang yang duduk, dan sudah berapa lama orang pertama menunggu. Yang tidak ditangkap analogi ini: di Kafka "ruang tunggu" tidak hilang setelah dilayani. Message tetap ada di log, dan backlog adalah selisih antara offset terakhir di partition dan offset yang sudah di-commit consumer group (consumer lag).

Di antara backlog dan umur message tertua, umur lebih mudah diberi ambang batas. "500 message menunggu" bisa normal pada jam sibuk dan bencana pada pukul tiga pagi. "Message tertua sudah menunggu 10 menit" selalu berarti sama: ada tamu yang sudah 10 menit belum menerima email.

### Who measures a dead consumer

Ada jebakan dalam cara mengukur backlog. Kalau consumer sendiri yang memperbarui gauge setiap selesai memproses message, gauge itu membeku pada nilai terakhir saat consumer macet. Angka yang membeku terlihat sehat.

Karena itu gauge antrean dihitung saat dibaca. Prometheus bekerja dengan model pull: ia mendatangi endpoint `/metrics` secara berkala (scrape). `prometheus.NewGaugeFunc` menerima sebuah function yang dipanggil pada setiap scrape, sehingga umur message tertua terus bertambah walaupun tidak ada satu baris kode handler pun yang berjalan. Pada sistem nyata, pengukuran ini biasanya ditaruh di proses terpisah dari consumer (untuk Kafka, sebuah lag exporter), supaya ia tetap hidup ketika consumer mati.

### Label cardinality

Setiap kombinasi nilai label adalah satu time series tersendiri yang disimpan Prometheus. Label `handler` dengan 5 nilai dan `outcome` dengan 2 nilai menghasilkan 10 series. Itu murah.

Label `booking_id` menghasilkan satu series per booking, selamanya. Setelah sebulan, Prometheus menyimpan ratusan ribu series yang masing-masing bernilai 1, memory-nya habis, dan query menjadi lambat. Aturannya: label hanya untuk dimensi yang jumlah nilainya kecil dan kamu bisa sebutkan semuanya dari kepala. Identitas satu booking adalah urusan log dan trace ([[18 Lab - Tracing]]), bukan metric.

### Alert on symptoms

Alert yang baik menjawab "apakah ada pengguna yang sedang dirugikan?". Alert yang buruk menjawab "apakah ada angka yang terlihat tidak biasa?".

CPU 95% bukan gejala. Kalau semua email terkirim tepat waktu, CPU tinggi berarti server dipakai dengan baik. Sebaliknya, CPU 5% saat consumer macet adalah insiden. Alert berbasis penyebab seperti ini menghasilkan dua kerugian sekaligus: ia berbunyi saat tidak ada masalah, dan diam saat ada masalah.

Tiga alert berbasis gejala untuk sebuah consumer:

1. **Umur message tertua melewati batas.** Ini gejala paling langsung, dan ia menangkap consumer yang mati, lambat, atau tersangkut pada satu message.
2. **Rasio kegagalan tinggi.** Rasio, bukan jumlah: 10 kegagalan dari 10 message adalah darurat, 10 dari 100.000 bukan.
3. **Dead letter queue bertambah.** Ada message yang sudah menyerah dan menunggu keputusan manusia ([[19 Lab - Fault Tolerance]]).

Setiap alert punya `for`: kondisi harus bertahan selama itu sebelum alert menyala, supaya lonjakan sesaat tidak membangunkan orang. Setiap alert juga punya severity. `page` berarti seseorang harus bertindak sekarang. `ticket` berarti cukup dikerjakan pada jam kerja.

## The Exercise

Kode ada di `labs/17-metrics-and-alerting/`.

| File | Isi | Diubah? |
|---|---|---|
| `backlog.go` | Antrean tiruan dengan `Len()` dan `OldestAge()` | Tidak |
| `metrics.go` | `Metrics`, `Middleware`, `RegisterQueueMetrics` | Ya |
| `alerts.yaml` | Prometheus alert rules | Ya (Step 5) |
| `metrics_test.go` | Enam skenario | Tidak |

Middleware di lab ini memakai tipe `message.HandlerMiddleware` dari Watermill, sama seperti di [[05 Lab - Middlewares]]. Test memanggil handler yang sudah dibungkus secara langsung, tanpa Router, supaya hasilnya deterministik.

### Step 0: The middleware is invisible

```bash
cd labs/17-metrics-and-alerting
go test -run Step0 -v
```

Hasilnya `PASS`. Middleware di kode awal meneruskan message, hasil, dan error apa adanya. Sifat ini harus tetap benar sampai akhir lab: instrumentasi tidak boleh mengubah perilaku handler.

### Step 1: Count outcomes

```bash
go test -run Step1 -v
```

Tebak dulu pesan gagalnya. Test menjalankan tiga message, satu di antaranya gagal, lalu mencari `pms_messages_processed_total` di registry. Metric itu belum ada.

**Task:** buat `CounterVec` dengan label `handler` dan `outcome`, daftarkan ke `reg` di `NewMetrics`, lalu naikkan di `Middleware`. Nilai `outcome` adalah `success` atau `failure`.

> [!hint]- Hint
> `prometheus.NewCounterVec(prometheus.CounterOpts{Name: ..., Help: ...}, []string{"handler", "outcome"})`. Simpan sebagai field di `Metrics`. Di middleware, panggil `next(msg)` dulu, lihat `err`, baru `WithLabelValues(handlerName, outcome).Inc()`. Daftarkan ke `reg` yang diterima sebagai parameter, bukan ke registry global: registry global membuat test saling mengotori.

### Step 2: Measure duration

```bash
go test -run Step2 -v
```

Handler di test memajukan jam tiruan 250 ms per message. Message ketiga gagal.

**Task:** tambahkan `HistogramVec` bernama `pms_message_processing_duration_seconds` dengan label `handler`. Ukur dengan `m.now()`, bukan `time.Now()`.

Perhatikan dua tuntutan test: tiga pengamatan (yang gagal ikut diukur), dan total 0,75 dalam satuan detik.

> [!hint]- Hint
> Ambil `start := m.now()` sebelum `next(msg)`, lalu `Observe(m.now().Sub(start).Seconds())` sesudahnya. Jangan taruh `Observe` di dalam cabang "berhasil". Pilih `Buckets` yang cocok untuk handler yang memanggil SMTP dan HTTP: dari puluhan milidetik sampai puluhan detik. Bucket bawaan berhenti di 10 detik.

### Step 3: Keep cardinality bounded

```bash
go test -run Step3 -v
```

Test memproses 50 booking di dua handler, lalu menghitung time series dengan `testutil.GatherAndCount`. Batasnya 4. Test juga menjalankan `testutil.GatherAndLint`, yang memeriksa konvensi penamaan Prometheus: counter berakhiran `_total`, satuan dasar seperti `_seconds`, dan `Help` terisi.

Kalau Step 1 dan 2 sudah benar, step ini kemungkinan langsung lulus. Ia ada sebagai pagar. Coba sengaja merusaknya: tambahkan label `booking_id` yang diisi dari `msg.Metadata.Get("booking_id")`, jalankan test, dan baca pesan gagalnya. Lalu kembalikan.

> [!hint]- Hint
> Kalau lint mengeluh, baca teksnya: ia menyebut nama metric dan aturan yang dilanggar. Keluhan paling umum adalah counter tanpa akhiran `_total` dan durasi dalam milidetik.

### Step 4: See the queue, not just the work

```bash
go test -run Step4 -v
```

Skenarionya consumer yang diam. Tiga message masuk, 90 detik berlalu, satu message lagi masuk, lalu lima menit berlalu tanpa ada yang memproses. Umur message tertua harus naik dari 90 ke 390 walaupun tidak ada handler yang berjalan.

**Task:** isi `RegisterQueueMetrics` supaya mendaftarkan `pms_queue_backlog_messages` dan `pms_queue_oldest_message_age_seconds`, keduanya dengan label `queue`.

> [!hint]- Hint
> Pakai `prometheus.NewGaugeFunc`. Function yang kamu berikan dipanggil setiap kali registry dibaca, jadi cukup kembalikan `float64(b.Len())` dan `b.OldestAge(now()).Seconds()`. Label `queue` nilainya tetap untuk satu pendaftaran, jadi taruh di `ConstLabels`. Pakai `reg.Register` dan kembalikan error-nya dengan `fmt.Errorf("...: %w", err)`.

### Step 5: Write the alerts

```bash
go test -run Step5 -v
```

`alerts.yaml` berisi satu alert buruk, `HighCPU`. Test menuntut alert itu hilang dan tiga alert ini ada:

| Alert | Kondisi |
|---|---|
| `OldestMessageTooOld` | umur message tertua di atas ambang batas |
| `HighFailureRatio` | rasio `outcome="failure"` terhadap total, dihitung dari `rate()` |
| `DeadLetterGrowing` | `increase()` pada `pms_dead_letter_messages_total` |

Setiap alert harus punya `for` minimal `1m`, `labels.severity` (`page` atau `ticket`), `annotations.summary`, dan `annotations.runbook`. Counter `pms_dead_letter_messages_total` belum dibuat di lab ini; anggap ia dinaikkan setiap kali sebuah message dipindahkan ke dead letter queue.

Test ini hanya memeriksa struktur file. Sintaks PromQL tidak divalidasi karena Prometheus tidak dijalankan. Kalau kamu punya `promtool`, jalankan `promtool check rules alerts.yaml`.

> [!hint]- Hint
> Rasio: `sum by (handler) (rate(metric{outcome="failure"}[5m])) / sum by (handler) (rate(metric[5m])) > 0.05`. Pilih severity dengan bertanya: kalau alert ini berbunyi pukul tiga pagi, apakah ada yang bisa dan harus dikerjakan saat itu juga?

## Solution

> [!success]- Solution
> Ide kuncinya ada tiga. Pertama, middleware mengukur di sekeliling `next(msg)` dan mencatat hasil apa pun keluarannya:
>
> ```go
> start := m.now()
> produced, err := next(msg)
>
> outcome := "success"
> if err != nil {
> 	outcome = "failure"
> }
> m.processed.WithLabelValues(handlerName, outcome).Inc()
> m.duration.WithLabelValues(handlerName).Observe(m.now().Sub(start).Seconds())
> return produced, err
> ```
>
> Kedua, kondisi antrean dihitung saat scrape, bukan saat consumer bekerja:
>
> ```go
> oldest := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
> 	Name:        "pms_queue_oldest_message_age_seconds",
> 	Help:        "Umur message tertua yang belum diproses.",
> 	ConstLabels: prometheus.Labels{"queue": queue},
> }, func() float64 { return b.OldestAge(now()).Seconds() })
> ```
>
> Ketiga, alert memakai rasio dan gejala. Kode lengkap ada di `_solusi/metrics.go` dan `_solusi/alerts.yaml`.

## When This Is the Wrong Choice

Metric adalah agregat. Ia menjawab "berapa banyak" dan "seberapa lama", dan tidak bisa menjawab "booking B-1042 kenapa". Untuk pertanyaan tentang satu kejadian, pakai log dengan correlation ID atau trace.

Histogram punya biaya: setiap bucket adalah satu time series per kombinasi label. Sepuluh bucket dikali dua puluh handler sudah dua ratus series. Kalau yang kamu butuhkan hanya "berapa kali", counter cukup.

Alert pada umur message tertua salah untuk antrean yang memang ditunda. Job yang sengaja dijadwalkan besok pagi akan terlihat "tua" sepanjang malam. Untuk antrean seperti itu, ukur keterlambatan terhadap jadwalnya, bukan umur sejak dibuat.

Terakhir, sistem dengan satu consumer dan sepuluh message per hari tidak butuh Prometheus. Sebuah query terjadwal ke tabel outbox yang mengirim pesan kalau ada baris `DEAD` atau baris pending berumur lebih dari satu jam memberi sinyal yang sama dengan biaya jauh lebih kecil.

## What Is Still Broken

1. **Backlog di lab ini tiruan.** `Backlog` hanya slice di memory. Untuk Kafka, backlog dihitung dari selisih offset per partition, dan umur message tertua dibaca dari timestamp record pada offset yang sudah di-commit. Keduanya biasanya diambil dari exporter terpisah, bukan dari consumer.
2. **Angka tidak menjelaskan sebab.** Alert `HighFailureRatio` memberi tahu bahwa handler email gagal, bukan bahwa SMTP timeout untuk booking tertentu. Menyambung satu booking dari HTTP request sampai handler adalah isi [[18 Lab - Tracing]].
3. **Counter dead letter belum ada.** `DeadLetterGrowing` merujuk ke metric yang baru punya sumber setelah dead letter queue dibangun di [[19 Lab - Fault Tolerance]].
4. **Alert belum pernah berbunyi.** Aturan yang tidak pernah diuji dengan kegagalan sungguhan sering salah ambang batasnya. Mematikan consumer dengan sengaja di staging dan menunggu alert adalah satu-satunya bukti.

## Reflection Questions

1. Laju `pms_messages_processed_total` turun ke nol pada pukul 02.00. Sebutkan dua penjelasan yang berlawanan, dan metric mana yang membedakannya.
2. Kenapa gauge umur message tertua harus dihitung saat scrape, bukan diperbarui oleh consumer setiap selesai memproses?
3. Seorang rekan mengusulkan label `recipient_email` supaya bisa melihat kegagalan per penerima. Apa yang terjadi pada Prometheus setelah sebulan, dan di mana informasi itu seharusnya disimpan?
4. Outbox billing di kantormu diproses cron tiap 2 menit dan punya status `PENDING` dan `DEAD`. Tulis dua alert berbasis gejala untuk outbox itu dalam kalimat biasa: kondisinya apa, ambang batasnya berapa, severity-nya apa, dan langkah pertama di runbook-nya apa.
5. Topic `emails.DLT` dicoba ulang oleh cron. Metric apa yang membedakan "DLT berisi message yang sedang menunggu giliran dicoba ulang" dari "DLT berisi message yang sudah gagal berkali-kali dan tidak akan pernah berhasil"?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Metrics - The RED and USE Methods]]
- [[Alerts That Do Not Cause Fatigue]]
- [[SLIs and SLOs]]
- [[Pull vs Push Metrics Collection]]
- [[Query Languages for Metrics]]
- [[Prometheus]]
