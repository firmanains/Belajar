# Vault Riset — Vector Index Maintenance

Research instrument, bukan personal knowledge base.
Periode: 27 Juli – 31 Desember 2026. Pemilik: Firman.
Bahasa: Indonesia, istilah teknis tetap Inggris.

## Enam aturan yang tidak dinegosiasikan

1. **Jangan pernah baca paper tanpa memegang angka yang ingin dijelaskan.**
   Setiap catatan paper wajib punya field `trigger` yang menunjuk experiment atau claim.
   Tidak ada trigger = paper itu belum boleh dibaca.
2. **Experiment tanpa baseline masih engineering, bukan riset.** Kolom baseline kosong
   akan muncul otomatis di `00-dashboard/Now.md` di bawah judul "Masih engineering".
3. **Claim wajib bisa salah.** Section "kalau klaim ini salah, tesis apa yang tersisa"
   tidak boleh kosong. Claim tanpa kondisi pembantah = opini.
4. **Log mingguan wajib**, 1 jam, tidak boleh diskip. Baris "apa yang tidak aku mengerti"
   yang menentukan bacaan minggu depan.
5. **`07-concepts/` bukan tempat belajar, itu tempat rujukan.** Ditulis Sonnet, dibatasi
   daftar di `00-dashboard/Concepts.md`. Jangan generate di luar daftar.
   Belajar terjadi di `08-zettel/` dan di terminal.
6. **Zettel ditulis dengan sumber tertutup.** Zettel yang memparafrase concept note
   adalah sampah — hapus. Zettel harus berisi koneksi, keberatan, atau pertanyaan.

## Ritme mingguan

| Aktivitas | Jam |
|---|---|
| 1 paper, three-pass | 2–3 |
| Ngoding / eksperimen | 6–8 |
| Research log | 1 (wajib) |

## Yang disimpan di sini vs di repo kode

Vault menyimpan **interpretasi**: angka hasil, klaim, kebingungan, gambar.
Repo menyimpan **bukti**: kode, plot mentah, data. Vault merujuk repo lewat
path + commit SHA. Jangan salin plot ke sini kecuali dipakai di proposal.

## Struktur

```
00-dashboard/   Now, Progress, Claims, Reading, Concepts, Timeline, Scope
01-log/         YYYY-Www.md — satu per minggu
02-experiments/ E-YYYY-MM-DD-NN.md — satu per run
03-claims/      C-NNN-slug.md — satu per klaim yang bisa dibantah
04-papers/      satu per paper, wajib punya trigger
05-proposal/    8 section proposal
06-supervisors/ diisi Fase 4
07-concepts/    primer hasil generate, terbatas daftar
08-zettel/      catatan atomik tulisan sendiri
09-excalidraw/  diagram
_templates/     template Obsidian
```

Jangan tambah folder top-level baru.

## Plugin

Wajib: **Dataview**, **Templates** (core), **Excalidraw**.
Opsional: Calendar, Periodic Notes.
Setiap dashboard punya fallback manual di bawah query, jadi vault tetap terbaca
kalau Dataview mati.
