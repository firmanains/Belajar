# Timeline 27 Jul – 31 Des 2026

| Fase | Periode | Deliverable | Gate |
|---|---|---|---|
| 0 — Ukur sesuatu | 27 Jul – **9 Agu** | repo + plot recall@10 vs ef, recall vs QPS (SIFT1M, hnswlib) | bisa jelaskan tradeoff recall–QPS tanpa catatan? |
| 1 — Masalah delete | 10 Agu – 6 Sep | kurva recall vs rasio delete (acak vs terklaster) + tulisan "apa yang mengejutkanku" | punya grafik yang bikin orang bertanya "kok bisa jatuh begitu?" |
| 2 — Low level | 7 Sep – 4 Okt | HNSW dari nol (Go/Rust) + reverse adjacency + local edge repair + tabel biaya | bisa jelaskan *kenapa* hapus node merusak recall, di level edge? |
| 3 — Distributed & sinyal | 5 Okt – 1 Nov | sharded fanout + eksperimen skew + plot korelasi proxy signal vs recall | punya kandidat sinyal, dan tahu mana yang gagal serta kenapa? |
| 4 — Proposal | 2 – 29 Nov | proposal 6–10 halaman + shortlist 6–8 supervisor + draft email | hipotesis bisa salah — dan tetap ada tesis kalau salah? |
| 5 — Poles & kirim | 30 Nov – 31 Des | repo rapi, proposal ditulis ulang, email terkirim, 2 minggu buffer | — |

**Kalau satu gate jawabannya "belum", jangan lanjut.** Lebih baik telat satu fase
daripada sampai Desember dengan lima fase setengah matang.
