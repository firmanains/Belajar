---
type: experiment
id: E-YYYY-MM-DD-NN
date: YYYY-MM-DD
phase: 0
dataset: SIFT1M
index_impl: hnswlib
repo: ""
commit: ""
seed: 42
n_runs: 1
hardware: ""
claims: []
status: running
---

## Pertanyaan yang dijawab run ini
<!-- satu kalimat, harus bisa dijawab dengan angka -->

## Parameter

| Param | Nilai |
|---|---|
| M | |
| efConstruction | |
| ef | |
| delete_ratio | |
| delete_pattern | random / clustered / — |
| n_runs | |

## Hasil — HANYA ANGKA

| Metrik | Nilai | Std |
|---|---|---|
| recall@10 | | |
| p99 latency (ms) | | |
| QPS | | |
| CPU-second | | |

## Baseline pembanding
<!-- Kosongkan HANYA kalau memang belum ada. Kosong = ini masih engineering. -->
baseline: BELUM ADA

## Apa yang tidak sesuai ekspektasi
<!-- Ini yang jadi bahan log mingguan dan penentu paper berikutnya -->

## Threat to validity
- internal:
- external:
- construct:

## Artefak
- plot: `repo/path/plot.png`
- script: `repo/path/script.py`
