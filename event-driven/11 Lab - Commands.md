# 11 Lab - Commands

[[10 Lab - Outbox]] · [[00 Start Here]] · [[12 Lab - Read Models]]

- **Goal:** membedakan command dari event, lalu memasang command bus dan command handler dengan Watermill sampai sebuah command dikerjakan dan hasilnya dikabarkan sebagai event.
- **Covers:** command vs event, command bus, command processor dan handler, satu handler per command, topic terpisah untuk command dan event, penolakan bisnis vs kegagalan teknis.
- **Prerequisites:** [[04 Lab - Watermill Basics]] (router, ack/nack), [[08 Lab - Events with CQRS]] (event bus dan event processor).
- **Infrastructure:** none.
- **Time:** 2 jam.

## Why This Exists

Sampai lab ini, semua message berbentuk event: `BookingCreated`, `PaymentCaptured`. Event memberi tahu apa yang sudah terjadi dan tidak peduli siapa yang mendengarkan. Itu cocok untuk "kabari semua yang berkepentingan".

Sebagian pekerjaan tidak berbentuk kabar. Setelah booking dibuat, pembayaran harus ditarik dari kartu tamu. Itu bukan fakta yang sudah terjadi. Itu permintaan agar sesuatu dikerjakan, oleh tepat satu pihak, tepat satu kali, dan permintaan itu bisa ditolak.

Tim yang hanya punya event biasanya memaksakan event untuk tugas ini. Mereka membuat `PaymentCaptureRequested`, dan masalahnya muncul berurutan. Dua service mendengarkan event itu dan kartu tamu ditarik dua kali. Atau tidak ada yang mendengarkan dan tidak ada yang tahu, karena event tanpa pendengar adalah hal yang sah. Nama event berbentuk lampau untuk sesuatu yang belum terjadi adalah tanda bahwa yang dibutuhkan sebenarnya **command**.

## Concepts

### Dua jenis message, dua janji berbeda

| | Event | Command |
|---|---|---|
| Isi | Fakta yang sudah terjadi | Permintaan untuk melakukan sesuatu |
| Nama | Bentuk lampau: `PaymentCaptured` | Bentuk perintah: `CapturePayment` |
| Penerima | Nol, satu, atau banyak handler | Tepat satu handler |
| Pengirim tahu penerimanya? | Tidak | Ya, secara konsep |
| Bisa ditolak? | Tidak; fakta tidak bisa dibantah | Ya |
| Tanpa penerima berarti | Tidak apa-apa | Bug |

Padanan di luar software: pengumuman lewat pengeras suara dan surat tugas. Pengumuman "kereta dari Bandung sudah tiba" didengar semua orang di stasiun, dan masing-masing memutuskan sendiri mau berbuat apa. Surat tugas "periksa rem gerbong 4" ditujukan ke satu teknisi, harus dikerjakan, dan teknisi itu boleh menjawab "tidak bisa, gerbong 4 sedang dipakai". Perbedaan pentingnya dari analogi ini: surat tugas biasanya dijawab langsung ke pemberinya, sedangkan command yang lewat broker tidak punya jalur jawaban. Hasilnya dikabarkan sebagai event.

### Command bus dan command processor

Lab 08 memperkenalkan dua komponen untuk event: event bus (mengirim) dan event processor (menerima lalu memanggil handler). Sisi command punya pasangan yang sama.

```
              commands.CapturePayment                 events.PaymentCaptured
Send(cmd) ──► [ command bus ] ──► topic ──► [ command processor ] ──► handler
                                                                        │ gateway.Capture(...)
                                                                        ▼
                                                              Publish(PaymentCaptured)
                                                                        │
                                              ┌─────────────────────────┴──────────┐
                                              ▼                                    ▼
                                      MarkBookingPaid                        QueueReceipt
```

Command bus melakukan marshal terhadap struct command dan melakukan publish ke topic command. Command processor melakukan subscribe ke topic itu, melakukan unmarshal, lalu memanggil handler. Handler yang return `nil` berarti ack. Handler yang return error berarti nack, dan message dikirim ulang.

Command processor Watermill menolak dua handler untuk command yang sama pada saat pemasangan. `TestDemo_TwoHandlersForOneCommandAreRejected` di lab ini menunjukkannya. Di broker nyata, aturan "satu pelaksana" dijaga juga oleh consumer group: semua instance service pelaksana berada di satu group, jadi satu command hanya sampai ke satu instance.

### Kenapa topic-nya dipisah

Command dan event punya pembaca dan aturan yang berbeda. Topic event adalah kontrak publik: service lain boleh subscribe tanpa izin, dan retensinya sering panjang. Topic command adalah kotak masuk milik satu service: hanya pemiliknya yang boleh membaca, dan service lain yang ikut subscribe akan mengeksekusi perintah yang bukan untuknya.

Mencampur keduanya di satu topic juga merusak penanganan error. Event yang tidak dikenali boleh dilewati dengan aman. Command yang tidak dikenali adalah pekerjaan yang hilang.

### Menolak bukan gagal

Handler `CapturePayment` punya tiga kemungkinan hasil, dan ketiganya butuh perlakuan berbeda.

| Hasil | Arti | Yang dilakukan handler |
|---|---|---|
| Gateway menarik uang | Berhasil | Publish `PaymentCaptured`, return `nil` |
| Bank menolak kartu | Penolakan bisnis: jawaban yang sah | Publish `PaymentRejected`, return `nil` |
| Gateway timeout | Kegagalan teknis: belum ada jawaban | Return error supaya dikirim ulang |

Kesalahan yang paling sering adalah memperlakukan baris kedua seperti baris ketiga. Kartu yang ditolak akan ditolak lagi pada percobaan berikutnya. Kalau handler return error, command itu dikirim ulang terus, gateway dipanggil berkali-kali untuk kartu yang sama, dan message di belakangnya ikut tertahan. Error dari handler berarti "coba lagi", jadi hanya dipakai ketika mencoba lagi masuk akal.

## The Exercise

Kode ada di `labs/11-commands/`.

| File | Isi | Diubah? |
|---|---|---|
| `messages.go` | Command `CapturePayment`, event `PaymentCaptured` dan `PaymentRejected` | Tidak, baca dulu |
| `fakes.go` | Payment gateway tiruan, penyimpan status booking, daftar kuitansi | Tidak |
| `app.go` | Router, pub/sub, event bus, event processor dengan tiga event handler | Tidak, baca dulu |
| `commands.go` | `setupCommands`, `Send`, dan command handler | Ya |
| `lab_test.go` | Skenario | Tidak |

Baca `app.go` sebelum mulai. Sisi command yang akan kamu tulis hampir cerminan dari cara event bus dan event processor dipasang di sana.

### Step 0: Events fan out

```bash
cd labs/11-commands
go test -run 'Step0|Demo' -v
```

Keduanya `PASS`. `Step0` mengulang lab 08: satu `PaymentCaptured` sampai ke dua handler, yang satu menandai booking `paid` dan yang lain menjadwalkan kuitansi. `Demo` memasang dua handler untuk satu command dan mencetak penolakan dari Watermill.

### Step 1: A command reaches its handler

```bash
go test -run Step1 -v
```

Test gagal dengan `Send(CapturePayment) gagal: command bus belum dipasang`.

**Tugas:** di `commands.go`:

1. Di `setupCommands`, buat command bus dengan `cqrs.NewCommandBusWithConfig` dan simpan di `a.commandBus`.
2. Buat command processor dengan `cqrs.NewCommandProcessorWithConfig(a.router, ...)`.
3. Tulis method `handleCapturePayment(ctx context.Context, cmd *CapturePayment) error` yang memanggil `a.deps.Gateway.Capture`, lalu pasang dengan `processor.AddHandlers(cqrs.NewCommandHandler("CapturePayment", a.handleCapturePayment))`.
4. Ubah `Send` supaya meneruskan ke `a.commandBus.Send`.

> [!hint]- Hint
> `cqrs.CommandBusConfig` butuh `GeneratePublishTopic` (fungsi dari `cqrs.CommandBusGeneratePublishTopicParams` ke nama topic) dan `Marshaler`. `cqrs.CommandProcessorConfig` butuh `GenerateSubscribeTopic` (dari `cqrs.CommandProcessorGenerateSubscribeTopicParams`), `SubscriberConstructor` (return `a.pubSub`), dan `Marshaler`. Kedua struct params punya field `CommandName`. Pakai variabel `marshaler` dari `app.go`, dan `a.publisher` sebagai publisher. Kedua fungsi topic harus menghasilkan nama yang sama.

### Step 2: Separate topics

```bash
go test -run Step2 -v
```

Test memeriksa topic yang dipakai. Command `CapturePayment` harus berada di `commands.CapturePayment`, terpisah dari `events.*`. Kalau di Step 1 kamu sudah memberi nama itu, test ini langsung lulus.

> [!hint]- Hint
> Buat satu fungsi kecil, misalnya `commandTopic(name string) string`, dan panggil dari `GeneratePublishTopic` maupun `GenerateSubscribeTopic`. Nama yang ditulis di dua tempat suatu hari akan berbeda.

### Step 3: The handler announces what happened

```bash
go test -run Step3 -v
```

Pembayaran sudah ditarik, tetapi booking tidak pernah menjadi `paid` dan kuitansi tidak dijadwalkan. Dua event handler dari Step 0 menunggu `PaymentCaptured`, dan belum ada yang mengabarkannya.

**Tugas:** setelah `Capture` berhasil, publish `PaymentCaptured` lewat `a.Publish`.

Perhatikan apa yang **tidak** kamu tulis: command handler tidak memanggil `Bookings.SetStatus` atau `Receipts.Queue`. Ia tidak tahu kedua hal itu ada.

> [!hint]- Hint
> `return a.Publish(ctx, PaymentCaptured{BookingID: cmd.BookingID, AmountIDR: cmd.AmountIDR})`. Error dari `Publish` dikembalikan apa adanya, supaya command dicoba lagi kalau event-nya gagal dikabarkan.

### Step 4: Rejection is not failure

```bash
go test -run Step4 -v
```

Dua test. Di `BusinessRejectionIsAnEventNotAnError`, bank menolak kartu booking `B-2`. Di `TechnicalFailureIsRetried`, gateway timeout sekali lalu pulih.

Tebak dulu apa yang terjadi dengan handler Step 3 pada kasus pertama. Handler itu return error, gochannel mengirim ulang command-nya, dan handler dipanggil lagi tanpa henti. Test menunggu sampai booking berstatus `payment_failed`, dan itu tidak pernah terjadi.

**Tugas:** bedakan `ErrCardDeclined` dari error lain. Untuk kartu yang ditolak, publish `PaymentRejected` dan return `nil`. Untuk error lain, return error.

> [!hint]- Hint
> `errors.Is(err, ErrCardDeclined)`. Periksa kasus itu dulu, baru `if err != nil`.

## Solution

> [!success]- Solution
> Pemasangan command hampir sama dengan pemasangan event di `app.go`; yang berbeda hanya nama tipe config dan awalan topic. Bagian yang layak dihafal adalah handler dengan tiga cabangnya:
>
> ```go
> func (a *App) handleCapturePayment(ctx context.Context, cmd *CapturePayment) error {
> 	err := a.deps.Gateway.Capture(ctx, cmd.BookingID, cmd.AmountIDR)
> 	if errors.Is(err, ErrCardDeclined) {
> 		return a.Publish(ctx, PaymentRejected{BookingID: cmd.BookingID, Reason: err.Error()})
> 	}
> 	if err != nil {
> 		return fmt.Errorf("capture payment %s: %w", cmd.BookingID, err)
> 	}
> 	return a.Publish(ctx, PaymentCaptured{BookingID: cmd.BookingID, AmountIDR: cmd.AmountIDR})
> }
> ```
>
> Kode lengkap: `labs/11-commands/_solusi/commands.go`.

## When This Is the Wrong Choice

Command lewat broker menukar jawaban langsung dengan ketahanan. Itu bukan pertukaran yang selalu menguntungkan.

Kalau pemanggil butuh hasilnya sekarang, pakai function call atau HTTP. Tamu yang menekan "Bayar" menunggu di layar; mengirim `CapturePayment` ke broker lalu menebak kapan `PaymentCaptured` datang membuat alurnya rumit tanpa manfaat. Command asinkron cocok ketika pemanggil tidak perlu menunggu: proses malam yang menagih no-show, atau langkah berikutnya dalam alur panjang.

Kalau pengirim dan handler berada di proses yang sama dan pekerjaannya cepat, function call biasa lebih jujur. Ia punya return value, stack trace, dan transaction. Membungkusnya dengan command bus menghilangkan ketiganya.

Kalau kamu tidak peduli siapa yang mengerjakan dan berapa banyak yang bereaksi, yang kamu butuhkan adalah event.

Tanda command bus dipakai berlebihan: setiap handler hanya meneruskan ke satu function, tidak ada yang pernah dicoba ulang, dan setiap pengirim langsung menunggu event balasannya.

## What Is Still Broken

1. **Retry bisa menarik kartu dua kali.** Kalau `Capture` berhasil tetapi `Publish(PaymentCaptured)` gagal, handler return error dan command dikirim ulang. Percobaan kedua menarik kartu lagi. Obatnya gabungan dua lab sebelumnya: idempotency key yang dikirim ke gateway ([[09 Lab - At-Least-Once Delivery]]), dan menulis event lewat outbox ([[10 Lab - Outbox]]).
2. **Retry tanpa batas dan tanpa jeda.** Gateway yang mati satu jam berarti satu jam nack tanpa henti. Batas percobaan, backoff, dan poison queue ada di [[19 Lab - Fault Tolerance]].
3. **Command hilang kalau proses mati.** gochannel hidup di memori. Di broker nyata command tersimpan, tetapi `Send` sendiri bisa gagal setelah database berubah. Itu dual write lagi.
4. **Pengirim tidak tahu hasilnya.** Alur yang butuh "tarik pembayaran, kalau ditolak lepaskan kamarnya" memerlukan sesuatu yang mengingat langkah. Itu [[20 Lab - Sagas and Process Managers]].

## Reflection Questions

1. Sebuah message bernama `InvoiceEmailRequested`. Apa yang janggal dari nama itu, dan nama apa yang lebih jujur?
2. Kenapa event tanpa handler adalah hal yang sah, sedangkan command tanpa handler adalah bug?
3. Aplikasi di kantormu menaruh email di Kafka dan `email-service` mengirimnya. Message itu lebih tepat disebut command atau event? Apa yang akan terjadi kalau service kedua ikut membaca topic yang sama dengan consumer group berbeda?
4. `email-service` menerima email dengan alamat penerima yang formatnya tidak valid. Menurut tabel di bagian Concepts, ini baris yang mana, dan apa yang seharusnya dilakukan handler?
5. Di alur booking dari channel manager, sebutkan satu langkah yang lebih baik berupa function call biasa daripada command, dan alasannya.

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[CQRS]]
- [[Queue vs Log Semantics]]
- [[Delivery Semantics]]
- [[Sagas - Orchestration vs Choreography]]
