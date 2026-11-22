# Reading List — 14 bacaan minimum

> **Aturan: jangan pernah baca paper tanpa memegang angka yang ingin dijelaskan.**
> Kolom `trigger` wajib terisi sebelum status berpindah dari `belum`.

| # | Bacaan | Fase | Trigger | Status |
|---|---|---|---|---|
| 1 | Keshav — How to Read a Paper | 0 | prasyarat | belum |
| 2 | Malkov & Yashunin — HNSW | 0 | | belum |
| 3 | Aumüller et al. — ANN-Benchmarks | 0 | | belum |
| 4 | Jayaram Subramanya et al. — DiskANN | 1 | | belum |
| 5 | Singh et al. — FreshDiskANN | 1 | | belum |
| 6 | Wang et al. — Milvus (SIGMOD'21) | 1 | | belum |
| 7 | Luo & Carey — LSM survey (bagian compaction) | 1 | | belum |
| 8 | Jégou et al. — Product Quantization | 2 | | belum |
| 9 | Chen et al. — SPANN | 3 | | belum |
| 10 | Dean & Barroso — The Tail at Scale | 3 | | belum |
| 11 | Dokumen arsitektur Vespa | 3 | | belum |
| 12–14 | Filtered-ANN / index maintenance 2023–2026 | 4 | | belum |

FreshDiskANN adalah yang paling dekat dengan idemu. Tugasnya bukan mengagumi,
tapi menemukan apa yang **belum** mereka selesaikan.

## Catatan paper yang sudah ada
```dataview
TABLE year, venue, status, trigger FROM "04-papers"
SORT phase ASC
```

## Paper tanpa trigger — jangan dibaca
```dataview
LIST FROM "04-papers" WHERE !trigger OR trigger = ""
```
