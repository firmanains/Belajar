# Diagram yang perlu digambar

Aturan: **kalau apa yang harus terlihat di gambar tidak bisa disebutkan dalam satu
kalimat, gambarnya belum perlu dibuat.** Gambar itu alat berpikir, bukan hiasan repo.

Gambar bagus adalah yang bisa kamu tunjukkan ke supervisor dan langsung dimengerti
tanpa penjelasan.

- [ ] **D-01 Struktur layer HNSW** — kenapa layer atas jarang dan layer bawah padat *(Fase 0)*
- [ ] **D-02 Greedy search satu query** — jejak hop dari entry point sampai konvergen *(Fase 0)*
- [ ] **D-03 Heuristic neighbor selection** — dua kandidat, satu dipangkas, kenapa *(Fase 2)*
- [ ] **D-04 Node dihapus merusak routing** — jalur sebelum vs sesudah, di level edge *(Fase 2)*
      → ini gambar pembuka proposalmu, kandidat terkuat
- [ ] **D-05 Reverse adjacency** — kenapa in-neighbor tidak diketahui tanpa menyimpannya *(Fase 2)*
- [ ] **D-06 Local edge repair** — edge mana yang dihitung ulang, edge mana yang tidak *(Fase 2)*
- [ ] **D-07 Fanout ke 8 shard, 3 degraded** — bagaimana top-k global jadi salah senyap *(Fase 3)*
- [ ] **D-08 Kurva recall vs delete_ratio, acak vs terklaster** — versi sketsa hipotesis,
      digambar SEBELUM eksperimen supaya kamu tahu apa yang kamu harapkan *(Fase 1)*

Simpan file `.excalidraw` di folder ini, satu catatan pendamping per gambar
memakai `_templates/excalidraw-note.md`.
