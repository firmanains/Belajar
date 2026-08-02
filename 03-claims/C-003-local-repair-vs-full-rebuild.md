---
type: claim
id: C-003
rq: RQ2
status: open
created: 2026-08-02
evidence: []
---

## Klaim
**Local edge repair** — menjalankan ulang neighbor selection heuristic hanya untuk
in-neighbor dari node yang dihapus — mencapai recall dalam ambang **X%** dari full
rebuild, dengan CPU-second yang jauh lebih rendah.

> **X sengaja dikosongkan.** Belum ada satu pun pengukuran yang memberi hak
> mengisinya. Isi setelah Fase 2. Jangan tebak supaya rapi.

## Prediksi terukur
*(menunggu Fase 2)* — bentuknya: selisih recall@10 ≤ X poin, CPU-second ≤ Y% dari
full rebuild, pada delete_ratio 10–40%.

## Kondisi yang MEMBANTAH klaim ini
- Local repair menurunkan recall secara akumulatif: setiap siklus delete-repair
  meninggalkan kerusakan sisa, sehingga setelah beberapa siklus recall-nya sama
  buruknya dengan tombstone-only.
- Atau biaya repair naik superlinear terhadap delete_ratio sehingga pada 30%+
  full rebuild justru lebih murah.

## Baseline pembanding
tombstone-only · periodic full rebuild · FreshDiskANN (Singh et al.) sebagai
pembanding literatur.

## Kalau klaim ini salah, tesis apa yang tersisa?
Kalau local repair kalah, pertanyaannya jadi **kapan titik silangnya** — pada
delete_ratio berapa, dan pada pola delete apa, full rebuild mulai menang.
Peta titik silang itu sendiri adalah kontribusi untuk RQ2, karena sistem produksi
hari ini memilih policy berdasarkan tebakan, bukan berdasarkan peta.

## Bukti
```dataview
TABLE dataset, status FROM "02-experiments"
WHERE contains(claims, "C-003")
```
