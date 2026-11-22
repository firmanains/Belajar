# 08 Lab - Events with CQRS

[[07 Lab - Component Testing]] · [[00 Start Here]] · [[09 Lab - At-Least-Once Delivery]]

---

**Goal:** memecah satu handler besar menjadi beberapa event handler yang independen dengan komponen `cqrs` dari Watermill, lalu mengatur consumer group supaya handler tidak saling menyumbat dan beberapa instance berbagi pekerjaan.

**Covers:** sisi event dari CQRS: `EventBus`, `EventProcessor`, typed event handler, fan-out satu event ke banyak handler, satu subscription per handler, consumer group untuk scaling.

**Prerequisites:** [[04 Lab - Watermill Basics]] dan [[06 Lab - Errors]]. Konsep consumer group dari [[02 Lab - Message Broker]].

**Infrastructure:** none.

**Time:** 2–3 jam.

---

## Why This Exists

Ketika sebuah booking dibuat, tiga hal harus terjadi: availability berkurang, tamu menerima email konfirmasi, dan channel manager diberi tahu. Cara paling langsung adalah satu handler yang mengerjakan ketiganya berurutan.

Handler itu benar selama tidak ada yang gagal. Begitu SMTP timeout di langkah kedua, handler mengembalikan error, broker mengantar ulang message, dan handler mulai lagi dari langkah pertama. Availability berkurang untuk kedua kalinya. Satu tamu memesan dua kamar, dan hotel kehilangan empat.

Ada kerugian kedua. Selama SMTP mati, handler itu tidak pernah selesai, jadi channel manager tidak pernah diberi tahu untuk booking ini maupun booking berikutnya. Gangguan pada email, yang paling tidak mendesak dari ketiganya, menghentikan sinkronisasi availability, yang paling mendesak.

Tiga akibat itu tidak saling bergantung. Mereka hanya kebetulan dipicu oleh fakta yang sama. Lab ini memisahkannya.

Ada pula urusan yang lebih membosankan. Di lab sebelumnya setiap handler menulis sendiri `json.Unmarshal`, dan nama topic diketik sebagai string. Bug wiring di [[07 Lab - Component Testing]] lahir dari satu string seperti itu. Komponen `cqrs` menurunkan nama topic dari tipe event, sehingga string itu tidak perlu diketik.

## Concepts

### Recap Lab 04 dan 06

Router memanggil handler untuk setiap message; error berarti nack dan message diantar ulang. Dalam satu partition, message berikutnya baru diantar setelah message sekarang mendapat ack, sehingga satu message yang terus gagal menahan semua di belakangnya.

### CQRS, dan bagian mana yang dibahas di sini

CQRS adalah singkatan dari Command Query Responsibility Segregation: jalur yang mengubah data dipisahkan dari jalur yang membacanya. Package `components/cqrs` di Watermill menyediakan perkakas messaging untuk pola itu: `EventBus` dan `EventProcessor` untuk event, `CommandBus` dan `CommandProcessor` untuk command.

Lab ini hanya memakai sisi event. Command dibahas di [[11 Lab - Commands]], dan read model di [[12 Lab - Read Models]].

### Event bus dan event processor

```
                                         topic: events.BookingCreated
                                                     │
 app.EventBus.Publish(ctx, BookingCreated{...})      │        ┌─► group decrease-availability   ──► handler
        │                                            │        │
        ▼                                            ├────────┼─► group send-confirmation-email ──► handler
   Marshaler: struct ──► JSON + nama event ──────────┘        │
                                                              └─► group sync-channel-manager    ──► handler
```

| Bagian | Tugasnya |
|---|---|
| `EventBus` | Menerima struct Go, mengubahnya menjadi message, dan menerbitkannya ke topic yang benar |
| `Marshaler` | Mengubah struct menjadi payload dan sebaliknya, dan menentukan nama event. Di lab ini `cqrs.JSONMarshaler{GenerateName: cqrs.StructName}`, jadi `BookingCreated` bernama `"BookingCreated"` |
| `GeneratePublishTopic`, `GenerateSubscribeTopic` | Menurunkan nama topic dari nama event. Di lab ini `"events." + nama` |
| `EventProcessor` | Mendaftarkan event handler ke router |
| `SubscriberConstructor` | Dipanggil sekali untuk setiap handler dan mengembalikan subscriber miliknya |
| `cqrs.NewEventHandler` | Membungkus fungsi bertipe `func(ctx, *T) error` menjadi handler. Unmarshal dikerjakan untukmu |

Handler jadi sependek ini:

```go
cqrs.NewEventHandler("send-confirmation-email", func(ctx context.Context, event *BookingCreated) error {
	return emailer.SendConfirmation(ctx, event.BookingID, event.GuestEmail)
})
```

Handler mendaftar ke sebuah event lewat tipe parameternya. Tidak ada nama topic di kode handler.

### Satu event, banyak pelanggan

Padanan terdekatnya di luar software adalah koran langganan. Penerbit mencetak satu edisi dan tidak tahu siapa saja pembacanya. Setiap pelanggan menerima eksemplarnya sendiri dan membacanya dengan kecepatannya sendiri. Pelanggan yang sedang ke luar kota tidak menghentikan pengantaran ke rumah lain.

Penerbit di sini adalah kode yang memanggil `EventBus.Publish`. Ia tidak tahu ada tiga handler, dan handler keempat bisa ditambahkan tanpa menyentuhnya. Pola ini disebut fan-out.

Perbedaan pentingnya ada dua. Loper koran tidak kembali kalau pelanggan belum membaca; broker mengantar ulang sampai handler melakukan ack. Dan pelanggan baru tidak menerima edisi lama: handler yang ditambahkan hari ini tidak otomatis menerima event kemarin, kecuali broker dan consumer group-nya diatur untuk itu.

### Consumer group adalah identitas sebuah langganan

Di analogi koran, "pelanggan" adalah alamat rumah, bukan orangnya. Dua orang di rumah yang sama berbagi satu eksemplar. Consumer group adalah alamat itu.

| Susunan | Akibat |
|---|---|
| Handler berbeda, group berbeda | Masing-masing menerima semua event dan punya posisi baca sendiri |
| Handler yang sama di dua instance, group yang sama | Dua instance berbagi; setiap event diproses salah satunya |
| Semua handler memakai satu group | Setiap event hanya sampai ke **salah satu** handler |
| Group berbeda untuk setiap instance | Setiap instance memproses semua event; pekerjaan menjadi ganda |

Dua baris pertama adalah tujuan. Dua baris terakhir adalah kesalahan konfigurasi yang umum, dan keduanya tidak menghasilkan error.

Aturan yang memenuhi dua baris pertama: **nama consumer group sama dengan nama handler**. Nama itu berbeda antar handler, dan sama di semua instance serta setelah restart.

```
            instance 1                         instance 2
   ┌──────────────────────────┐       ┌──────────────────────────┐
   │ decrease-availability    │◄─────►│ decrease-availability    │  satu group: berbagi
   │ send-confirmation-email  │◄─────►│ send-confirmation-email  │  satu group: berbagi
   │ sync-channel-manager     │◄─────►│ sync-channel-manager     │  satu group: berbagi
   └──────────────────────────┘       └──────────────────────────┘
```

`gochannel` tidak punya consumer group, jadi lab ini memakai `Broker` yang sama dengan [[06 Lab - Errors]]. `broker.Subscriber("nama")` mengembalikan subscriber yang bergabung ke group bernama itu.

## The Exercise

Kode ada di `labs/08-events-with-cqrs/`.

| File | Isi | Diubah? |
|---|---|---|
| `broker.go` | Broker tiruan dengan consumer group dan redelivery | Tidak |
| `events.go` | `BookingCreated`, `BookingCancelled`, dan interface dependency | Tidak |
| `app.go` | `NewApp`: event bus, event processor, dan pendaftaran handler | Ya |
| `app_test.go` | Lima test | Tidak |

Baca `NewApp` sampai kamu bisa menunjuk baris yang menentukan nama topic dan baris yang menentukan nama consumer group.

### Step 0: One Event, Three Effects

```bash
cd ~/Developer/belajar/event-driven/labs/08-events-with-cqrs
go test -run Step0 -v
```

`PASS`. Satu handler bernama `on-booking-created` mengerjakan tiga hal berurutan, dan di dunia tanpa kegagalan itu cukup.

### Step 1: Split the Handler

```bash
go test -run Step1 -v
```

SMTP tiruan gagal dua kali, lalu pulih. Tebak dulu: berapa kali `Availability.Decrease` dipanggil untuk satu booking ini?

```
availability berubah -6, seharusnya -2: kegagalan email ikut mengulang pengurangan kamar
```

**Task:** ganti handler `on-booking-created` dengan tiga handler, satu untuk setiap akibat. Ketiganya menerima `*BookingCreated`.

> [!hint]- Hint
> `processor.AddHandlers` menerima banyak handler sekaligus. Beri nama yang menggambarkan akibatnya, misalnya `decrease-availability`, `send-confirmation-email`, dan `sync-channel-manager`. Nama handler harus unik di dalam satu router.

### Step 2: Predict Before You Run

Baca test `TestStep2_EmailOutageDoesNotBlockOtherHandlers` di `app_test.go`. SMTP mati total untuk booking B-1. Test menerbitkan B-1 lalu B-2, dan menuntut availability dan channel manager memproses keduanya.

Sebelum menjalankan, tulis jawabanmu: setelah perubahan di Step 1, apakah test ini lulus tanpa perubahan lagi? Kenapa?

```bash
go test -run Step2 -v
```

Tidak ada kode yang perlu ditulis di step ini. Yang perlu kamu pastikan adalah bahwa kamu bisa menjelaskan hasilnya. Sebagai pembanding, kode awal gagal di test ini dengan pesan:

```
channel manager diberi tahu untuk B-1 dan B-2; handler lain ikut tersumbat oleh email B-1: tidak terjadi dalam 2s
```

> [!hint]- Hint
> Lihat `SubscriberConstructor`. Ia dipanggil sekali untuk setiap handler, jadi tiga handler berarti tiga subscriber dan tiga consumer group. Setiap group punya antrean sendiri di `broker.go`. Email B-1 hanya menyumbat antrean milik group email.

### Step 3: Let Two Instances Share the Work

```bash
go test -run Step3 -v
```

Test menjalankan `NewApp` dua kali di atas broker yang sama, seperti dua pod dari service yang sama, lalu menerbitkan sepuluh booking.

```
dua instance memproses event yang sama: availability -20 (seharusnya -10), email 20, kabar ke channel manager 20
```

Setiap tamu menerima dua email, dan availability berkurang dua kali lipat. Dengan satu instance tidak ada yang salah, jadi bug ini baru muncul pada hari service di-scale.

**Task:** perbaiki `SubscriberConstructor`.

> [!hint]- Hint
> Perhatikan nama yang diberikan ke `broker.Subscriber`. Ia memuat `instanceID`, yang berbeda di setiap instance. Dua instance berakhir di dua group, dan setiap group menerima semua event. Nama group harus sama di semua instance.

### Step 4: Add a Handler for a New Event

```bash
go test -run Step4 -v
```

```
availability kembali ke 0 setelah BookingCancelled; belum ada handler untuk event ini: tidak terjadi dalam 2s
```

**Task:** tambahkan handler untuk `BookingCancelled` yang memanggil `deps.Availability.Increase`.

> [!hint]- Hint
> Satu `cqrs.NewEventHandler` lagi dengan parameter `*BookingCancelled`. Kamu tidak perlu menulis nama topic: `GenerateSubscribeTopic` menurunkannya dari nama struct menjadi `events.BookingCancelled`, dan `EventBus` menerbitkan ke topic yang sama.

Setelah itu jalankan semuanya:

```bash
go test -race -v
```

## Solution

> [!success]- Solution
> Dua keputusan menyelesaikan seluruh lab: satu handler per akibat, dan nama handler sebagai nama consumer group.
>
> ```go
> SubscriberConstructor: func(params cqrs.EventProcessorSubscriberConstructorParams) (message.Subscriber, error) {
> 	return broker.Subscriber(params.HandlerName), nil
> },
>
> err = processor.AddHandlers(
> 	cqrs.NewEventHandler("decrease-availability", func(ctx context.Context, event *BookingCreated) error {
> 		return deps.Availability.Decrease(ctx, event.RoomType, event.Rooms)
> 	}),
> 	cqrs.NewEventHandler("send-confirmation-email", func(ctx context.Context, event *BookingCreated) error {
> 		return deps.Emailer.SendConfirmation(ctx, event.BookingID, event.GuestEmail)
> 	}),
> 	cqrs.NewEventHandler("sync-channel-manager", func(ctx context.Context, event *BookingCreated) error {
> 		return deps.ChannelManager.PushAvailability(ctx, event.RoomType)
> 	}),
> 	cqrs.NewEventHandler("restore-availability", func(ctx context.Context, event *BookingCancelled) error {
> 		return deps.Availability.Increase(ctx, event.RoomType, event.Rooms)
> 	}),
> )
> ```
>
> Versi lengkap dengan error wrapping ada di `labs/08-events-with-cqrs/_solusi/app.go`.

## When This Is the Wrong Choice

**Handler independen salah untuk langkah yang bergantung satu sama lain.** Di lab ini email konfirmasi terkirim walau pengurangan availability terus gagal. Kalau email hanya boleh terkirim setelah kamar benar-benar tercatat, email bukan reaksi atas `BookingCreated`. Ia reaksi atas event yang diterbitkan setelah pencatatan berhasil, atau satu langkah dalam proses yang dikoordinasi. [[20 Lab - Sagas and Process Managers]] membahas yang kedua.

**Satu topic per jenis event menghilangkan urutan antar jenis.** `BookingCreated` dan `BookingCancelled` untuk booking yang sama tinggal di dua topic. Tidak ada yang menjamin pembatalan diproses setelah pembuatan. Kalau urutan itu penting, kedua event harus berada di topic dan partition yang sama; Watermill menyediakan `EventGroupProcessor` supaya beberapa handler bisa berbagi satu subscriber. Persoalan urutannya sendiri ada di [[13 Lab - Message Ordering]].

**Nama struct sebagai nama event mengikat kontrak ke kode.** Mengganti nama `BookingCreated` di Go mengubah nama event dan nama topic tanpa peringatan apa pun. Untuk event yang dibaca tim lain, nama dan versinya harus ditetapkan secara eksplisit. Lihat [[15 Lab - Versioned Events]].

**Event bus bukan untuk menyuruh.** "Kirim email ini" punya satu penerima yang dituju dan diharapkan dikerjakan. Itu command, bukan event.

**Untuk sistem kecil, ini berlebihan.** Kalau tiga akibat itu hidup di satu proses dan satu database, satu transaction dan tiga pemanggilan fungsi lebih sederhana dan lebih mudah dibuktikan benar. Fan-out lewat broker baru sepadan ketika akibat-akibat itu punya kecepatan, pemilik, atau cara gagal yang berbeda.

## What Is Still Broken

1. **Email B-2 tidak pernah terkirim di Step 2.** Group email masih tersumbat oleh B-1 selamanya. Handler lain selamat, tetapi handler email sendiri belum. Middleware `Retry` dan `Park` dari [[06 Lab - Errors]] belum dipasang di router ini.
2. **Setiap handler masih bisa menerima event yang sama dua kali.** Memecah handler memperkecil akibat redelivery, tidak menghilangkannya. Kalau `Decrease` berhasil tetapi ack hilang, kamar berkurang dua kali. Itu [[09 Lab - At-Least-Once Delivery]].
3. **`EventBus.Publish` terpisah dari penyimpanan booking.** Booking bisa tersimpan di database lalu proses mati sebelum event terbit, dan tidak ada handler yang pernah tahu. [[10 Lab - Outbox]] menutup lubang ini.
4. **Nama handler tidak boleh lagi diubah sembarangan.** Karena nama handler adalah nama consumer group, mengganti `send-confirmation-email` menjadi `email-sender` membuat group baru. Di Kafka, group baru mulai dari awal topic atau dari ujungnya, tergantung konfigurasi `auto.offset.reset`: semua email lama terkirim ulang, atau email yang terbit selama pergantian hilang.

## Reflection Questions

1. Di Step 1, kenapa availability berkurang enam dan bukan empat? Ikuti jalannya satu message dari percobaan pertama sampai ketiga.
2. Jelaskan dengan kalimatmu sendiri kenapa Step 2 lulus tanpa perubahan kode setelah Step 1.
3. Seorang rekan mengusulkan nama consumer group `pms-` diikuti hostname pod, "supaya gampang dilacak". Apa yang terjadi ketika service berjalan di tiga pod? Apa yang terjadi setiap kali pod diganti dengan yang baru?
4. `email-service` membaca topic email sebagai satu consumer group. Kalau besok ada service kedua yang ingin mencatat statistik dari topic yang sama, consumer group apa yang ia pakai, dan apa akibatnya kalau ia memakai group milik layanan email?
5. Dari tiga akibat `BookingCreated` di lab ini, mana yang menurutmu tidak boleh berjalan sebelum yang lain berhasil? Bagaimana kamu mengubah rancangannya?

**My answers:**

1.
2.
3.
4.
5.

## Read Next in the Vault

- [[CQRS]]
- [[Consumer Groups and Rebalancing]]
- [[Term - Fan-Out (Messaging)]]
- [[Queue vs Log Semantics]]
- [[Kafka]]
