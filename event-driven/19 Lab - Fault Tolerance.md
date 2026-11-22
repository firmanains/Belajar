# 19 Lab - Fault Tolerance

[[18 Lab - Tracing]] · [[00 Start Here]] · [[20 Lab - Sagas and Process Managers]]

**Goal:** membuat consumer yang tetap berdiri ketika dependency-nya lambat atau mati, dan yang tidak tersumbat oleh satu message buruk.
**Covers:** fault tolerance: timeout, retry dengan exponential backoff dan jitter, circuit breaker, poison queue (dead letter queue), requeue.
**Prerequisites:** [[06 Lab - Errors]] (jenis kegagalan dan retry dasar), [[09 Lab - At-Least-Once Delivery]] (message bisa datang dua kali).
**Infrastructure:** none.
**Time:** 3–4 jam.

---

## Why This Exists

Consumer email bergantung pada server SMTP. Server itu milik pihak lain, dan suatu saat ia akan lambat atau mati. Pertanyaannya bukan apakah itu terjadi, melainkan apa yang ikut jatuh bersamanya.

Tanpa perlindungan, urutan kejadiannya selalu mirip. SMTP mulai lambat. Setiap handler menunggu tanpa batas, jadi semua worker terpakai untuk menunggu. Backlog naik. Seseorang menambahkan retry tanpa jeda, sehingga server yang sudah kewalahan kini menerima beban tiga kali lipat. SMTP mati total. Sementara itu, satu message dengan alamat email rusak dicoba ulang terus-menerus di kepala antrean, dan seribu message sehat di belakangnya tidak pernah tersentuh.

Empat teknik di lab ini masing-masing memotong satu mata rantai dari cerita itu:

| Mata rantai | Teknik |
|---|---|
| Menunggu tanpa batas | Timeout pada setiap panggilan keluar |
| Gangguan sesaat langsung dianggap gagal | Retry dengan backoff dan jitter, jumlahnya dibatasi |
| Dependency yang sakit terus dihujani request | Circuit breaker |
| Satu message buruk menahan semua yang lain | Poison queue |

Kamu sudah mengoperasikan ujung terakhirnya. `email-service` punya dead letter topic `emails.DLT` yang dicoba ulang oleh cron. Lab ini menjelaskan kenapa topic itu ada, kapan sebuah message layak masuk ke sana, dan apa bahayanya kalau isinya dicoba ulang tanpa batas.

## Concepts

### Recap from lab 06

[[06 Lab - Errors]] membedakan tiga jenis kegagalan: transient (SMTP sibuk, akan sembuh sendiri), malformed message (tidak akan pernah berhasil), dan bug kode. Hanya yang transient layak dicoba lagi. Di lab ini kegagalan yang tidak akan sembuh ditandai dengan `ErrPermanent`.

### Timeouts: the cheapest protection

Panggilan keluar tanpa timeout adalah janji untuk menunggu selamanya. Server yang menerima koneksi TCP lalu diam tidak menghasilkan error apa pun; ia hanya tidak menjawab. Worker yang menunggunya tidak mati dan tidak menulis log. Ia hilang begitu saja dari kapasitas.

Di Go, timeout dipasang lewat `context.WithTimeout`, dan `ctx` hasilnya diteruskan ke panggilan keluar. Library yang ditulis dengan benar berhenti ketika `ctx` berakhir. Nilai timeout dipilih dari perilaku normal dependency: kalau SMTP biasanya menjawab dalam dua detik, sepuluh detik sudah longgar, dan enam puluh detik hanya menunda kabar buruk.

### Retry: backoff, jitter, and a limit

Retry tanpa jeda mengubah gangguan kecil menjadi serangan. Bayangkan SMTP menolak karena kewalahan, dan seratus consumer langsung mencoba lagi pada milidetik yang sama. Server itu kini menerima beban lebih besar daripada saat ia mulai menolak.

**Exponential backoff** menggandakan jeda setiap kali gagal: 100 ms, 200 ms, 400 ms, sampai batas `MaxDelay`. Dependency diberi ruang yang makin lebar untuk pulih.

Backoff saja belum cukup. Kalau seratus consumer gagal pada saat yang sama dan semuanya menunggu tepat 100 ms, mereka kembali bersamaan, lalu bersamaan lagi setelah 200 ms. Gelombangnya hanya lebih jarang, tidak lebih kecil. **Jitter** memecah gelombang itu dengan mengacak jeda. Varian yang dipakai di lab ini disebut full jitter: jeda diambil acak di antara nol dan nilai backoff saat itu.

```
tanpa jitter:   ████      ████            ████                    ████
                 t=0      100ms           300ms                   700ms

full jitter:    █▄▂█  ▂ █▄ ▂ █ ▄  ▂  █ ▂ ▄   █  ▂   ▄  █   ▂    ▄   █
                beban tersebar, tidak ada puncak bersama
```

Cara paling mudah memahaminya: pintu keluar stadion. Kalau semua penonton disuruh "tunggu lima menit lalu keluar", mereka tetap berdesakan lima menit kemudian. Kalau setiap orang disuruh menunggu selama angka acak antara nol dan lima menit, arusnya rata. Perbedaan pentingnya: penonton pada akhirnya pasti keluar, sedangkan retry harus punya batas. Retry tanpa `MaxAttempts` menahan message itu selamanya dan menahan semua message di belakangnya.

### Circuit breaker: stop calling a dead dependency

Retry melindungi satu panggilan. Ia tidak tahu bahwa dua ratus panggilan sebelumnya juga gagal. Kalau SMTP mati selama sepuluh menit, setiap message tetap menghabiskan seluruh jatah timeout dan retry-nya sebelum menyerah. Consumer bekerja keras untuk hasil nol, dan server yang sedang berusaha bangun terus dipukul.

Circuit breaker mengingat riwayat kegagalan lintas panggilan. Ia punya tiga keadaan:

```
                 failureThreshold kegagalan berturut-turut
        ┌──────────┐ ─────────────────────────────────► ┌──────────┐
        │  closed  │                                    │   open   │ ◄──┐
        └──────────┘ ◄───────────────┐                  └──────────┘    │
         panggilan                   │                   panggilan      │
         diteruskan                  │ percobaan         ditolak        │ percobaan
                                     │ berhasil          seketika       │ gagal
                                     │                        │         │
                                ┌───────────┐   cooldown      │         │
                                │ half-open │ ◄───────────────┘         │
                                └───────────┘ ──────────────────────────┘
                                 satu panggilan percobaan diizinkan
```

Saat **closed**, panggilan lewat dan kegagalan berturut-turut dihitung. Satu keberhasilan mengembalikan hitungan ke nol. Saat **open**, panggilan ditolak seketika dengan `ErrCircuitOpen`, tanpa menyentuh dependency. Setelah cooldown berlalu, breaker menjadi **half-open**: satu panggilan percobaan diizinkan lewat. Kalau berhasil, breaker menutup. Kalau gagal, ia membuka lagi untuk satu cooldown penuh.

Namanya diambil dari sekring listrik, dan cara kerjanya memang serupa: memutus arus supaya kerusakan tidak menjalar. Analogi ini berhenti bekerja di satu titik: sekring rumah harus dinaikkan lagi oleh orang, sedangkan circuit breaker mencoba sendiri lewat half-open.

Bagi consumer message, breaker yang terbuka berarti "berhenti mengambil message dulu". Message yang ditolak breaker tidak boleh dihitung sebagai percobaan gagal milik message itu; penyebabnya ada pada dependency, bukan pada message.

### Poison queue: move it aside

Stream diproses berurutan. Message yang gagal dan terus dicoba ulang menahan semua message di belakangnya. Gejala ini disebut head-of-line blocking, dan message penyebabnya disebut poison message.

Jalan keluarnya adalah menyerah secara tertib. Setelah `maxAttempts` percobaan, message dipindahkan ke tempat terpisah bersama alasan gagalnya dan jumlah percobaannya, lalu consumer lanjut ke message berikutnya. Tempat terpisah itu disebut dead letter queue atau poison queue. Di Kafka bentuknya topic tersendiri, seperti `emails.DLT`.

Dead letter queue bukan tempat sampah. Ia adalah daftar pekerjaan yang butuh keputusan. Karena itu harus ada jalan pulang (**requeue**): setelah penyebabnya diperbaiki, message dikembalikan ke stream utama. Urutan langkah requeue penting: publish dulu, hapus dari dead letter queue kemudian. Kalau proses mati di antara keduanya, message terkirim dua kali, dan itu aman selama consumer idempotent. Urutan sebaliknya membuat message hilang.

## The Exercise

Kode ada di `labs/19-fault-tolerance/`.

| File | Isi | Diubah? |
|---|---|---|
| `types.go` | `Email`, `Message`, `Sender`, error sentinel, `DeadLetterQueue` | Tidak |
| `timeout.go` | `SendWithTimeout` | Ya (Step 1) |
| `retry.go` | `Retry` | Ya (Step 2) |
| `breaker.go` | `Breaker` | Ya (Step 3) |
| `poison.go` | `Consume` dan `Requeue` | Ya (Step 4, 5) |
| `fault_tolerance_test.go` | Enam skenario | Tidak |

Jam, jeda, dan angka acak disuntikkan dari luar. Itu sebabnya `Retry` menerima `Sleep` dan `Jitter`, dan `NewBreaker` menerima `now`: test bisa memajukan waktu 30 detik tanpa menunggu 30 detik.

### Step 0: A healthy dependency

```bash
cd labs/19-fault-tolerance
go test -run Step0 -v
```

`PASS`. Tiga email terkirim, dead letter queue kosong.

### Step 1: Put a timeout on the outbound call

```bash
go test -run Step1 -v
```

`hangingSender` meniru server SMTP yang menerima koneksi lalu diam. Test meminta timeout 50 ms. Tebak dulu berapa lama test ini berjalan di kode awal.

Ia berjalan satu detik penuh, yaitu batas jaring pengaman milik test. Tanpa jaring itu, test menggantung selamanya, seperti worker di production.

**Task:** pasang timeout di `SendWithTimeout`.

> [!hint]- Hint
> `ctx, cancel := context.WithTimeout(ctx, timeout)` lalu `defer cancel()`. Teruskan `ctx` yang baru ke `sender.Send`. `defer cancel()` wajib walaupun panggilan berhasil, supaya timer-nya dilepas. Bungkus error dengan `%w` supaya `errors.Is(err, context.DeadlineExceeded)` tetap benar.

### Step 2: Retry with backoff and jitter

```bash
go test -run Step2 -v
```

Empat sub-test, empat sifat:

1. Gangguan sesaat sembuh di percobaan ketiga, dengan jeda 50 ms lalu 100 ms.
2. Kegagalan terus-menerus berhenti setelah `MaxAttempts`, dan backoff tertahan di `MaxDelay`.
3. `ErrPermanent` tidak dicoba lagi.
4. Kalau `Sleep` mengembalikan error (context dibatalkan), `Retry` berhenti.

`Jitter` di test selalu mengembalikan 0,5, jadi jeda yang diharapkan adalah setengah dari backoff: backoff 100, 200, 300, 300 menghasilkan jeda 50, 100, 150, 150.

**Task:** tulis loop retry di `Retry`.

> [!hint]- Hint
> Simpan `backoff := cfg.BaseDelay`. Di tiap putaran: jalankan `op`; kalau `nil`, selesai; kalau `errors.Is(err, ErrPermanent)`, kembalikan; kalau percobaan sudah mencapai `MaxAttempts`, kembalikan error yang dibungkus `%w`. Selain itu hitung `delay := time.Duration(cfg.Jitter() * float64(backoff))`, panggil `cfg.Sleep(ctx, delay)`, lalu `backoff = min(backoff*2, cfg.MaxDelay)`. Jangan panggil `Sleep` setelah percobaan terakhir.

### Step 3: Build a circuit breaker

```bash
go test -run Step3 -v
```

Lima sub-test mengikuti diagram keadaan di atas, satu panah per sub-test. Kerjakan berurutan; sub-test pertama sudah lulus di kode awal.

**Task:** isi `Breaker`. Tambahkan field untuk menyimpan keadaan, lalu tulis `State` dan `Do`.

Sub-test terakhir memanggil `Do` dari dalam `Do`. Itu cara deterministik untuk meniru dua panggilan yang datang bersamaan saat half-open. Akibatnya, mutex tidak boleh dipegang selama `op` berjalan.

> [!hint]- Hint
> Kamu tidak perlu menyimpan tiga keadaan. Simpan `open bool` dan `openedAt time.Time`; half-open adalah "open yang cooldown-nya sudah lewat", dan itu bisa dihitung dari `now()`. Tambahkan `failures int` dan `trialInFlight bool`. Pecah `Do` menjadi tiga bagian: `allow()` yang mengunci, memutuskan, dan melepas kunci; `op()` tanpa kunci; `record(err)` yang mengunci lagi dan memperbarui keadaan. Percobaan yang gagal saat half-open langsung membuka lagi dan mengisi ulang `openedAt`.

### Step 4: Move poison messages aside

```bash
go test -run Step4 -v
```

Lima message. `m-2` selalu gagal dengan `SMTP 451`. `m-4` punya alamat tidak valid (`ErrPermanent`). Tiga lainnya sehat.

Kode awal berhenti di `m-2` dan mengembalikan error. `m-3`, `m-4`, dan `m-5` tidak pernah diproses.

**Task:** ubah `Consume` supaya message yang gagal dicoba sampai `maxAttempts` kali lalu masuk dead letter queue dengan alasan dan jumlah percobaannya. Error permanent masuk dead letter queue setelah satu percobaan. Stream harus terus mengalir.

> [!hint]- Hint
> Loop dalam per message: naikkan `attempts`, panggil `handle`, berhenti kalau `err == nil` atau `errors.Is(err, ErrPermanent)`. Setelah loop, kalau `err != nil`, panggil `dlq.Add(DeadLetter{Message: msg, Reason: err.Error(), Attempts: attempts})`. `Consume` sendiri mengembalikan `nil`: message yang dipinggirkan bukan kegagalan stream.

### Step 5: Requeue without losing the message

```bash
go test -run Step5 -v
```

SMTP sudah pulih, dan message di dead letter queue perlu dikembalikan.

**Task:** tulis `Requeue`. Kalau publish berhasil, message keluar dari dead letter queue. Kalau publish gagal, message tetap di sana dan error dikembalikan. Kalau ID tidak ditemukan, kembalikan `ErrNotInDeadLetterQueue` tanpa memanggil publish.

> [!hint]- Hint
> Cari entry-nya, panggil `publish(entry.Message)`, dan hapus dari `q.entries` hanya kalau publish mengembalikan `nil`. Menghapus dulu lalu publish terlihat sama di jalur bahagia, dan kehilangan message di jalur gagal.

## Solution

> [!success]- Solution
> Inti `Retry`: tiga alasan berhenti, lalu jeda acak yang membesar.
>
> ```go
> backoff := cfg.BaseDelay
> for attempt := 1; ; attempt++ {
> 	err := op(ctx)
> 	if err == nil {
> 		return nil
> 	}
> 	if errors.Is(err, ErrPermanent) {
> 		return err
> 	}
> 	if attempt >= cfg.MaxAttempts {
> 		return fmt.Errorf("menyerah setelah %d percobaan: %w", attempt, err)
> 	}
>
> 	delay := time.Duration(cfg.Jitter() * float64(backoff))
> 	if err := cfg.Sleep(ctx, delay); err != nil {
> 		return fmt.Errorf("retry dibatalkan: %w", err)
> 	}
> 	backoff = min(backoff*2, cfg.MaxDelay)
> }
> ```
>
> Inti `Breaker`: keputusan diambil di bawah mutex, `op` berjalan di luar mutex.
>
> ```go
> func (b *Breaker) Do(op func() error) error {
> 	if err := b.allow(); err != nil {
> 		return err
> 	}
> 	err := op()
> 	b.record(err)
> 	return err
> }
> ```
>
> Inti `Requeue`: publish dulu, hapus kemudian. Kode lengkap ada di `_solusi/`.

## When This Is the Wrong Choice

**Retry** salah untuk operasi yang tidak idempotent. Mencoba ulang `CapturePayment` setelah timeout bisa menagih tamu dua kali, karena timeout tidak memberi tahu apakah permintaan pertama sampai. Retry hanya aman kalau penerimanya mengenali permintaan ganda.

**Retry bertingkat** juga berbahaya. Kalau handler mencoba 3 kali, HTTP client di dalamnya 3 kali, dan broker mengirim ulang 3 kali, satu message menghasilkan 27 panggilan ke dependency yang sedang sakit. Pilih satu lapisan yang bertanggung jawab atas retry.

**Circuit breaker** berlebihan untuk dependency yang jarang dipanggil. Breaker butuh arus panggilan untuk belajar; dengan sepuluh panggilan per hari, ia tidak pernah punya cukup data, dan timeout ditambah retry sudah memadai. Breaker juga tidak cocok kalau kegagalan bergantung pada isi message: tiga alamat email rusak berturut-turut bukan tanda SMTP mati. Karena itu error permanent sebaiknya tidak dihitung oleh breaker.

**Dead letter queue** salah kalau urutan message wajib dijaga. Meminggirkan `BookingCreated` lalu memproses `BookingCancelled` milik booking yang sama menghasilkan keadaan yang tidak masuk akal. Untuk stream seperti itu, pilihannya adalah berhenti dan memanggil manusia, atau meminggirkan semua message dengan key yang sama ([[13 Lab - Message Ordering]]).

## What Is Still Broken

1. **Semua keadaan ada di memory.** Hitungan percobaan, keadaan breaker, dan isi dead letter queue hilang saat proses restart. Di Kafka, hitungan percobaan biasanya dibawa di header message, dan dead letter queue adalah topic.
2. **Breaker per proses.** Sepuluh replica consumer berarti sepuluh breaker yang masing-masing harus menemukan sendiri bahwa SMTP mati.
3. **`Consume` mencoba ulang tanpa jeda.** Supaya lab tetap kecil, loop di Step 4 tidak memakai `Retry` dari Step 2. Consumer sungguhan menggabungkan keempat teknik: timeout di dalam, retry di sekelilingnya, breaker di sekeliling retry, dead letter queue sebagai jalan keluar terakhir.
4. **Requeue otomatis bisa berputar selamanya.** Cron yang mengembalikan isi dead letter queue ke stream utama menyembuhkan gangguan transient yang panjang. Untuk poison message sejati, ia hanya membuat lingkaran: gagal, dipinggirkan, dikembalikan, gagal lagi. Lingkaran itu butuh batasnya sendiri.
5. **Tidak ada yang tahu dead letter queue terisi.** Alert `DeadLetterGrowing` dari [[17 Lab - Metrics and Alerting]] baru berguna kalau `Consume` menaikkan counter setiap kali memanggil `dlq.Add`.

## Reflection Questions

1. SMTP mati selama sepuluh menit. Jelaskan apa yang dialami server SMTP dan apa yang dialami backlog, pertama dengan retry tanpa batas dan tanpa jeda, lalu dengan backoff, jitter, dan circuit breaker.
2. Kenapa backoff tanpa jitter belum menyelesaikan masalah ketika seratus consumer gagal pada saat yang sama?
3. Kenapa message yang ditolak karena breaker sedang open tidak boleh menambah hitungan percobaan milik message itu?
4. `emails.DLT` di `email-service` dicoba ulang oleh cron. Sebuah email dengan alamat penerima yang tidak valid masuk ke sana. Apa yang terjadi padanya dari waktu ke waktu, dan informasi apa yang harus ikut dibawa message itu supaya lingkarannya bisa diputus?
5. Outbox billing di kantormu punya status `DEAD`. Samakan dengan istilah di lab ini: apa padanan `maxAttempts`, apa padanan dead letter queue, dan apa padanan requeue? Kondisi apa yang harus kamu periksa sebelum mengembalikan baris `DEAD` ke `PENDING`?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[Timeout Budgets]]
- [[Retries with Exponential Backoff and Jitter]]
- [[Circuit Breakers]]
- [[Dead Letter Queues]]
- [[Bulkheads]]
- [[Term - Poison Message]]
- [[Case - The Retry Storm That Became a Total Outage]]
