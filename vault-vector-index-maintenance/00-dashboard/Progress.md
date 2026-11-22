# Progress Tracker

Centang setelah selesai. Jangan centang "hampir".

## Prasyarat (weekend pertama, jangan lebih)
- [ ] Baca Keshav — How to Read a Paper (3 halaman)
- [ ] Paham norma vektor, cosine vs L2, kapan ekuivalen
- [ ] Paham intuisi curse of dimensionality + intrinsic dimensionality
- [ ] Paham mean vs p99, kenapa satu run tidak cukup

## Fase 0 — Ukur sesuatu (27 Jul – 9 Agu)
- [ ] Concept track 1 selesai digenerate & dibaca
- [ ] Concept track 2 selesai digenerate & dibaca
- [ ] Concept track 3 selesai digenerate & dibaca
- [ ] SIFT1M terunduh, format .fvecs bisa dibaca sendiri
- [ ] Ground truth top-100 brute force selesai + waktunya dicatat
- [ ] hnswlib terpasang, index terbangun
- [ ] Plot recall@10 vs ef
- [ ] Plot recall@10 vs QPS
- [ ] Bentuk kurva ANN-Benchmarks tereproduksi
- [ ] Repo publik + README + cara reproduksi
- [ ] Paper: HNSW (Malkov & Yashunin) three-pass
- [ ] Paper: ANN-Benchmarks (Aumüller et al.) three-pass
- [ ] **GATE:** bisa jelaskan tradeoff recall–QPS tanpa catatan

## Fase 1 — Masalah delete (10 Agu – 6 Sep)
- [ ] Delete bertahap 5/10/20/30/40%, tombstone-only
- [ ] Delete bertahap 5/10/20/30/40%, full rebuild
- [ ] Inflasi latency saat tombstone menumpuk (ef tetap)
- [ ] Pola delete acak vs terklaster
- [ ] Paper: DiskANN
- [ ] Paper: FreshDiskANN — cari apa yang BELUM diselesaikan
- [ ] Paper: Milvus SIGMOD'21 (growing vs sealed segment)
- [ ] Paper: Luo & Carey LSM survey, bagian compaction policy
- [ ] Concept track 4 & 5 digenerate sesuai kebutuhan
- [ ] Tulisan 2 halaman "apa yang mengejutkanku"
- [ ] **GATE:** punya grafik yang bikin orang bertanya "kok bisa jatuh begitu?"

## Fase 2 — Low level (7 Sep – 4 Okt)
- [ ] Baca source hnswlib sampai habis (perhatikan getNeighborsByHeuristic2)
- [ ] HNSW minimal dari nol: insertion
- [ ] neighbor selection heuristic
- [ ] search
- [ ] layer assignment
- [ ] Validasi recall vs hnswlib pada dataset sama
- [ ] Reverse adjacency list
- [ ] Delete dengan local edge repair
- [ ] Tabel biaya: repair lokal vs full rebuild (recall + CPU-second)
- [ ] Paper: Product Quantization (Jégou et al.)
- [ ] **GATE:** bisa jelaskan KENAPA hapus node merusak recall, di level edge

## Fase 3 — Distributed & sinyal (5 Okt – 1 Nov)
- [ ] Shard index ke N proses
- [ ] Fanout + merge top-k global
- [ ] Eksperimen skew: degradasi 3 dari 8 shard
- [ ] Ukur error top-k global
- [ ] Rekam sinyal: panjang traversal
- [ ] Rekam sinyal: rasio tombstone yang dilewati
- [ ] Rekam sinyal: degree distribution
- [ ] Rekam sinyal: konvergensi hasil antar dua nilai ef
- [ ] Plot korelasi tiap sinyal vs recall sebenarnya
- [ ] Paper: SPANN
- [ ] Paper: The Tail at Scale
- [ ] Dokumen arsitektur Vespa
- [ ] Concept track 6 digenerate
- [ ] **GATE:** punya kandidat sinyal, dan tahu mana yang gagal serta kenapa

## Fase 4 — Proposal (2 – 29 Nov)
- [ ] 6–8 paper 2023–2026 (filtered-ANN / index maintenance), catat lab & supervisor
- [ ] Draft 8 section proposal
- [ ] Concept track 7 digenerate
- [ ] Shortlist 6–8 supervisor
- [ ] Draft email perkenalan
- [ ] **GATE:** hipotesis bisa salah — dan tetap ada tesis kalau salah

## Fase 5 — Poles & kirim (30 Nov – 31 Des)
- [ ] Proposal ditinggal 1 minggu penuh tanpa dibuka
- [ ] Proposal ditulis ulang
- [ ] Repo dirapikan: README, cara reproduksi, semua plot
- [ ] Email terkirim (lampirkan SATU plot, bukan seluruh proposal)
- [ ] 2 minggu buffer tersisa
