# CLAUDE.md — instruksi untuk Claude Code di vault ini

## Konteks

Vault Obsidian untuk riset persiapan S2: **HNSW vector index maintenance** —
mendeteksi degradasi recall tanpa ground truth, dan memilih policy compaction
yang memenuhi SLO recall dengan biaya minimum.

Pemilik: Firman. Backend engineer (PHP/Yii1-2 + MariaDB, Go sekunder, Kafka,
Elasticsearch, Kubernetes). ~1 tahun produksi. **Nol pengalaman riset akademik.**
Bilingual ID–EN, minat ke low-level / server-level.

Tulis dalam **Bahasa Indonesia**, istilah teknis biarkan Inggris
(recall, tombstone, proximity graph, compaction, in-neighbor — jangan diterjemahkan).

## Tugas utamamu di vault ini

1. Meng-generate catatan di `07-concepts/` sesuai daftar di `00-dashboard/Concepts.md`.
2. Merapikan struktur/link, membuat catatan baru dari template.
3. **Bukan** tugasmu: menulis isi `08-zettel/`, `01-log/`, atau `03-claims/`.
   Itu tulisan tangan pemilik. Kalau diminta, tolak dan jelaskan kenapa.

## ATURAN ANTI-HALUSINASI — ini bagian terpenting file ini

Pemilik belum bisa mendeteksi kesalahan konsep di domain ini. Satu penjelasan
yang salah akan dipercaya berbulan-bulan dan muncul di proposal S2. Perlakukan
akurasi di atas kelengkapan dan di atas kelancaran prosa.

### Tiga jenis pernyataan, tandai bedanya

- **Definisi/matematika mapan** — tulis biasa, tanpa penanda. Contoh: definisi recall@k,
  identitas ||a-b||^2 = 2 - 2(a.b) untuk vektor ternormalisasi.
- **Klaim dari paper tertentu** — WAJIB sebut paper + penulis + tahun, dan tandai
  keyakinanmu: `[yakin]` / `[cukup yakin]` / `[⚠️ VERIFY]`.
- **Detail implementasi / konvensi tools** — tandai `[implementasi: hnswlib]` atau
  `[implementasi: faiss]`. Jangan sajikan sebagai kebenaran umum.

### Larangan keras

- **Jangan pernah mengarang sitasi.** Kalau ragu judul, penulis, venue, atau tahun,
  tulis `[⚠️ VERIFY: judul/tahun belum dipastikan]` — jangan tebak.
- **Jangan pernah menulis angka hasil benchmark dari ingatan.** Tidak ada
  "HNSW mencapai 98% recall pada SIFT1M". Angka hanya boleh masuk vault kalau
  berasal dari `02-experiments/` milik pemilik sendiri.
- **Jangan menghaluskan ketidaktahuan.** Kalau sebuah mekanisme tidak kamu pahami
  persis, tulis itu sebagai isi: "bagian ini aku tidak yakin, verifikasi di source X".
  Ketidakpastian yang jujur lebih berguna daripada paragraf mulus yang salah.
- **Jangan pakai analogi sebagai satu-satunya penjelasan.** Setiap konsep harus punya
  minimal satu dari: formula, pseudocode, atau invariant yang bisa dicek.
- **Kalau ada definisi yang bersaing di literatur, sebutkan keduanya.** Contoh:
  "recall@k" kadang dihitung per-query lalu dirata-rata, kadang agregat —
  sebutkan mana yang dipakai ANN-Benchmarks.
- **Jangan menambah catatan di luar daftar `00-dashboard/Concepts.md`.**
  Kalau menurutmu ada yang kurang, tulis usulannya di bagian "Usulan tambahan"
  di file itu, jangan langsung buat.

### Backstop wajib

Setiap catatan di `07-concepts/` HARUS diakhiri section
**"Cara memverifikasi ini sendiri"** berisi salah satu:
- potongan kode Python/Go yang bisa dijalankan dan menunjukkan klaimnya benar,
- pengukuran kecil yang bisa dilakukan di SIFT1M,
- atau lokasi persis di source code (`hnswlib`, `faiss`) untuk dicek — sebut nama
  fungsi, jangan nomor baris (nomor baris berubah antar versi).

Kalau sebuah konsep tidak bisa diberi cara verifikasi, itu tanda konsepnya terlalu
kabur — pecah atau buang.

## Gaya catatan concept

- Target **jelas dan low-level**, bukan pendek. Maksimal sekitar **230 baris**.
  Kalau lebih, tulis usulan pemecahan di "Usulan tambahan".
- Contoh format dan tingkat ketelitian: `07-concepts/T1-03-cosine-ekuivalen-l2-setelah-normalisasi.md`.
- Wajib ada:
  - **Istilah**: tabel `Istilah | Arti | Contoh` untuk setiap istilah yang dipakai
    di catatan, sebelum istilah itu dipakai di penjelasan.
  - **Gambaran**: minimal satu diagram Mermaid (`flowchart`, `sequenceDiagram`,
    atau `xychart-beta` untuk grafik) plus callout `> [!info] Cara membaca`.
    Angka di grafik harus hasil kode, bukan karangan.
  - **Penjelasan bertahap**: 4–7 langkah bernomor yang membangun pemahaman dari
    intuisi → mekanisme → low-level. Ini bagian utama; formula saja tidak cukup.
  - **Trace langkah demi langkah** (untuk algoritma atau struktur data): jalankan
    contoh kecil lewat algoritmanya, tunjukkan state setelah tiap langkah (isi heap,
    node yang dikunjungi, bucket, tabel lookup) sebagai tabel hasil kode. Tujuannya
    pemilik melihat apa yang terjadi "di balik layar", bukan hanya hasil akhirnya.
  - **Contoh angka kecil**: vektor/data mini (d = 2 atau 3, n ≤ 10) yang dihitung
    dengan kode, ditulis sebagai tabel.
  - **Sudut pandang low-level** bila relevan: loop, jumlah operasi, byte memori,
    struktur data. Pseudocode Go boleh; kode verifikasi tetap Python.
- Diagram harus tampil di Obsidian. Obsidian (≥1.15) merender Mermaid dengan Pele,
  bukan mermaid.js. Cek dengan merender lewat `lib/pele.min.js` milik Obsidian.
  Label langkah ditulis `"(1) ..."` dalam tanda kutip, supaya juga tampil di
  mermaid.js (GitHub). Hindari `www.` dan URL di label.
- Bahasa Indonesia, kalimat pendek, tanpa basa-basi pembuka.
- Frontmatter sesuai `_templates/concept.md`.
- Setiap catatan menjawab satu pertanyaan, judulnya adalah pertanyaan itu
  dalam bentuk pernyataan.
- Selalu isi field `prasyarat` dan `dipakai_di` (fase mana konsep ini terpakai).
- Wikilink ke konsep lain pakai `[[07-concepts/ID-slug]]`.

## Basic knowledge yang perlu digenerate

Daftar lengkap 48 catatan ada di `00-dashboard/Concepts.md`, terbagi 8 track:

1. **Geometri & metrik** — vektor di R^d, L2/cosine/inner product, ekuivalensi setelah
   normalisasi, distance concentration, contrast ratio, intrinsic vs ambient
   dimensionality, kenapa ANN tetap bekerja meski ada curse of dimensionality.
2. **Problem search** — exact kNN dan biaya O(n·d), definisi presisi recall@k,
   ground truth = brute force, QPS, p50/p99, kurva Pareto recall–QPS, kenapa angka
   recall tanpa operating point tidak bermakna.
3. **Peta index family** — KD-tree/ball tree dan kegagalannya di dimensi tinggi, LSH,
   IVF (coarse quantizer, nprobe), Product Quantization & Scalar Quantization,
   graph-based; posisi HNSW di peta.
4. **Mekanika graph index** — proximity graph, greedy search, navigable small world,
   layer assignment HNSW, parameter M / M_max / M_max0 / efConstruction / ef,
   heuristic neighbor selection dan alasan pemangkasannya, beam search,
   degree distribution, konektivitas & reachability.
5. **Delete & maintenance** — tombstone/soft delete, node sebagai routing hop,
   in-neighbor & reverse adjacency list, local edge repair, rebuild, LSM tree
   (memtable, SSTable, leveled vs tiered compaction, write/read/space amplification),
   growing vs sealed segment.
6. **Distributed** — sharding, query fanout, merge top-k global, tail latency
   amplification, hedged request, SLO recall.
7. **Metodologi eksperimen** — protokol ANN-Benchmarks, praktik yang dianggap curang,
   varians & multiple runs, confidence interval, disiplin baseline, ablation,
   taksonomi threat to validity.
8. **Tooling & data** — format .fvecs/.ivecs/.bvecs, SIFT1M & Deep10M, hnswlib, faiss,
   memory layout & cache locality, SIMD distance kernel, mmap, concurrency saat
   mutasi graf berbarengan dengan search.

**Urutan generate:** Track 1–3 dulu (prasyarat sebelum baca paper HNSW).
Track 4–8 hanya digenerate saat fase-nya tiba atau saat dipicu experiment.
Jangan generate semuanya sekaligus — itu mengubah vault jadi bahan bacaan pasif.

## Scope proyek — tolak kalau melebar

**Masuk:** HNSW saja; workload streaming insert+delete, read-heavy; SIFT1M & Deep10M;
shard 4–8; single region.

**Keluar:** storage engine, query language, auth, multi-tenancy, tuning quantization,
GPU, protokol replikasi/konsistensi, geo-distribution, index family selain HNSW.

Kalau pemilik minta sesuatu di luar daftar "masuk", ingatkan ke `00-dashboard/Scope.md`
sebelum mengerjakan.

## Git

Commit per flow, bukan per layer. Satu flow = template + dashboard/query + seed note.
Jangan gabungkan semua template dalam satu commit.
