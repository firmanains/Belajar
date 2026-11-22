# Scope

> **Baca halaman ini setiap kali muncul ide baru.**
> Ide bagus yang di luar scope tetap harus ditolak. Tulis di future work.

## MASUK
- Satu keluarga index: **HNSW**
- Satu kelas workload: streaming insert + delete, read-heavy
- Dua dataset skala laptop/workstation: **SIFT1M**, **Deep10M**
- Shard **4–8**
- **Single region**

## KELUAR (tulis eksplisit di proposal — ini yang bikin reviewer percaya)
- storage engine
- query language
- auth
- multi-tenancy
- tuning quantization
- GPU
- protokol replikasi / konsistensi
- geo-distribution
- index family selain HNSW → future work
- Deep1B / skala miliar — hardware tidak cukup

## Istilah
Dipakai saat bicara ke akademisi: *high-dimensional similarity search*,
*vector index maintenance*, *approximate nearest neighbor search*.
Dihindari: *vector database* — terdengar seperti nama produk.

Komunitasnya **data management systems** (VLDB, SIGMOD, ICDE), bukan komunitas ML.
