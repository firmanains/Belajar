---
type: claim
id: C-002
rq: RQ1
status: open
created: 2026-08-02
evidence: []
---

## Klaim
Kesehatan HNSW index dapat diestimasi **online tanpa ground truth** menggunakan
statistik yang sudah tersedia saat query dieksekusi. Kandidat: distribusi panjang
traversal, rasio tombstone yang dilewati, degree distribution, dan konvergensi hasil
antar dua nilai `ef` berbeda.

## Prediksi terukur
Minimal **satu** kandidat sinyal punya korelasi (Spearman) |rho| > 0.7 terhadap
recall@10 sebenarnya, konsisten di dua dataset dan di kedua pola delete.

## Kondisi yang MEMBANTAH klaim ini
- Semua kandidat sinyal |rho| < 0.4.
- Atau sinyalnya berkorelasi kuat di SIFT1M tapi runtuh di Deep10M — artinya
  sinyalnya menangkap properti dataset, bukan kesehatan index.
- Atau sinyalnya baru bergerak setelah recall sudah jatuh — terlambat, tidak berguna
  sebagai alarm.

## Baseline pembanding
- Naif: rasio tombstone mentah (jumlah tombstone / total node) tanpa informasi traversal.
- Upper bound: recall sebenarnya yang dihitung dari ground truth di lab.

Sinyal yang tidak mengalahkan baseline naif tidak punya nilai kontribusi.

## Kalau klaim ini salah, tesis apa yang tersisa?
Kalau semua sinyal lemah, tesisnya berubah jadi **hasil negatif yang berguna**:
menunjukkan bahwa informasi lokal per-query tidak cukup untuk menyimpulkan kualitas
hasil global, dan menjelaskan kenapa — misalnya karena panjang traversal didominasi
struktur layer, bukan kerusakan edge. Konsekuensi praktisnya: observability recall
harus lewat sampling ground truth periodik, dan pertanyaan risetnya bergeser ke
"berapa anggaran sampling minimum untuk mendeteksi degradasi X%".
Ini tetap satu tesis penuh.

## Bukti
```dataview
TABLE dataset, status FROM "02-experiments"
WHERE contains(claims, "C-002")
```
