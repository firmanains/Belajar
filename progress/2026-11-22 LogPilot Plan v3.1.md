# 2026-11-22 LogPilot Plan v3.1

Kembali ke [[00 Progress Log]]

LogPilot adalah proyek pribadi: platform log ingestion dan alerting dengan Go, Kafka, ClickHouse, dan Kubernetes. Repo-nya masih private; entri ini mencatat keputusan rancangannya.

## Yang dikerjakan

Rencana proyek ditinjau ulang dan empat hal diputuskan.

- **Control plane pindah dari PHP ke Go.** Management API menjadi modul di satu service bersama Query API. Satu bahasa untuk semua service berarti satu pola untuk konfigurasi, logging, metrik, dan graceful shutdown.
- **Endpoint internal diberi port sendiri.** NetworkPolicy di Kubernetes bekerja per port, bukan per path, jadi endpoint internal yang berbagi port dengan endpoint publik tidak bisa dilindungi olehnya.
- **Perkakas platform dipasang satu per satu**: ingress, sertifikat, GitOps, monitoring, lalu operator untuk Kafka, ClickHouse, dan PostgreSQL, dan terakhir autoscaling dari consumer lag. Kalau ada yang rusak, penyebabnya perkakas terakhir.
- **Satu tulisan pendek per masalah sulit**, ditulis saat masalahnya selesai.

## Yang dipelajari

- Session token yang bisa dicabut cocok untuk endpoint management yang trafiknya kecil. Token berumur pendek yang diverifikasi tanpa database cocok untuk jalur baca yang ramai. Pertimbangannya terbalik, jadi keduanya dipakai.
- Consumer yang tertinggal bisa saja CPU-nya rendah karena sedang menunggu database. Sinyal autoscaling yang benar untuk consumer adalah lag, bukan CPU. Catatan terkait: [[Service Discovery]] dan [[Metrics - The RED and USE Methods]].

## Yang belum beres

- Semua ini masih rencana; kodenya baru sampai Ingestor dan consumer yang membaca pesan.
- Belum diputuskan apakah Ingestor mengecek API key lewat endpoint internal atau langsung ke database.

## Langkah berikutnya

Menambah trace context di Ingestor, lalu migrasi skema ClickHouse.
