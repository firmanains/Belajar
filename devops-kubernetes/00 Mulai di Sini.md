# Belajar DevOps & Kubernetes dari Cluster Kantor

Dibuat: 2026-11-03
Untuk: software engineer dengan pengalaman ±1 tahun yang mau belajar DevOps, CI/CD, dan Kubernetes.

Catatan ini memakai cluster production kantor (10.20.0.x) sebagai contoh nyata. Fakta tentang kantor di sini adalah hasil pengecekan read-only per 2026-11-03: daftar proses, port, dan file config yang bisa dibaca tanpa sudo, ditambah isi `.gitlab-ci.yml` di repo. Hal yang belum bisa dipastikan ditandai **(dugaan)**.

## Urutan belajar

| # | Catatan | Isinya | Perkiraan waktu |
|---|---|---|---|
| 1 | [[01 Arsitektur Cluster Belajar]] | Konsep dasar (container, pod, CP, WK, MinIO), alur request, alur deploy | 1–2 jam baca |
| 2 | [[02 Lab 1 - Kubernetes di Laptop]] | Cluster latihan 1 CP + 2 WK di laptop pakai `kind`, deploy aplikasi pertama, scale, rollback | 1 sore |
| 3 | [[03 Lab 2 - Cluster seperti Kantor]] | Bangun cluster "beneran" di VM pakai `kubeadm`, plus registry, MinIO, MetalLB, ingress | 1–2 hari |
| 4 | [[04 Lab 3 - CI-CD Deploy dari main]] | GitLab CI: push ke `main` → build image → deploy otomatis ke cluster | 1 hari |
| 5 | [[05 Cheat Sheet dan Troubleshooting]] | Command harian, arti status pod, cara mencari sumber masalah | referensi |

Kerjakan berurutan. Lab 2 dan Lab 3 memakai hasil lab sebelumnya.

## Aturan main

1. **Jangan latihan di cluster kantor.** Itu production. Akun `<akun>` punya sudo penuh di sana, jadi satu salah ketik bisa mematikan layanan publik. Semua latihan dilakukan di laptop atau VM sendiri.
2. Di cluster kantor, cukup lakukan perintah yang **membaca** (`get`, `describe`, `logs`). Jangan `apply`, `delete`, `edit`, `scale`, `rollout undo` tanpa persetujuan tim.
3. Kredensial VM ada di catatan VM (`catatan-vm.xlsx`). Jangan disalin ke catatan ini atau ke repo.
4. Ketik command sendiri, jangan copy-paste buta. Sebelum menekan Enter, tebak dulu apa yang akan terjadi. Kalau hasilnya beda dari tebakanmu, di situlah kamu belajar.

## Peta singkat (kalau cuma punya 1 menit)

- **Kubernetes (k8s)** = sistem yang menjalankan container di banyak server sekaligus dan menjaganya tetap hidup.
- **CP (control plane)** = otak cluster. Menyimpan "keinginan" (misalnya: *aku mau 5 pod `lx-api` versi `1a2b3c4d`*) dan terus memastikan kenyataan sama dengan keinginan itu.
- **WK (worker)** = tenaga kerja. Di sinilah container aplikasi benar-benar jalan.
- **MinIO** = gudang file yang kompatibel dengan Amazon S3. Letaknya di luar Kubernetes. Aplikasi menyimpan PDF dan file upload ke sana.
- **Registry** = gudang image Docker. CI menaruh image di sana, worker mengambilnya dari sana.
- **GitLab CI** = robot yang otomatis build dan deploy setiap ada push ke branch `main`.

> Di kantor, branch production namanya `main`, bukan `master`.

## Setelah semua lab selesai, kamu harus bisa

- [ ] Menjelaskan dengan kata-katamu sendiri apa yang terjadi dari `git push` sampai user melihat versi baru.
- [ ] Menjelaskan kenapa database dan MinIO kantor ada di luar Kubernetes.
- [ ] Membangun cluster 1 CP + 2 WK dari nol dengan `kubeadm`.
- [ ] Menulis Deployment, Service, Ingress, ConfigMap, dan Secret sendiri.
- [ ] Membuat pipeline GitLab CI yang build, push, dan deploy.
- [ ] Melakukan rollback ketika deploy gagal.
- [ ] Membaca status `CrashLoopBackOff` / `ImagePullBackOff` / `Pending` dan tahu harus cek apa.

## Sumber belajar lanjutan

- Dokumentasi resmi: https://kubernetes.io/docs/home/ (bagian *Concepts* dan *Tutorials*)
- `kubectl explain deployment.spec` → dokumentasi langsung dari terminal
- Dokumentasi GitLab CI: https://docs.gitlab.com/ci/
- Kalau mau sertifikasi: CKAD (fokus developer) lalu CKA (fokus admin cluster). Lab 2 di sini sudah mirip materi CKA.

## Baca juga di backend-engineering-vault

- [[Kubernetes Core Concepts - Pods, Deployments, Services, Ingress]]
- [[Kubernetes Config, Secrets, Probes, and Autoscaling]]
- [[Desired-State Reconciliation]]
- [[Docker - Images, Layers, and Multi-Stage Builds for Go]]
- [[CI-CD Pipelines]]
- [[Blue-Green and Canary Releases]]
