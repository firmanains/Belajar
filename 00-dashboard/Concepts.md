# Concept Curriculum — 48 catatan, 8 track

> Daftar ini **tertutup**. Claude Code hanya mengisi item di sini.
> Usulan tambahan ditulis di bagian bawah, tidak langsung dibuat.
>
> **Urutan generate:** Track 1–3 dulu (prasyarat sebelum baca paper HNSW).
> Track 4–8 hanya saat fase-nya tiba atau saat dipicu experiment.
> Jangan generate semuanya sekaligus.

## Track 1 — Geometri & metrik `(wajib sebelum Fase 0 selesai)`
- [ ] T1-01 Vektor embedding sebagai titik di R^d, dan kenapa similarity jadi geometri
- [ ] T1-02 L2, cosine, dan inner product: definisi dan kapan masing-masing dipakai
- [ ] T1-03 Cosine ekuivalen L2 setelah normalisasi — turunkan identitasnya
- [ ] T1-04 Distance concentration: kenapa di dimensi tinggi semua titik terasa sama jauh
- [ ] T1-05 Contrast ratio sebagai ukuran kesulitan dataset
- [ ] T1-06 Intrinsic dimensionality vs ambient dimensionality
- [ ] T1-07 Kenapa ANN tetap bekerja meskipun curse of dimensionality nyata

## Track 2 — Problem search `(wajib sebelum Fase 0 selesai)`
- [ ] T2-01 Exact kNN dan biaya O(n·d): kenapa fatal di skala produksi
- [ ] T2-02 Definisi presisi recall@k, dan dua cara orang menghitungnya
- [ ] T2-03 Ground truth = brute force, dan konsekuensinya untuk observability
- [ ] T2-04 QPS, throughput, dan pengaruh batching
- [ ] T2-05 Latency percentile: p50, p95, p99, dan kenapa mean menipu
- [ ] T2-06 Kurva Pareto recall–QPS: plot standar bidang ini
- [ ] T2-07 Kenapa "95% recall" tanpa operating point tidak bermakna

## Track 3 — Peta index family `(wajib sebelum Fase 0 selesai)`
- [ ] T3-01 KD-tree dan ball tree, dan kenapa gagal di dimensi tinggi
- [ ] T3-02 Locality-Sensitive Hashing: ide dan alasan kalah di praktik
- [ ] T3-03 IVF: coarse quantizer, centroid, nprobe
- [ ] T3-04 Product Quantization: dekomposisi subspace, codebook, asymmetric distance
- [ ] T3-05 Scalar quantization dan tradeoff memori vs akurasi
- [ ] T3-06 Peta besar index family, dan di mana HNSW duduk

## Track 4 — Mekanika graph index `(Fase 1–2)`
- [ ] T4-01 Proximity graph: definisi dan invariant yang dijaga
- [ ] T4-02 Greedy search di graf, dan kapan ia terjebak local minimum
- [ ] T4-03 Navigable small world: long-range link dan biaya hop logaritmik
- [ ] T4-04 Layer assignment HNSW dan distribusinya
- [ ] T4-05 Parameter M, M_max, M_max0, efConstruction, ef — apa yang masing-masing kendalikan
- [ ] T4-06 Heuristic neighbor selection: kenapa memangkas edge justru menaikkan recall
- [ ] T4-07 Beam search dengan candidate heap dan result heap
- [ ] T4-08 Degree distribution, konektivitas, dan reachability sebagai kesehatan graf

## Track 5 — Delete & maintenance `(Fase 1–2, inti tesis)`
- [ ] T5-01 Tombstone / soft delete: mekanisme dan biayanya
- [ ] T5-02 Node sebagai routing hop, bukan sekadar data
- [ ] T5-03 In-neighbor dan reverse adjacency list: kenapa jarang disimpan
- [ ] T5-04 Local edge repair: menjalankan ulang neighbor selection untuk in-neighbor
- [ ] T5-05 LSM tree: memtable, SSTable, dan kenapa relevan ke masalahku
- [ ] T5-06 Compaction policy: leveled vs tiered, write/read/space amplification
- [ ] T5-07 Growing segment vs sealed segment (desain Milvus)

## Track 6 — Distributed `(Fase 3)`
- [ ] T6-01 Sharding vektor: partisi acak vs partisi berbasis cluster
- [ ] T6-02 Query fanout dan merge top-k global
- [ ] T6-03 Tail latency amplification: kenapa fanout bikin p99 jadi masalah struktural
- [ ] T6-04 SLO recall: bagaimana merumuskan target yang bisa diaudit

## Track 7 — Metodologi eksperimen `(Fase 0 & 4)`
- [ ] T7-01 Protokol ANN-Benchmarks dan apa yang dianggap curang
- [ ] T7-02 Varians, multiple run, dan confidence interval sederhana
- [ ] T7-03 Disiplin baseline dan ablation study
- [ ] T7-04 Taksonomi threat to validity: internal, external, construct, conclusion

## Track 8 — Tooling & data `(jalan sambil ngoding)`
- [ ] T8-01 Format .fvecs / .ivecs / .bvecs dan cara membacanya
- [ ] T8-02 SIFT1M dan Deep10M: isi, ukuran, kegunaan
- [ ] T8-03 hnswlib dan faiss: peta API dan bagian yang perlu dibaca
- [ ] T8-04 Memory layout, cache locality, dan SIMD distance kernel
- [ ] T8-05 mmap, dan kenapa mutasi graf saat search berjalan itu concurrency non-trivial

---

## Status generate
```dataview
TABLE track, status, confidence FROM "07-concepts"
SORT id ASC
```

## Usulan tambahan (Claude Code menulis di sini, tidak langsung membuat catatan)
