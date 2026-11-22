# Belajar

Catatan belajar pribadi tentang backend engineering, dalam satu vault Obsidian dan satu repo. Penjelasan ditulis dalam Bahasa Indonesia; istilah teknis tetap bahasa Inggris.

Sebagian besar catatan ditulis dengan bantuan AI dan belum diperiksa orang lain. Perlakukan sebagai bahan belajar, bukan rujukan resmi.

## Isi

| Folder | Isi | Mulai dari |
|---|---|---|
| `backend-engineering-vault/` | Konsep backend engineering dari junior sampai senior, dengan Go | [Read Me First](backend-engineering-vault/00%20Start%20Here/Read%20Me%20First.md) |
| `event-driven/` | 20 lab dan satu proyek akhir event-driven dengan Go, Kafka, dan PostgreSQL | [00 Start Here](event-driven/00%20Start%20Here.md) |
| `devops-kubernetes/` | Kubernetes dan CI/CD: arsitektur cluster dan tiga lab | [00 Mulai di Sini](devops-kubernetes/00%20Mulai%20di%20Sini.md) |
| `AWS/SAA/` | Catatan persiapan AWS Solutions Architect Associate | [Exam guide](AWS/SAA/_exam-guide.md) |
| `vault-vector-index-maintenance/` | Catatan riset tentang pemeliharaan vector index | [Now](vault-vector-index-maintenance/00-dashboard/Now.md) |

## Peta lintas topik

Satu topik biasanya punya catatan konsep, latihan, dan padanan di AWS. Tabel ini menghubungkan ketiganya.

| Topik | Konsep | Latihan | AWS dan lanjutan |
|---|---|---|---|
| Messaging dan event | [Queue vs Log Semantics](backend-engineering-vault/30%20APIs%20and%20Web/Queue%20vs%20Log%20Semantics.md) · [Transactional Outbox](backend-engineering-vault/30%20APIs%20and%20Web/The%20Transactional%20Outbox%20Pattern.md) · [Idempotency Keys](backend-engineering-vault/60%20Distributed%20Systems/Idempotency%20Keys.md) | [Lab 02 Message Broker](event-driven/02%20Lab%20-%20Message%20Broker.md) · [Lab 10 Outbox](event-driven/10%20Lab%20-%20Outbox.md) · [Proyek akhir](event-driven/21%20Project%20-%20Hotel%20Channel%20Sync.md) | [Decoupling (SQS, SNS)](AWS/SAA/decoupling/_index-decoupling.md) |
| Kegagalan dan ketahanan | [Dead Letter Queues](backend-engineering-vault/30%20APIs%20and%20Web/Dead%20Letter%20Queues.md) · [Disaster Recovery - RTO and RPO](backend-engineering-vault/60%20Distributed%20Systems/Disaster%20Recovery%20-%20RTO%20and%20RPO.md) | [Lab 19 Fault Tolerance](event-driven/19%20Lab%20-%20Fault%20Tolerance.md) · [Lab 20 Sagas](event-driven/20%20Lab%20-%20Sagas%20and%20Process%20Managers.md) | [Multi-Region](backend-engineering-vault/60%20Distributed%20Systems/Multi-Region%20Architecture%20and%20Geo-Replication.md) |
| Observability | [The Three Pillars of Observability](backend-engineering-vault/70%20Infrastructure%20and%20Delivery/The%20Three%20Pillars%20of%20Observability.md) · [Distributed Tracing](backend-engineering-vault/70%20Infrastructure%20and%20Delivery/Distributed%20Tracing.md) | [Lab 17 Metrics and Alerting](event-driven/17%20Lab%20-%20Metrics%20and%20Alerting.md) · [Lab 18 Tracing](event-driven/18%20Lab%20-%20Tracing.md) | [Monitoring and audit](AWS/SAA/monitoring/_index-monitoring-and-audit.md) |
| Kubernetes dan delivery | [Kubernetes Core Concepts](backend-engineering-vault/70%20Infrastructure%20and%20Delivery/Kubernetes%20Core%20Concepts%20-%20Pods%2C%20Deployments%2C%20Services%2C%20Ingress.md) · [CI-CD Pipelines](backend-engineering-vault/70%20Infrastructure%20and%20Delivery/CI-CD%20Pipelines.md) | [Arsitektur cluster](devops-kubernetes/01%20Arsitektur%20Cluster%20Kantor.md) · [Lab kind](devops-kubernetes/02%20Lab%201%20-%20Kubernetes%20di%20Laptop.md) · [Lab CI/CD](devops-kubernetes/04%20Lab%203%20-%20CI-CD%20Deploy%20dari%20main.md) | — |
| Jaringan | [The TCP-IP Model](backend-engineering-vault/10%20Foundations/The%20TCP-IP%20Model.md) · [DNS Resolution](backend-engineering-vault/10%20Foundations/DNS%20Resolution.md) | [Lab cluster di VM](devops-kubernetes/03%20Lab%202%20-%20Cluster%20seperti%20Kantor.md) | [VPC connectivity](AWS/SAA/vpc/_index-vpc-connectivity.md) |
| Keamanan | [RBAC](backend-engineering-vault/80%20Security/RBAC.md) · [Secret Management](backend-engineering-vault/80%20Security/Secret%20Management.md) · [Encryption at Rest vs In Transit](backend-engineering-vault/80%20Security/Encryption%20at%20Rest%20vs%20In%20Transit.md) | — | [Encryption and secrets](AWS/SAA/encryption/_index-encryption-and-secrets.md) · [Threat protection](AWS/SAA/threat-protection/_index-threat-protection.md) |

## Cara membuka

1. Clone repo ini.
2. Di Obsidian pilih **Open folder as vault**, lalu pilih folder hasil clone. Seluruh folder adalah satu vault, jadi tautan antar-folder dan graph view bekerja lintas topik.
3. Di perangkat mobile, clone sekali dengan aplikasi git pilihanmu, lalu buka foldernya sebagai vault. Satu pull membawa semua perubahan.

File `.obsidian/workspace*.json` tidak ikut repo supaya tidak bentrok antar-perangkat.

## Menjalankan lab

Lab di `event-driven/` butuh Go 1.26 dan Docker.

```bash
cd event-driven
make up       # PostgreSQL di port 55432 dan Kafka di port 59092
make verify   # kode awal gagal sesuai rancangan, jawaban lulus semua test
make down
```

Jawaban tiap lab ada di folder `_solusi/`. Coba dulu sebelum membukanya.

## Konvensi

- Tiap folder punya aturan penulisan sendiri di `CLAUDE.md`-nya; ikuti aturan folder tempat catatan itu berada.
- Repo ini publik. Jangan menulis nama perusahaan, alamat IP internal, nama akun, atau detail sistem tempat kerja. Pakai sebutan umum dan alamat contoh.
