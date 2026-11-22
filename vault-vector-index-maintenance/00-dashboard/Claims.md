# Claims

Proposal bulan November ditulis dari halaman ini, bukan dari catatan paper.

## Per RQ
```dataview
TABLE status, created, length(evidence) AS bukti FROM "03-claims"
GROUP BY rq
```

## Per status
```dataview
TABLE rq, file.link FROM "03-claims"
GROUP BY status
```

*Fallback manual:*

| ID | RQ | Ringkas | Status |
|---|---|---|---|
| [[03-claims/C-001-clustered-delete-lebih-merusak\|C-001]] | RQ1 | delete terklaster jauh lebih merusak daripada acak | open |
| [[03-claims/C-002-proxy-signal-tanpa-ground-truth\|C-002]] | RQ1 | ada proxy signal yang berkorelasi kuat dengan recall | open |
| [[03-claims/C-003-local-repair-vs-full-rebuild\|C-003]] | RQ2 | local edge repair mendekati full rebuild dengan biaya jauh lebih murah | open |

## Aturan
- Claim tanpa kondisi pembantah = opini, bukan claim.
- Section "kalau salah, tesis apa yang tersisa" tidak boleh kosong.
- Claim yang `refuted` **jangan dihapus** — itu hasil, dan sering jadi bagian
  paling menarik di proposal.
