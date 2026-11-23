---
title: IAM fundamentals
aliases: [IAM, AWS Identity and Access Management, IAM policy]
tags: [aws, saa-c03, domain/secure, service/iam]
exam: SAA-C03
exam_domains: ["Domain 1: Design Secure Architectures"]
verified_on: 2026-11-08
status: verified
---

# IAM fundamentals

> [!summary] TL;DR
> - IAM (AWS Identity and Access Management) menentukan **siapa** boleh melakukan **apa** pada **resource mana** di AWS. IAM tidak dikenai biaya tambahan.[^2]
> - Secara default semua request ditolak (*implicit deny*). Request baru diizinkan kalau ada policy yang mengizinkannya secara eksplisit. *Explicit deny* selalu menang atas Allow.[^2][^8]
> - IAM user punya *long-term credentials*. IAM role tidak punya. Siapa pun yang memakai (*assume*) role mendapat *temporary credentials*.[^6]
> - Aplikasi (EC2, Lambda) diberi role, bukan access key. Karyawan masuk lewat *federation* dengan temporary credentials.[^5]
> - SCP, permissions boundary, dan session policy hanya **membatasi** izin. Ketiganya tidak pernah memberi izin.[^7]
>
> Istilah yang dicetak miring dijelaskan di bagian berikutnya.

## Istilah penting

Baca tabel ini dulu, atau kembali ke sini kalau ketemu istilah yang belum jelas.

| Istilah | Artinya | Contoh |
|---|---|---|
| **AWS account** | Wadah untuk resource, tagihan, dan identitas IAM milikmu. Setiap account punya ID berupa angka. | `123456789012` |
| **Resource** | "Barang" yang kamu buat di AWS dan bisa diatur izinnya. | EC2 instance, S3 bucket, tabel DynamoDB |
| **Action** | Satu operasi API milik sebuah service, ditulis `service:NamaOperasi`. | `s3:GetObject` (baca objek), `ec2:TerminateInstances` (hapus server) |
| **Request** | Satu panggilan ke AWS API, dari console, CLI, SDK, atau service lain. Setiap request membawa action dan resource tujuan. | "panggil `s3:GetObject` untuk file `inv-001.pdf`" |
| **Principal** | Pihak yang mengirim request ke AWS.[^6] | root user, IAM user, role, service AWS |
| **Identity** | Objek di IAM yang bisa ditempeli policy: user, group, atau role.[^7] Group tidak bisa jadi principal; group hanya cara mengumpulkan user untuk memberi izin.[^12] | user `budi`, group `developers` |
| **Credentials** | Bukti identitas yang dikirim bersama request. | password (login console), access key (CLI/SDK) |
| **Long-term credentials** | Credentials yang berlaku terus sampai kamu hapus atau ganti sendiri.[^11] | password dan access key IAM user |
| **Temporary credentials** | Credentials yang kedaluwarsa sendiri setelah waktu tertentu.[^6] | credentials dari role session |
| **Access key** | Pasangan *access key ID* dan *secret access key* untuk memanggil API lewat CLI atau SDK.[^11] | `AKIAIOSFODNN7EXAMPLE` + secret-nya |
| **MFA** | Multi-factor authentication: login butuh faktor kedua selain password. | kode dari aplikasi authenticator di HP |
| **Policy** | Dokumen berisi aturan izin. Kebanyakan policy disimpan dalam format JSON.[^7] | lihat contoh JSON di bagian 3 |
| **Statement** | Satu aturan di dalam policy, terdiri dari `Effect`, `Action`, `Resource`, dan `Condition` (opsional).[^7] | "Allow `s3:GetObject` ke bucket X" |
| **ARN** | Amazon Resource Name: nama unik sebuah resource di AWS. Dipakai di policy untuk menunjuk resource.[^10] | `arn:aws:s3:::shop-invoices` |
| **Federation** | Login memakai identitas dari sistem lain (bukan IAM user), lalu mendapat temporary credentials ke AWS. | karyawan login dengan akun kantornya |
| **Identity provider (IdP)** | Sistem yang menyimpan daftar pengguna dan menangani login. Dipakai untuk federation. | direktori karyawan perusahaan |
| **STS** | AWS Security Token Service: service yang mengeluarkan temporary credentials. Detailnya di [[iam-advanced]]. | panggilan `AssumeRole` |
| **Assume role** | Memakai sebuah role: kamu mendapat temporary credentials milik role itu.[^6] | developer assume role `s3-reader` |
| **Role session** | Masa berlaku temporary credentials setelah assume role. Durasinya terbatas (lihat tabel fakta). | sesi 1 jam |
| **Least privilege** | Prinsip memberi izin sekecil mungkin yang masih cukup untuk tugasnya.[^5] | aplikasi laporan hanya boleh baca, tidak boleh hapus |
| **AWS Organizations, OU** | Organizations mengelola banyak account sekaligus. OU (organizational unit) adalah kelompok account di dalam organization, mirip folder. Detailnya di [[iam-advanced]]. | OU `Production` berisi account-account produksi |
| **Implicit deny** | Ditolak karena **tidak ada** policy yang mengizinkan. Ini keputusan default.[^8] | user baru tanpa policy apa pun |
| **Explicit deny** | Ditolak karena **ada** statement `"Effect": "Deny"` yang cocok dengan request.[^8] | policy "Deny `ec2:TerminateInstances`" |

## Kenapa IAM ada

Satu AWS account bisa berisi ratusan resource: server, database, bucket berisi data pelanggan. Tanpa sistem izin, siapa pun yang bisa login bisa mengubah atau menghapus semuanya. IAM membuat kamu bisa menulis aturan seperti "aplikasi laporan boleh membaca bucket `shop-invoices`, tapi tidak boleh menghapus apa pun".

Di server on-prem, pekerjaan ini tersebar di beberapa tempat: user Linux dan permission file, `GRANT` di MariaDB, dan direktori karyawan untuk login. Di AWS, izin untuk memanggil API service AWS diatur lewat IAM policy. Exam guide memasukkan topik ini ke task statement 1.1 "Design secure access to AWS resources".[^1]

## Gambaran besar

Setiap request ke AWS melewati tiga langkah yang sama.

```mermaid
flowchart LR
  p["Principal<br>contoh: aplikasi laporan"]
  api["AWS API<br>contoh: s3:GetObject"]
  iam{"AWS mengevaluasi<br>semua policy terkait"}
  ok["Allow<br>request dijalankan"]
  no["Deny<br>request ditolak"]
  p -->|"(1) request + credentials"| api
  api -->|"(2) authenticate + request context"| iam
  iam -->|"(3a) diizinkan"| ok
  iam -->|"(3b) tidak diizinkan"| no
```

> [!info] Cara membaca
> Ikuti nomor di panah. Langkah 2 menjawab "kamu siapa?", langkah 3 menjawab "kamu boleh atau tidak?". Dua pertanyaan ini terpisah.

1. Principal mengirim request bersama credentials-nya. Contoh: aplikasi laporan memanggil `s3:GetObject` untuk membaca satu file invoice.
2. AWS memeriksa credentials (*authentication*: memastikan pengirim memang identitas yang dia klaim). Lalu AWS mengumpulkan *request context*, yaitu informasi tentang request: siapa principal-nya, action apa, dan resource mana. Dari situ AWS tahu policy mana saja yang berlaku.[^4]
3. AWS mengevaluasi policy-policy itu (*authorization*: memutuskan boleh atau tidak), lalu memberi keputusan Allow atau Deny.[^4][^8] Urutan evaluasinya dijelaskan di bagian 4.

Kalau kamu biasa memakai MariaDB, IAM policy mirip perintah `GRANT`:

```sql
-- MariaDB: user 'report' dari jaringan 10.0.x.x boleh SELECT tabel shop.orders
GRANT SELECT ON shop.orders TO 'report'@'10.0.%';
```

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::shop-invoices/*"
    }
  ]
}
```

| Bagian | MariaDB `GRANT` | IAM policy |
|---|---|---|
| Boleh melakukan apa | `SELECT` | `Action`: `s3:GetObject` |
| Pada objek mana | `shop.orders` | `Resource`: ARN bucket `shop-invoices` |
| Untuk siapa | `'report'@'10.0.%'` | identity tempat policy ditempel, atau elemen `Principal` di resource-based policy |

**Di mana analogi ini tidak cocok:** privilege MariaDB hanya menambah izin; tidak ada aturan "tolak" yang mengalahkan GRANT lain. IAM punya explicit `Deny` yang menang atas Allow mana pun, plus lapisan policy (SCP, permissions boundary) yang hanya membatasi.[^2][^7]

## 1. Identitas: siapa yang bisa minta akses

Semua identitas IAM hidup di dalam satu AWS account. Diagram ini menunjukkan isi sebuah account dan siapa saja dari luar yang bisa masuk.

```mermaid
flowchart TB
  subgraph org["AWS Organizations (opsional)"]
    scp["SCP dan RCP<br>pembatas untuk account"]
    subgraph acct["AWS account 123456789012"]
      root["Root user<br>akses penuh secara default"]
      subgraph grp["Group: developers"]
        u1["IAM user: budi"]
        u2["IAM user: sari"]
      end
      role["IAM role: app-s3-reader"]
    end
  end
  ec2["EC2 instance<br>aplikasi laporan"]
  idp["Identity provider<br>direktori karyawan"]
  scp -.->|"membatasi"| acct
  ec2 -->|"assume role"| role
  idp -->|"federation"| role
```

> [!info] Cara membaca
> User dan group ada di dalam account. Role juga ada di dalam account, tapi dipakai oleh pihak lain: EC2 instance, karyawan lewat federation, atau account lain. Organizations ada di level atas account.

Simplified: service-linked role, identitas di account lain, dan policy yang ditempel ke tiap identitas tidak digambar.

**Root user** adalah identitas account itu sendiri, dibuat saat account dibuat. Secara default root user punya akses penuh ke account.[^8] Kamu tidak bisa menempelkan identity-based policy atau permissions boundary ke root user.[^7] Karena aksesnya sebesar itu, aktifkan MFA untuk root user[^5] dan jangan membuat access key untuk root user.[^11]

**IAM user** adalah identitas untuk satu orang atau satu aplikasi, dengan long-term credentials: password untuk login console, access key untuk CLI/SDK, atau keduanya.[^2][^11] AWS menyarankan IAM user hanya untuk kasus yang tidak bisa memakai federation.[^6]

**IAM group** adalah kumpulan IAM user. Policy yang ditempel ke group berlaku untuk semua user di dalamnya.[^7] Contoh: group `developers` diberi policy baca S3, maka Budi dan Sari otomatis mendapat izin itu. Group hanya berisi IAM user; role atau EC2 instance tidak bisa dimasukkan ke group.[^7]

**IAM role** adalah identitas dengan izin tertentu yang tidak terikat pada satu orang. Siapa pun yang diizinkan bisa assume role itu, dan role tidak punya password atau access key sendiri.[^6] Setiap role butuh dua policy:[^6][^7]
- **Trust policy** menjawab "*siapa* yang boleh assume role ini?". Contoh: service EC2, user `budi`, atau account lain.
- **Permissions policy** menjawab "*apa* yang boleh dilakukan setelah role dipakai?". Contoh: baca bucket `shop-invoices`.

**Service role** adalah role yang di-assume oleh service AWS untuk bertindak atas namamu.[^6] **Service-linked role** adalah service role yang terhubung ke satu service AWS tertentu. Admin IAM bisa melihat izinnya, tapi tidak bisa mengubahnya.[^6]

Role di IAM sekilas mirip RBAC di Kubernetes: Role berisi aturan, lalu subject (user atau ServiceAccount) di-bind ke Role itu.

**Di mana analogi ini tidak cocok:** di Kubernetes, subject tetap memakai identitasnya sendiri dan mendapat tambahan aturan dari Role. Di IAM, principal yang assume role untuk sementara melepas izinnya sendiri dan hanya memakai izin role itu.[^6]

| | IAM user | IAM role |
|---|---|---|
| Credentials | Long-term (password, access key)[^2][^11] | Temporary, per role session[^6] |
| Terikat pada | Satu identitas | Siapa pun yang diizinkan trust policy[^6] |
| Dipakai untuk | Kasus yang tidak didukung federation[^6] | Aplikasi di AWS, karyawan lewat federation, akses lintas account |
| Kalau credentials bocor | Berlaku sampai kamu hapus atau rotate sendiri | Kedaluwarsa sendiri saat session habis |

## 2. Credentials: bukti identitas

Credentials ada dua jenis, dan bedanya penting untuk keamanan.

| | Long-term credentials | Temporary credentials |
|---|---|---|
| Contoh | Password IAM user, access key[^11] | Credentials dari role session[^6] |
| Masa berlaku | Sampai dihapus atau diganti manual | Kedaluwarsa sendiri |
| Risiko kalau bocor | Bisa dipakai penyerang sampai kamu sadar dan menghapusnya | Hanya bisa dipakai sampai session habis |
| Rekomendasi AWS | Hindari kalau bisa | Pakai ini: role, bukan access key[^11] |

Access key terdiri dari dua bagian: *access key ID* (contoh `AKIAIOSFODNN7EXAMPLE`) dan *secret access key*.[^11] Access key ID mirip username, secret access key mirip password. CLI atau SDK memakai keduanya bersama-sama untuk mengautentikasi request.

Alur assume role di bawah ini menunjukkan cara temporary credentials didapat. Contohnya developer Budi yang perlu membaca bucket lewat role `s3-reader`.

```mermaid
sequenceDiagram
  participant Dev as Budi (IAM user)
  participant STS as AWS STS
  participant S3 as Amazon S3
  Dev->>STS: 1. AssumeRole untuk role s3-reader
  STS->>STS: 2. Cek trust policy role: apakah Budi boleh?
  STS-->>Dev: 3. Temporary credentials dengan masa berlaku
  Dev->>S3: 4. GetObject memakai temporary credentials
  Note over S3: 5. Yang dicek permissions policy role, bukan policy Budi
  S3-->>Dev: 6. Objek dikembalikan kalau diizinkan
```

> [!info] Cara membaca
> Ada dua pengecekan terpisah: trust policy di langkah 2 ("boleh pakai role ini?") dan permissions policy di langkah 5 ("role ini boleh baca bucket?"). Kalau salah satunya gagal, Budi tidak bisa membaca objek.

Simplified: pengecekan izin di sisi Budi untuk memanggil `AssumeRole` tidak digambar.

1. Budi meminta credentials role `s3-reader` ke STS.
2. AWS memeriksa trust policy role. Trust policy menentukan principal mana yang boleh assume role itu.[^6][^7]
3. Kalau boleh, Budi mendapat temporary credentials. Masa berlakunya mulai 900 detik (15 menit) sampai *maximum session duration* yang diatur di role (1–12 jam).[^3]
4. Budi memanggil S3 memakai temporary credentials itu.
5. Selama session ini, Budi memakai izin role, bukan izinnya sendiri.[^6]
6. S3 mengembalikan objek kalau permissions policy role mengizinkan `s3:GetObject`.

Untuk aplikasi di EC2 atau Lambda, kamu tidak perlu memanggil `AssumeRole` sendiri di kode. Kamu menempelkan role ke EC2 instance atau Lambda function, dan aplikasi memakai temporary credentials dari role itu. Hasilnya, tidak ada access key yang disimpan di server atau di file config.[^5][^6]

*Role chaining* artinya memakai sebuah role untuk assume role lain. Session hasil role chaining paling lama satu jam.[^6]

## 3. Policy: aturan izin

### Anatomi sebuah policy

Contoh di bawah adalah identity-based policy. Policy ini mengizinkan membaca dan menulis objek di bucket `shop-invoices`, tapi hanya dari jaringan kantor `203.0.113.0/24`.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ReadWriteInvoicesFromOffice",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject"],
      "Resource": "arn:aws:s3:::shop-invoices/*",
      "Condition": {
        "IpAddress": { "aws:SourceIp": "203.0.113.0/24" }
      }
    }
  ]
}
```

| Elemen | Fungsi | Di contoh |
|---|---|---|
| `Version` | Versi aturan bahasa policy. `2012-10-17` adalah versi saat ini.[^14] | `2012-10-17` |
| `Statement` | Daftar aturan. Satu policy bisa berisi banyak statement. | satu statement |
| `Sid` | Nama statement (opsional), untuk memudahkan dibaca. | `ReadWriteInvoicesFromOffice` |
| `Effect` | `Allow` atau `Deny`.[^7] | `Allow` |
| `Action` | Operasi yang diatur.[^7] Bisa pakai wildcard `*`, misalnya `s3:*` untuk semua action S3.[^13] | baca dan tulis objek |
| `Resource` | ARN resource yang diatur.[^7] | semua objek di bucket `shop-invoices` |
| `Condition` | Syarat tambahan (opsional) agar statement berlaku.[^7] | hanya dari IP kantor |

Di identity-based policy kamu tidak boleh menulis elemen `Principal`, karena "siapa"-nya otomatis identity tempat policy itu ditempel. Di resource-based policy, elemen `Principal` wajib ada untuk menulis siapa yang diberi izin.[^12]

### Membaca ARN

ARN dipakai di elemen `Resource` untuk menunjuk resource secara tepat. Formatnya:[^10]

```text
arn:partition:service:region:account-id:resource-id
```

| Bagian | Artinya[^10] | Contoh |
|---|---|---|
| `partition` | Kelompok Region tempat resource berada. Setiap account ada di satu partition. | `aws` |
| `service` | Nama service. | `s3`, `iam`, `ec2` |
| `region` | Kode Region. | `ap-southeast-3` |
| `account-id` | ID account pemilik resource, tanpa tanda hubung. | `123456789012` |
| `resource-id` | Nama, ID, atau path resource. Kadang diawali tipe resource, misalnya `user/` atau `instance/`. | `user/budi` |

ARN untuk sebagian resource tidak berisi Region, account ID, atau keduanya.[^10] Perhatikan bagian yang kosong (`::`) di contoh berikut:

| Resource | ARN |
|---|---|
| IAM user `budi` | `arn:aws:iam::123456789012:user/budi` (Region kosong)[^10] |
| Semua objek di bucket `shop-invoices` | `arn:aws:s3:::shop-invoices/*` (Region dan account kosong) |
| EC2 instance | `arn:aws:ec2:ap-southeast-3:123456789012:instance/i-0abc1234def567890` |

### Jenis-jenis policy

Policy dibagi dua kelompok besar: yang **memberi** izin dan yang hanya **membatasi** izin.

```mermaid
flowchart LR
  subgraph grant["Memberi izin"]
    ibp["Identity-based policy<br>ditempel ke user, group, role"]
    rbp["Resource-based policy<br>ditempel ke resource, mis. bucket"]
  end
  subgraph limit["Hanya membatasi"]
    pb["Permissions boundary<br>untuk user atau role"]
    scp["SCP dan RCP<br>untuk account di Organizations"]
    sp["Session policy<br>untuk satu role session"]
  end
  eff["Izin efektif"]
  grant -->|"izin yang diberikan"| eff
  limit -->|"memotong izin"| eff
```

> [!info] Cara membaca
> Izin efektif adalah izin dari kelompok kiri atas, lalu dipotong oleh kelompok kiri bawah. Kalau kelompok "memberi izin" kosong, hasilnya tetap Deny walaupun SCP mengizinkan semuanya.

Simplified: explicit deny, yang bisa muncul di policy jenis apa pun, tidak digambar. Lihat bagian 4.

**Identity-based policy** ditempel ke user, group, atau role. Ada tiga bentuk:[^7]
- **AWS managed policy**: dibuat dan dikelola oleh AWS. Praktis untuk mulai, tapi belum tentu least privilege untuk kasusmu.[^5]
- **Customer managed policy**: kamu buat sendiri di account-mu. Bisa ditempel ke banyak user, group, dan role.
- **Inline policy**: ditulis langsung di satu user, group, atau role. Ikut terhapus kalau identity-nya dihapus.

**Resource-based policy** ditempel ke resource, misalnya S3 bucket policy. Policy ini memberi izin ke principal yang ditulis di dalamnya, termasuk principal dari account lain.[^7][^9]

**Trust policy** adalah resource-based policy milik role. Isinya menentukan principal mana yang boleh assume role itu.[^6][^7]

**Permissions boundary** menetapkan izin *maksimum* yang bisa dimiliki sebuah user atau role. Boundary sendiri tidak memberi izin.[^7] Contoh: boundary hanya mengizinkan `s3:*`. Walaupun admin menempelkan policy `ec2:*` ke user itu, user tetap tidak bisa memakai EC2.

**SCP (service control policy)** dan **RCP (resource control policy)** menetapkan izin maksimum untuk account di AWS Organizations. Keduanya tidak memberi izin.[^5][^7] Detailnya di [[iam-advanced]].

**Session policy** diberikan saat assume role, untuk membatasi izin satu session itu saja.[^7]

| | Managed policy | Inline policy |
|---|---|---|
| Bisa dipakai ulang di banyak identity | Ya[^7] | Tidak, hubungannya satu-ke-satu[^7] |
| Kalau identity dihapus | Policy tetap ada | Ikut terhapus[^7] |
| Cocok untuk | Izin yang dipakai bersama | Izin khusus yang tidak boleh ikut ke identity lain |

## 4. Cara AWS memutuskan Allow atau Deny

Ini bagian yang paling sering diuji. Di dalam satu account, AWS memeriksa jenis-jenis policy dalam urutan berikut.[^8]

```mermaid
flowchart TD
  s(["Request masuk"])
  d1{"(1) Ada explicit Deny?"}
  d2{"(2) RCP mengizinkan?"}
  d3{"(3) SCP mengizinkan?"}
  d4{"(4) Resource-based<br>policy mengizinkan?"}
  d5{"(5) Identity-based<br>policy mengizinkan?"}
  d6{"(6) Permissions boundary<br>mengizinkan?"}
  d7{"(7) Session policy<br>mengizinkan?"}
  x1["DENY"]
  x2["DENY"]
  x3["DENY"]
  x5["DENY"]
  x6["DENY"]
  x7["DENY"]
  allow["ALLOW"]
  s --> d1
  d1 -->|"ya"| x1
  d1 -->|"tidak"| d2
  d2 -->|"tidak"| x2
  d2 -->|"ya, atau tidak ada RCP"| d3
  d3 -->|"tidak"| x3
  d3 -->|"ya, atau tidak ada SCP"| d4
  d4 -->|"ya: bisa langsung Allow"| allow
  d4 -->|"tidak"| d5
  d5 -->|"tidak"| x5
  d5 -->|"ya"| d6
  d6 -->|"tidak"| x6
  d6 -->|"ya, atau tidak ada boundary"| d7
  d7 -->|"tidak"| x7
  d7 -->|"ya, atau tidak ada session policy"| allow
```

> [!info] Cara membaca
> Untuk sampai ke ALLOW, request harus lolos semua gerbang. Gagal di satu gerbang saja sudah DENY. Perhatikan langkah 1: explicit Deny diperiksa paling awal, jadi tidak ada Allow yang bisa mengalahkannya.

Simplified: di langkah 4, Allow dari resource-based policy bisa langsung jadi Allow *tergantung jenis principal-nya*. Pengecualiannya ada di bawah daftar ini. Langkah 7 hanya berlaku untuk *session principal* (misalnya role session).

1. **Explicit deny.** AWS mengumpulkan semua policy yang berlaku. Kalau ada satu saja explicit deny yang cocok, keputusannya Deny.[^8]
2. **RCP.** Kalau account ada di Organizations dan memakai RCP, RCP harus mengizinkan request itu.[^4][^8]
3. **SCP.** Sama seperti RCP: kalau ada SCP, SCP harus mengizinkan.[^4][^8]
4. **Resource-based policy.** Kalau resource-based policy mengizinkan, hasilnya bisa langsung Allow, walaupun identity-based policy tidak mengizinkan.[^8]
5. **Identity-based policy.** Kalau langkah 4 tidak memberi Allow, identity-based policy harus mengizinkan. Kalau tidak, hasilnya implicit deny.[^8]
6. **Permissions boundary.** Kalau user atau role punya boundary, boundary harus mengizinkan action itu.[^8]
7. **Session policy.** Kalau principal adalah session dan ada session policy, session policy harus mengizinkan. Kalau principal bukan session, keputusan akhirnya Allow.[^8]

Pengecualian di langkah 4, menurut docs:[^8]
- Resource-based policy yang memberi izin ke **ARN role** tetap dibatasi oleh permissions boundary atau session policy.
- Resource-based policy yang memberi izin ke **ARN IAM user** di account yang sama tidak dibatasi oleh implicit deny di identity-based policy atau permissions boundary.
- **Trust policy role** dan **KMS key policy** harus mengizinkan principal secara eksplisit. Untuk dua jenis policy ini, izin dari identity-based policy saja tidak cukup. Lihat [[kms-and-cloudhsm]].

Ringkasnya untuk ujian: di dalam satu account, izin dari identity-based policy dan resource-based policy digabung (*union*). Kalau salah satunya mengizinkan, AWS mengizinkan. Permissions boundary, SCP, dan RCP lalu memotong hasilnya (*intersection*). Explicit deny di mana pun mengalahkan semuanya.[^4]

### Worked example

Semua contoh di dalam satu account. Kolom "Gerbang" menunjuk nomor langkah di diagram.

| # | Situasi | Hasil | Gerbang |
|---|---|---|---|
| 1 | User baru tanpa policy apa pun memanggil `s3:ListBucket`. | Deny (implicit) | 5: tidak ada yang mengizinkan |
| 2 | Identity policy: Allow `ec2:*`. Policy lain: Deny `ec2:TerminateInstances`. User memanggil `ec2:TerminateInstances`. | Deny (explicit) | 1 |
| 3 | Identity policy: Allow `*` (semua action). Boundary: Allow `s3:*` saja. User memanggil `ec2:RunInstances`. | Deny | 6: boundary tidak mengizinkan EC2 |
| 4 | Sama seperti nomor 3, tapi user memanggil `s3:PutObject`. | Allow | lolos 5 dan 6 |
| 5 | SCP: Allow `s3:*`. User tidak punya identity policy, bucket tidak punya bucket policy. | Deny | 5: SCP tidak memberi izin |

## 5. Akses lintas account (cross-account)

Kalau principal dan resource ada di account yang berbeda, aturannya lebih ketat. Docs menyebut account peminta sebagai *trusted account* dan account pemilik resource sebagai *trusting account*.[^9]

```mermaid
flowchart LR
  subgraph a["Account A 111111111111 (trusted, peminta)"]
    ua["IAM user: budi"]
    ipa["Identity-based policy<br>Allow s3:GetObject ke bucket di B"]
  end
  subgraph b["Account B 222222222222 (trusting, pemilik)"]
    bk[("S3 bucket: shop-invoices")]
    bp["Bucket policy<br>Allow untuk budi dari account A"]
  end
  ipa -.-|"ditempel ke"| ua
  bp -.-|"ditempel ke"| bk
  ua -->|"GetObject"| bk
```

> [!info] Cara membaca
> Ada dua policy, satu di tiap account. Request hanya berhasil kalau **keduanya** Allow. Di dalam satu account, salah satu saja sudah cukup; di lintas account tidak.

Untuk request lintas account, principal di account A butuh identity-based policy yang mengizinkan request ke resource di account B. Selain itu, resource-based policy di account B harus mengizinkan principal dari account A. Request diizinkan hanya kalau kedua evaluasi menghasilkan Allow.[^9]

Cara lain adalah role di account B dengan trust policy yang mengizinkan principal dari account A. Principal di account A lalu assume role itu.[^9] Pola cross-account role dibahas di [[iam-advanced]].

## Fakta dan batas penting (Key facts and limits)

| Fakta | Nilai | Sumber |
|---|---|---|
| Biaya IAM | Tidak ada biaya tambahan | [^2] |
| Keputusan default untuk request | Implicit deny; root user punya akses penuh | [^8] |
| Customer managed policy per account | 1.500 default (kuota maksimum 10.000) | [^3] |
| Group per account | 300 default (kuota maksimum 500) | [^3] |
| Role per account | 1.000 default (kuota maksimum 10.000) | [^3] |
| Managed policy per role / user / group | 20 / 10 / 10 default | [^3] |
| Ukuran satu managed policy | Maksimal 6.144 karakter (spasi tidak dihitung) | [^3] |
| Durasi role session | 900 detik (15 menit) sampai maximum session duration role (1–12 jam) | [^3] |
| Role chaining | Session maksimal satu jam | [^6] |

## Perbandingan (Comparisons)

| Jenis policy | Memberi izin? | Ditempel ke |
|---|---|---|
| Identity-based | Ya | User, group, role[^7] |
| Resource-based | Ya, ke principal yang ditulis di policy | Resource[^7] |
| Trust policy | Ya, izin untuk assume role | Role[^6][^7] |
| Permissions boundary | Tidak, hanya menetapkan batas maksimum[^7] | User atau role |
| SCP / RCP | Tidak, hanya menetapkan batas maksimum[^5][^7] | Root organization, OU, atau account |
| Session policy | Tidak, hanya membatasi session[^7] | Diberikan saat assume role |

## Memilih di soal ujian (Choosing in exam scenarios)

```mermaid
flowchart TD
  start(["Kebutuhan di soal"])
  q1{"Memberi akses ke siapa?"}
  q2{"Membatasi izin maksimum untuk siapa?"}
  a1["IAM role<br>ditempel ke EC2 atau Lambda"]
  a2["Federation lewat<br>IAM Identity Center"]
  a3["IAM user + MFA<br>hanya jika federation tidak bisa"]
  a4["Permissions boundary"]
  a5["SCP di Organizations"]
  start -->|"memberi akses"| q1
  start -->|"membatasi akses"| q2
  q1 -->|"aplikasi di AWS"| a1
  q1 -->|"karyawan"| a2
  q1 -->|"kasus tanpa federation"| a3
  q2 -->|"satu user atau role"| a4
  q2 -->|"seluruh account atau OU"| a5
```

> [!info] Cara membaca
> Pertama tentukan apakah soal meminta *memberi* akses atau *membatasi* akses. Soal "membatasi" hampir selalu dijawab dengan boundary atau SCP, karena keduanya tidak bisa memberi izin.

- **Aplikasi di EC2 perlu memanggil S3 atau DynamoDB** → tempelkan IAM role ke instance. Workload sebaiknya memakai temporary credentials dari role, jadi tidak ada long-term key yang disebar.[^5]
- **Karyawan perlu akses ke AWS** → federation dengan identity provider. AWS menyarankan IAM Identity Center untuk pengelolaan akses terpusat.[^5] Detailnya di [[iam-advanced]].
- **Banyak user butuh izin yang sama** → kumpulkan dalam group, tempelkan policy ke group.[^7]
- **Least privilege** → mulai dari AWS managed policy, lalu kurangi izinnya dengan customer managed policy sesuai kebutuhan.[^5]
- **Harus ada IAM user atau root user** → wajibkan MFA.[^5]
- **Batasi apa yang bisa dilakukan role, walaupun nanti ada yang menambah policy** → permissions boundary atau SCP. Keduanya tidak memberi akses sendiri.[^5]
- **Account lain perlu membaca bucket-mu** → bucket policy di account-mu **dan** identity-based policy di account mereka, atau cross-account role.[^9]

> [!tip] Exam tip (community)
> Belum ada. Tidak ada halaman Tutorials Dojo yang di-fetch untuk catatan ini.

## Jebakan umum (Common traps and misconceptions)

- **Miskonsepsi:** SCP atau permissions boundary memberi akses ke user. → **Kenyataan:** "No permissions are granted by SCPs and RCPs." Permissions boundary juga tidak memberi izin sendiri. Kamu tetap butuh identity-based atau resource-based policy.[^5]
- **Miskonsepsi:** Allow di satu policy bisa mengalahkan Deny di policy lain. → **Kenyataan:** explicit Deny selalu menang atas Allow.[^2]
- **Miskonsepsi:** Role punya access key seperti user. → **Kenyataan:** role tidak punya long-term credentials standar. Assume role memberi temporary credentials.[^6]
- **Miskonsepsi:** Permissions boundary bisa dipasang di service-linked role. → **Kenyataan:** tidak bisa.[^6]
- **Miskonsepsi:** Inline policy bisa dipakai ulang. → **Kenyataan:** inline policy punya hubungan satu-ke-satu dengan identity-nya. Pakai managed policy kalau ingin berbagi policy.[^7]
- **Miskonsepsi:** Untuk akses lintas account, bucket policy yang mengizinkan account lain sudah cukup. → **Kenyataan:** principal di account lain juga butuh identity-based policy yang mengizinkan. Keduanya harus Allow.[^9]
- **Miskonsepsi:** Root user praktis untuk kerja harian dan script karena aksesnya penuh. → **Kenyataan:** jangan buat access key untuk root user[^11], dan wajibkan MFA untuk root user.[^5]

## Latihan soal (Self-check)

Soal ditulis dalam bahasa Inggris seperti di ujian. Pembahasannya dalam bahasa Indonesia.

**Q1.** An application runs on EC2 instances and must read objects from an S3 bucket. The company wants no long-term credentials stored on the instances. What should the architect do?
- A. Create an IAM user, and store its access keys in the application config.
- B. Create an IAM role with S3 read permissions and attach it to the instances.
- C. Use the root user access keys.
- D. Create an IAM group with S3 permissions and add the instances to it.

> [!question]- Jawaban
> **B.** Role memberi temporary credentials ke workload, jadi tidak ada long-term key yang disebar.[^5] A dan C memakai long-term access key, yang tidak disarankan AWS untuk workload; untuk root user, access key malah tidak boleh dibuat.[^5][^11] D salah karena group hanya berisi IAM user, bukan EC2 instance.[^7]

**Q2.** An SCP in an organization allows `s3:*`. An IAM user in a member account has no identity-based policy that allows any S3 action, and the bucket has no resource-based policy. Can the user read objects?
- A. Yes, because the SCP allows it.
- B. Yes, because the user is in the organization.
- C. No, because an SCP does not grant permissions.
- D. No, because SCPs only apply to the root user.

> [!question]- Jawaban
> **C.** SCP tidak memberi izin. Request harus diizinkan oleh policy milik user **dan** oleh SCP.[^4][^7] Ini worked example nomor 5. A dan B menganggap SCP sebagai pemberi izin. D salah karena SCP menetapkan izin maksimum untuk IAM user dan role di member account, dan juga berlaku untuk root user member account.[^2][^7]

**Q3.** A user's identity-based policy allows `ec2:TerminateInstances`. A second policy that applies to the same request has an explicit `Deny` for the same action. What is the result?
- A. Allowed, because the Allow was attached first.
- B. Allowed, because the more specific policy wins.
- C. Denied, because an explicit Deny prevails over an Allow.
- D. Denied, because AWS always denies when two policies conflict, even if neither contains a Deny.

> [!question]- Jawaban
> **C.** Explicit Deny selalu menang atas Allow; di diagram evaluasi, ini gerbang nomor 1.[^2][^8] A dan B mengarang aturan urutan atau "paling spesifik" yang tidak ada di docs. D salah karena penolakannya datang dari statement Deny. Tanpa Deny, satu Allow di identity-based atau resource-based policy sudah cukup di dalam satu account.[^4]

**Q4.** An IAM role has an identity-based policy that allows all actions (`"Action": "*"`). The role also has a permissions boundary that allows only `s3:*` and `cloudwatch:*`. An application using the role calls `ec2:RunInstances`. What happens?
- A. Allowed, because the identity-based policy allows all actions.
- B. Denied, because the permissions boundary does not allow the action.
- C. Allowed, because the permissions boundary has no explicit Deny for EC2.
- D. Denied, because a role cannot have both a permissions boundary and an identity-based policy.

> [!question]- Jawaban
> **B.** Permissions boundary membatasi hasil akhir (intersection). Action harus diizinkan oleh identity-based policy **dan** boundary.[^4][^8] Ini worked example nomor 3. A mengabaikan boundary. C salah karena boundary tidak perlu Deny; cukup tidak mengizinkan, dan hasilnya implicit deny. D salah karena boundary memang dipasang bersama identity-based policy; boundary sendiri tidak memberi izin.[^7]

**Q5.** Account B owns an S3 bucket. Its bucket policy allows `s3:GetObject` for an IAM user in Account A. The IAM user in Account A has no identity-based policy that allows S3 actions. The user tries to read an object. What is the result?
- A. Allowed, because the bucket policy allows the user.
- B. Denied, because the user also needs an identity-based policy in Account A that allows the request.
- C. Allowed, because resource-based policies always override identity-based policies.
- D. Denied, because S3 buckets cannot be accessed from another account.

> [!question]- Jawaban
> **B.** Untuk request lintas account, identity-based policy di account peminta dan resource-based policy di account pemilik harus sama-sama Allow.[^9] A benar hanya untuk kasus di dalam satu account. C salah karena aturan "salah satu cukup" tidak berlaku lintas account. D salah karena akses lintas account memang bisa, asal kedua sisi mengizinkan.[^9]

## Terkait (Related)

- [[iam-advanced]]: STS, cross-account role, AWS Organizations dan SCP, IAM Identity Center
- [[security-groups-vs-nacls]]: security group dan network ACL, yaitu kontrol di level jaringan. IAM mengatur izin memanggil API; security group mengatur traffic jaringan.
- [[kms-and-cloudhsm]]: KMS key policy, salah satu pengecualian di aturan evaluasi

## Sources
[^1]: AWS Certified Solutions Architect – Associate (SAA-C03) Exam Guide, Version 1.1 — https://d1.awsstatic.com/training-and-certification/docs-sa-assoc/AWS-Certified-Solutions-Architect-Associate_Exam-Guide.pdf (accessed 2026-11-08). Task statement 1.1 "Design secure access to AWS resources" lists IAM users, groups, roles, policies, MFA for IAM and root users, STS, SCPs, and resource policies.
[^2]: AWS Identity and Access Management FAQs — https://aws.amazon.com/iam/faqs/ (accessed 2026-11-08)
[^3]: IAM and AWS STS quotas — https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html (accessed 2026-11-08)
[^4]: Policy evaluation logic — https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_evaluation-logic.html (accessed 2026-11-08)
[^5]: Security best practices in IAM — https://docs.aws.amazon.com/IAM/latest/UserGuide/best-practices.html (accessed 2026-11-08)
[^6]: IAM roles — https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles.html (accessed 2026-11-08)
[^7]: Policies and permissions in AWS Identity and Access Management — https://docs.aws.amazon.com/IAM/latest/UserGuide/access_policies.html (accessed 2026-11-08)
[^8]: Determining whether a request is allowed or denied within an account — https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_evaluation-logic_policy-eval-denyallow.html (accessed 2026-11-23)
[^9]: Cross-account policy evaluation logic — https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_evaluation-logic-cross-account.html (accessed 2026-11-23)
[^10]: Identify AWS resources with Amazon Resource Names (ARNs) — https://docs.aws.amazon.com/IAM/latest/UserGuide/reference-arns.html (accessed 2026-11-23)
[^11]: Manage access keys for IAM users — https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html (accessed 2026-11-23)
[^12]: IAM JSON policy elements: Principal — https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements_principal.html (accessed 2026-11-23)
[^13]: IAM JSON policy elements: Action — https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements_action.html (accessed 2026-11-23)
[^14]: IAM JSON policy elements: Version — https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements_version.html (accessed 2026-11-23)
