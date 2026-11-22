# Now

**Fase aktif: 0 — Ukur sesuatu.** Gate berikutnya: **9 Agustus 2026**.

## Gate Fase 0
- [ ] Bisa jelaskan tradeoff recall–QPS ke orang lain **tanpa buka catatan**

## Deliverable Fase 0
- [ ] Unduh SIFT1M (korpus TEXMEX)
- [ ] Hitung ground truth top-100 brute force, catat berapa lama
- [ ] Pasang hnswlib, bangun index
- [ ] Plot recall@10 vs ef
- [ ] Plot recall@10 vs QPS
- [ ] Repo publik + README
- [ ] Concept track 1–3 sudah digenerate & dibaca

## Masih engineering, belum riset
> Experiment di bawah ini belum punya baseline pembanding.

```dataview
TABLE dataset, date FROM "02-experiments"
WHERE status != "invalid"
SORT date DESC
```
*Fallback manual: cek tiap catatan di `02-experiments/`, cari yang section "Baseline
pembanding"-nya masih berbunyi BELUM ADA.*

## Claim yang masih terbuka
```dataview
TABLE rq, created, length(evidence) AS bukti FROM "03-claims"
WHERE status = "open"
SORT id ASC
```
*Fallback: [[03-claims/C-001-clustered-delete-lebih-merusak|C-001]] ·
[[03-claims/C-002-proxy-signal-tanpa-ground-truth|C-002]] ·
[[03-claims/C-003-local-repair-vs-full-rebuild|C-003]]*

## Experiment 14 hari terakhir
```dataview
TABLE date, dataset, status FROM "02-experiments"
WHERE date >= date(today) - dur(14 days)
SORT date DESC
```

## Yang belum aku mengerti (dari log terakhir)
```dataview
TABLE week, phase FROM "01-log"
SORT week DESC
LIMIT 3
```
*Buka log terbaru, baca section "Apa yang tidak aku mengerti" — itu daftar bacaan
minggu depan.*

## Tugas terbuka di seluruh vault
```dataview
TASK FROM "" WHERE !completed AND !contains(file.folder, "_templates")
GROUP BY file.folder
```
