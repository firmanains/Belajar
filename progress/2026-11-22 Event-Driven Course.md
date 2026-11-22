# 2026-11-22 Event-Driven Course

Kembali ke [[00 Progress Log]]

## Yang dikerjakan

Kursus latihan event-driven dengan Go selesai disusun di folder `event-driven/`: 20 lab dan satu proyek akhir, dengan PMS hotel dan channel manager sebagai domain. Titik mulainya [[00 Start Here]].

- Setiap lab punya catatan konsep, kode awal yang sengaja gagal di test, dan jawaban di folder `_solusi/`.
- Lab memakai Go, Kafka, dan PostgreSQL. Infrastruktur lokal dinyalakan dengan `make up`.
- Proyek akhir, [[21 Project - Hotel Channel Sync]], menggabungkan webhook, inbox idempotent, outbox, forwarder ke Kafka, read model, retry dan dead letter, serta metrik.
- Catatan lab menautkan ke catatan konsep di `backend-engineering-vault/`, misalnya [[The Transactional Outbox Pattern]] dan [[Idempotency Keys]].

## Cara memastikannya benar

`make verify` memeriksa setiap lab dua kali: kode awal harus lulus Step 0 dan gagal di step lain, dan kode awal ditambah jawaban harus lulus semua test dengan race detector. Ke-20 lab dan proyek akhir lulus.

Itu membuktikan latihannya bisa dikerjakan dan jawabannya konsisten dengan test. Itu tidak membuktikan penjelasan konsepnya benar.

## Yang belum beres

- Materi disusun dengan bantuan AI dan belum diperiksa engineer lain.
- Aturan alert di lab 17 belum dijalankan di Prometheus sungguhan.
- Belum ada lab yang saya kerjakan sendiri dari awal sampai lulus.

## Langkah berikutnya

Mengerjakan [[01 Lab - Basic Async]] dan menulis jawaban pertanyaan refleksinya.
