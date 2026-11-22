---
type: claim
id: C-001
rq: RQ1
status: open
created: 2026-08-02
evidence: []
---

## Klaim
Pada rasio delete yang sama, pola delete **terklaster** (menghapus satu region di ruang
vektor) menurunkan recall@10 jauh lebih parah daripada pola delete **acak**, karena
delete terklaster memutus jalur routing di satu wilayah graf sekaligus, bukan
menyebarkan kerusakan tipis ke seluruh graf.

## Prediksi terukur
Pada `delete_ratio = 20%`, strategi tombstone-only, SIFT1M, `ef` tetap:
recall@10 turun **>15 poin** untuk pola terklaster, dan **<5 poin** untuk pola acak.
Selisih antar keduanya melebar seiring naiknya delete_ratio.

## Kondisi yang MEMBANTAH klaim ini
- Selisih recall antara pola terklaster dan acak <3 poin pada semua delete_ratio.
- Atau selisihnya terbalik: pola acak yang lebih merusak.
- Atau selisihnya ada tapi hilang begitu `ef` dinaikkan sedikit — artinya ini masalah
  parameter, bukan masalah topologi.

## Baseline pembanding
tombstone-only vs full rebuild, pada dua pola delete.

## Kalau klaim ini salah, tesis apa yang tersisa?
Kalau pola delete ternyata **tidak** penting, itu justru temuan yang kuat: artinya
policy compaction tidak perlu memodelkan distribusi spasial delete, dan cukup melihat
rasio agregat. Itu menyederhanakan RQ2 secara drastis dan layak ditulis — sekaligus
membantah asumsi implisit yang dipakai sistem yang men-trigger compaction per-segment.

## Bukti
```dataview
TABLE dataset, status FROM "02-experiments"
WHERE contains(claims, "C-001")
```
