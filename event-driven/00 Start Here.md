# Event-Driven Go: Start Here

Dibuat: 2026-11-22

Kursus ini berisi 20 lab dan satu proyek akhir tentang event-driven architecture dengan Go. Semua contoh memakai satu domain: PMS hotel yang menyinkronkan booking dengan channel manager.

Kursus ini ditulis oleh Claude dari nol. Tidak ada orang berpengalaman yang sudah memeriksa materi ini. Kalau sebuah penjelasan terasa janggal, bandingkan dengan sumber di bagian akhir.

## How To Study

Teori yang lebih dalam ada di `../backend-engineering-vault`. Kursus ini melengkapinya dengan latihan yang bisa dijalankan. Setiap lab punya catatan (`NN Lab - ....md`) dan kode (`labs/NN-slug/`).

1. Baca bagian *Why This Exists* dan *Concepts* di catatan lab.
2. Jalankan test step demi step. Tebak hasilnya sebelum menekan Enter, lalu baca pesan gagalnya.
3. Ubah hanya file yang disebut catatan sampai test lulus. Coba minimal 15 menit sebelum membuka *Hint*, dan jangan buka `_solusi/` sebelum mencoba.
4. Jawab *Reflection Questions* dengan kata-katamu sendiri di catatan itu.
5. Centang lab di tabel bawah, lalu baca catatan vault yang disebut di *Read Next in the Vault*.

Kerjakan berurutan. Kode tiap lab berdiri sendiri, tetapi konsepnya bertumpuk.

## Setup

Go 1.26 dan Docker (OrbStack) sudah ada di Mac ini. Lab bertanda Postgres atau Kafka butuh infrastruktur lokal:

```bash
cd ~/Developer/belajar/event-driven
make up      # Postgres di port 55432, Kafka di port 59092
make down    # matikan dan hapus datanya
make verify  # cek semua lab: kode awal gagal sesuai rancangan, solusi lulus
```

Tidak ada lab yang menyentuh Kafka atau database kantor.

## Course Map

| Done | Lab | Topik | Infrastructure |
|---|---|---|---|
| [ ] | [[01 Lab - Basic Async]] | Dari panggilan sync ke background work, retry | none |
| [ ] | [[02 Lab - Message Broker]] | Kafka, publish/subscribe, consumer group, offset | Kafka |
| [ ] | [[03 Lab - Events]] | Merancang event, envelope, metadata, marshalling | none |
| [ ] | [[04 Lab - Watermill Basics]] | Router, handler, graceful shutdown, health check | none |
| [ ] | [[05 Lab - Middlewares]] | Correlation ID, logging, cross-cutting concerns | none |
| [ ] | [[06 Lab - Errors]] | Transient, malformed, bug; retry yang tidak menyumbat | none |
| [ ] | [[07 Lab - Component Testing]] | Menguji service sebagai black box | none |
| [ ] | [[08 Lab - Events with CQRS]] | Event bus, satu event banyak handler | none |
| [ ] | [[09 Lab - At-Least-Once Delivery]] | Redelivery, idempotent handler dan repository | none |
| [ ] | [[10 Lab - Outbox]] | Dual write, outbox dalam satu transaction, forwarder | Postgres |
| [ ] | [[11 Lab - Commands]] | Command vs event, command bus | none |
| [ ] | [[12 Lab - Read Models]] | Membangun tampilan baca dari event | Postgres |
| [ ] | [[13 Lab - Message Ordering]] | Partition key, entity version, pesan tidak berurutan | Kafka |
| [ ] | [[14 Lab - Data Lake]] | Menyimpan semua event selamanya | Postgres |
| [ ] | [[15 Lab - Versioned Events]] | Schema evolution, internal vs public event | none |
| [ ] | [[16 Lab - Migrating Read Models]] | Membangun ulang read model dari riwayat event | Postgres |
| [ ] | [[17 Lab - Metrics and Alerting]] | Metrik untuk message processing, alert rule | none |
| [ ] | [[18 Lab - Tracing]] | Trace context lintas pesan dengan OpenTelemetry | none |
| [ ] | [[19 Lab - Fault Tolerance]] | Timeout, backoff, circuit breaker, poison queue | none |
| [ ] | [[20 Lab - Sagas and Process Managers]] | Proses lintas service dengan compensation | none |
| [ ] | [[21 Project - Hotel Channel Sync]] | Proyek akhir: satu service utuh | Postgres + Kafka |

## What You Should Be Able To Do Afterwards

- [ ] Menjelaskan kenapa pesan bisa datang dua kali dan menulis consumer yang idempotent.
- [ ] Menjelaskan dual write dan menulis outbox beserta forwarder-nya.
- [ ] Menentukan partition key Kafka dan menangani pesan yang tidak berurutan.
- [ ] Membangun dan membangun ulang read model dari event.
- [ ] Membedakan transient error dari poison message dan menangani keduanya.
- [ ] Menjelaskan arti status `DEAD` di outbox billing di kantormu dan kapan aman mengulangnya.
- [ ] Merancang alur booking channel manager dari webhook sampai tape chart tanpa double booking.
- [ ] Memasang metrik, alert, dan tracing pada alur asynchronous.

## Further Reading

- Dokumentasi Watermill: https://watermill.io/
- *Designing Data-Intensive Applications* (Martin Kleppmann), bab tentang stream processing
