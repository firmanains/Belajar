# GUIDANCE — cara pakai vault ini sehari-hari

README = aturan. CLAUDE.md = instruksi mesin. **File ini = ke mana aku menaruh apa.**

---

## 1. Aku mau nulis sesuatu — taruh di mana?

| Yang mau kutulis | Folder | Kenapa di situ |
|---|---|---|
| Angka hasil run | `02-experiments/` | satu catatan = satu run |
| Ide yang bisa salah | `03-claims/` | ini yang jadi proposal |
| Apa yang aku pahami (kalimat sendiri) | `08-zettel/` | pemahaman, bukan rujukan |
| Apa yang ingin aku tahu | `01-log/` → *"Apa yang tidak aku mengerti"* | ini penentu bacaan minggu depan |
| Isi & struktur sebuah paper | `04-papers/` | catatan three-pass |
| Definisi / mekanisme baku | `07-concepts/` | ditulis Sonnet, aku hanya baca & koreksi |
| Gambar | `09-excalidraw/` | + catatan pendamping |
| Kalimat untuk proposal | `05-proposal/` | 8 section |
| Calon pembimbing | `06-supervisors/` | Fase 4 |
| PDF, dataset kecil, gambar mentah | `_attachments/` | bukan catatan, cuma file |

**Aturan pemisah yang paling sering dilanggar orang:**
`04-papers/` = *apa kata paper itu*. `08-zettel/` = *apa kata aku*.
Kalau kalimatnya masih bisa dilacak ke paragraf tertentu di paper, ia milik `04-papers/`.

---

## 2. "Apa yang sudah aku pahami" → zettel

Satu ide, ≤15 baris, **sumber ditutup saat menulis**. Ambil dari `_templates/zettel.md`.

```markdown
---
type: zettel
id: Z-002
created: 2026-08-09
kind: koneksi        # koneksi | keberatan | pertanyaan
links: ["[[04-papers/malkov-yashunin-hnsw]]", "[[03-claims/C-001-clustered-delete-lebih-merusak]]"]
---

## Isi
Layer atas HNSW berfungsi seperti index sparse di B-tree: ...

## Kenapa ini bukan ringkasan
Malkov tidak menyebut B-tree; perbandingan ini punyaku, dan ia memprediksi ...

## Terhubung ke
[[07-concepts/T4-04-layer-assignment-hnsw]]
```

Kalau section kedua tidak bisa diisi, catatan itu ringkasan — hapus.

---

## 3. "Apa yang ingin aku tahu" → log, bukan catatan sendiri

Jangan bikin folder `questions/`. Pertanyaan hidup di dua tempat saja:

- **Belum matang** → `01-log/`, section *"Apa yang tidak aku mengerti"*.
- **Sudah cukup presisi untuk diuji** → naikkan jadi zettel `kind: pertanyaan`,
  atau langsung jadi `03-claims/`.

Alurnya satu arah: **bingung → pertanyaan → claim → experiment → bukti**.
Pertanyaan di log itulah yang mengisi field `trigger` di catatan paper.

---

## 4. PDF paper — taruh di `_attachments/papers/`

```
_attachments/papers/2018-malkov-hnsw.pdf
```

Konvensi nama: `{tahun}-{penulis-pertama}-{slug}.pdf`.

Lalu tautkan dari catatan papernya (`04-papers/malkov-yashunin-hnsw.md`):

```markdown
pdf: [[_attachments/papers/2018-malkov-hnsw.pdf]]
```

Klik link itu, PDF terbuka di dalam Obsidian dan bisa dianotasi.

**PDF tidak masuk git** — sudah kutambahkan ke `.gitignore`. Alasannya dua: hak cipta,
dan repo yang membengkak. Kalau vault ini nanti dipublikasikan, yang terlihat cuma
catatanmu, bukan paper orang.

---

## 5. Menulis pemahaman dari sebuah paper

Dua langkah, jangan dicampur.

**Langkah 1 — `04-papers/{penulis}-{slug}.md`.** Pakai `_templates/paper.md`.
Isi `## Angka yang aku bawa ke paper ini` **sebelum** membaca. Kalau kosong, jangan baca.
Bagian paling berharga adalah `## Apa yang mereka TIDAK selesaikan` — di situ letak
celah kontribusimu.

**Langkah 2 — tutup PDF-nya, tunggu sehari, tulis zettel.** Kalau tidak bisa menulis
apa pun tanpa membuka paper, kamu belum paham; baca ulang pass 2, jangan menyalin.

---

## 6. Menyambungkan semuanya

Tiga mekanisme, itu saja:

| Mekanisme | Untuk apa |
|---|---|
| **Wikilink** `[[...]]` | koneksi bebas antar catatan |
| **Frontmatter** (`claims:`, `trigger:`, `evidence:`) | koneksi terstruktur, dibaca Dataview |
| **Dataview query** di `00-dashboard/` | koneksi terkumpul otomatis |

Yang wajib diisi supaya dashboard tidak kosong:

- experiment → `claims: ["C-001"]`
- paper → `trigger: "[[02-experiments/E-2026-08-02-01]]"`
- claim → `evidence: ["[[02-experiments/E-2026-08-02-01]]"]`
- zettel → `links: [...]`

Rutinitas 30 detik tiap selesai menulis: **catatan ini menempel ke mana?**
Catatan tanpa satu pun link akan hilang dalam dua minggu.

---

## 7. Excalidraw — cara menyambungkannya

**Setup sekali:** Settings → Excalidraw → *Excalidraw folder* = `09-excalidraw`.

**Bikin gambar:** Command palette → `Excalidraw: Create new drawing`.
Nama file pakai ID dari `09-excalidraw/README-daftar-gambar.md`, misal `D-04-delete-merusak-routing`.

**Sisipkan ke catatan mana pun** — embed biasa, jalan di semua markdown:

```markdown
![[D-04-delete-merusak-routing]]
```

Ukur lebarnya: `![[D-04-delete-merusak-routing|600]]`.

**Arah sebaliknya — dari gambar ke catatan.** Ini yang bikin gambar jadi bagian vault,
bukan lampiran. Di dalam kanvas Excalidraw:

- Ketik teks → seleksi → `Ctrl/Cmd+K` → jadikan link ke `[[03-claims/C-001-...]]`.
  Elemen itu jadi bisa diklik dan melompat ke catatan.
- Atau embed catatan sebagai kartu di kanvas: `Excalidraw: Insert markdown file`.

**Kaitkan lewat frontmatter juga**, supaya muncul di query. Catatan pendamping tiap
gambar pakai `_templates/excalidraw-note.md`:

```yaml
---
type: drawing
judul: "Node dihapus merusak routing"
menjelaskan: ["[[03-claims/C-001-clustered-delete-lebih-merusak]]", "[[07-concepts/T5-02-node-sebagai-routing-hop]]"]
fase: 2
---
```

Di catatan yang dijelaskan, embed balik gambarnya. Dua arah = tersambung.

**Kapan menggambar:** kalau apa yang harus terlihat tidak bisa disebut dalam satu
kalimat, gambarnya belum perlu. Gambar itu alat berpikir, bukan hiasan repo.
Satu pengecualian yang justru paling berguna: **D-08**, sketsa kurva hipotesis
yang digambar *sebelum* eksperimen — supaya kamu punya bukti tertulis tentang apa
yang kamu harapkan, dan bisa terkejut secara jujur.

---

## 8. Alur satu minggu

1. **Senin** — buka `00-dashboard/Now.md`, lihat gate dan claim terbuka.
2. **Senin–Kamis** — ngoding. Tiap run → catatan `02-experiments/`, isi `claims:`.
3. **Jumat** — baca 1 paper. Isi `trigger` dulu; kalau tidak ada, jangan baca.
4. **Sabtu** — tulis zettel dengan sumber tertutup. Gambar kalau perlu.
5. **Minggu** — tulis log. Centang `00-dashboard/Progress.md`.
   Isi *"apa yang tidak aku mengerti"* → itu paper minggu depan.

---

## 9. Tiga tanda vault ini mulai busuk

- `02-experiments/` menumpuk, `03-claims/` tidak bertambah → kamu sedang ngoding, bukan riset.
- `04-papers/` bertambah lebih cepat dari `02-experiments/` → kamu sedang menghindari terminal.
- `07-concepts/` penuh, `08-zettel/` kosong → kamu mengumpulkan, bukan memahami.

Semuanya terbaca dari `00-dashboard/Progress.md`. Cek sekali sebulan.
