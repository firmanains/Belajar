---
type: zettel
id: Z-001
created: 2026-08-02
kind: koneksi
links: []
---

> Contoh format. Ganti isinya dengan tulisanmu sendiri setelah kamu benar-benar
> punya koneksi — jangan dipertahankan sebagai hiasan.

## Isi
Tombstone di HNSW dan tombstone di LSM-tree punya nama sama tapi biaya berbeda
jenisnya. Di LSM, tombstone membebani **pembacaan berurutan**: satu key mati harus
dilewati saat merge. Di HNSW, node mati tetap dipakai sebagai **jalur routing** —
jadi ia bukan cuma beban baca, ia ikut menentukan ke mana pencarian pergi.
Artinya intuisi "tombstone = ruang terbuang" yang benar di LSM justru menyesatkan
di sini. Kalau benar, maka biaya tombstone di HNSW tidak linear terhadap jumlahnya,
melainkan tergantung posisi node mati itu di graf.

## Kenapa ini bukan ringkasan
Tidak ada sumber yang menyandingkan keduanya; ini perbandingan yang kubuat sendiri,
dan ia melahirkan prediksi yang bisa diuji (posisi > jumlah).

## Terhubung ke
[[03-claims/C-001-clustered-delete-lebih-merusak]]
