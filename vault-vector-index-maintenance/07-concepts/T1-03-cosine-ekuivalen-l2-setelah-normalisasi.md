---
type: concept
id: T1-03
track: 1
judul: "Cosine ekuivalen L2 setelah normalisasi"
prasyarat: [T1-02]
dipakai_di: [fase-0]
status: contoh
sumber: []
confidence: yakin
---

> **Catatan untuk Claude Code:** ini catatan contoh. Tiru format dan tingkat
> ketelitiannya. Perhatikan bahwa setiap klaim punya cara verifikasi, dan tidak ada
> satu pun angka benchmark yang dikarang.

## Pernyataan inti
Untuk vektor yang sudah dinormalisasi ke panjang 1, mengurutkan tetangga berdasarkan
jarak L2 menaik menghasilkan urutan yang **identik** dengan mengurutkan berdasarkan
cosine similarity menurun. Karena itu index yang dibangun dengan metrik L2 di atas
data ternormalisasi menjawab query cosine dengan benar. Ini bukan pendekatan — ini
ekuivalensi eksak pada tingkat *ranking*. Nilai jaraknya tetap berbeda; yang sama
adalah urutannya.

## Mekanisme / formula
Untuk `a` dan `b` dengan `||a|| = ||b|| = 1`:

```
||a - b||^2 = ||a||^2 + ||b||^2 - 2(a . b)
            = 1 + 1 - 2(a . b)
            = 2 - 2 cos(a, b)
```

`||a-b||^2` adalah fungsi menurun monoton terhadap `cos(a,b)`. Fungsi menurun monoton
mempertahankan urutan (dibalik), jadi argmin L2 = argmax cosine.

Syarat yang mudah dilupakan: **kedua** vektor harus ternormalisasi. Menormalisasi
hanya sisi query tidak cukup.

## Kenapa ini penting untuk risetku
Menentukan apakah eksperimen delete-ku harus diulang untuk metrik cosine.
Kalau ekuivalensi ini berlaku pada setup-ku, satu metrik cukup — dan itu menghemat
setengah matriks eksperimen. Kalau tidak (misalnya data tidak ternormalisasi, atau
implementasi memakai inner product mentah pada vektor tak-ternormalisasi), maka
"metrik" jadi variabel bebas tambahan dan harus disebut di threat to validity.

SIFT descriptor **tidak** ternormalisasi secara default. Cek dulu sebelum berasumsi.

## Salah paham yang umum
- "Cosine dan L2 itu sama" — salah tanpa syarat normalisasi.
- "Nilai jaraknya sama" — salah. Yang sama urutannya, bukan nilainya. Threshold yang
  dikalibrasi untuk cosine tidak bisa langsung dipakai untuk L2.
- "Inner product = cosine" — hanya kalau ternormalisasi. Maximum inner product search
  pada vektor tak-ternormalisasi adalah masalah berbeda, dan proximity graph tidak
  otomatis benar untuk itu karena inner product bukan metrik (tidak memenuhi
  pertidaksamaan segitiga).

## Cara memverifikasi ini sendiri
```python
import numpy as np
rng = np.random.default_rng(0)
X = rng.normal(size=(2000, 128)); q = rng.normal(size=128)
Xn = X / np.linalg.norm(X, axis=1, keepdims=True)
qn = q / np.linalg.norm(q)

l2    = np.argsort(((Xn - qn) ** 2).sum(1))[:10]
cos   = np.argsort(-(Xn @ qn))[:10]
print(np.array_equal(l2, cos))          # True

# tanpa normalisasi:
l2_raw  = np.argsort(((X - q) ** 2).sum(1))[:10]
cos_raw = np.argsort(-(X @ q) / np.linalg.norm(X, axis=1))[:10]
print(np.array_equal(l2_raw, cos_raw))  # umumnya False
```
Jalankan juga pada SIFT1M asli untuk melihat apakah datanya sudah ternormalisasi.

## Rujukan
Identitas aljabar dasar, bukan klaim dari paper tertentu — tidak butuh sitasi.
Untuk konteks maximum inner product search sebagai masalah terpisah:
`[⚠️ VERIFY]` cari literatur MIPS sebelum mengutip apa pun.
