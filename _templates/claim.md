---
type: claim
id: C-NNN
rq: RQ1
status: open
created: YYYY-MM-DD
evidence: []
---

## Klaim
<!-- satu kalimat. Kalau tidak mungkin salah, ini bukan klaim. -->

## Prediksi terukur
<!-- angka + arah. "lebih baik" tidak diterima. Contoh: recall@10 turun >15 poin
     pada delete_ratio 20% untuk pola clustered vs <5 poin untuk random. -->

## Kondisi yang MEMBANTAH klaim ini
<!-- observasi konkret yang, kalau terjadi, memaksa aku menolak klaim ini -->

## Baseline pembanding

## Kalau klaim ini salah, tesis apa yang tersisa?
<!-- WAJIB DIISI. Kosong = klaim ini belum siap dipakai. -->

## Bukti
```dataview
TABLE dataset, status FROM "02-experiments"
WHERE contains(claims, this.file.link)
```
