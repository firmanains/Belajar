# Aturan zettel

Folder ini berisi **tulisanmu sendiri**. Claude Code tidak boleh menulis di sini.

## Tiga aturan

1. **Ditulis dengan sumber tertutup.** Tutup concept note dan paper-nya, baru tulis.
   Kalau tidak bisa menulisnya tanpa melihat, kamu belum paham — kembali ke sumber,
   jangan menyalin.
2. **Satu ide per catatan**, maksimal 15 baris.
3. **Harus salah satu dari tiga jenis** (isi field `kind`):
   - `koneksi` — menghubungkan dua hal yang sumbernya tidak menghubungkan
   - `keberatan` — kamu tidak setuju, atau melihat asumsi tersembunyi
   - `pertanyaan` — sesuatu yang belum terjawab, ditulis cukup presisi untuk diuji

## Yang harus dihapus
Zettel yang isinya ringkasan atau parafrase concept note. Itu bukan pemahaman,
itu penyalinan dengan langkah tambahan. Kalau ragu, tanya: apa yang kutambahkan
yang tidak ada di sumber? Kalau tidak ada jawabannya, hapus.

## Kenapa dipisah dari 07-concepts/
`07-concepts/` adalah tulisan mesin — rujukan, bisa salah, harus diverifikasi.
`08-zettel/` adalah tulisan kepalamu — inilah yang benar-benar kamu bawa ke wawancara
supervisor. Kalau keduanya bercampur, kamu tidak akan tahu mana yang benar-benar
kamu pahami.

## Indeks
```dataview
TABLE kind, created FROM "08-zettel" WHERE type = "zettel" SORT created DESC
```
