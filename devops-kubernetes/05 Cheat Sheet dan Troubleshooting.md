# 05 Cheat Sheet dan Troubleshooting

Kembali ke [[00 Mulai di Sini]]

---

## kubectl harian

```bash
# Konteks & namespace
kubectl config get-contexts
kubectl config use-context <nama>
kubectl config set-context --current --namespace=<ns>

# Melihat
kubectl get nodes -o wide
kubectl get pods -n <ns> -o wide              # -o wide = tampilkan node & IP
kubectl get pods -A                           # semua namespace
kubectl get deploy,rs,svc,ingress -n <ns>
kubectl get pods -w                           # pantau terus (watch)
kubectl get events -n <ns> --sort-by=.lastTimestamp
kubectl describe pod <pod> -n <ns>            # detail + Events di bagian bawah (paling berguna)
kubectl get deploy <d> -n <ns> -o yaml        # lihat definisi lengkap
kubectl top nodes / kubectl top pods -n <ns>  # pemakaian CPU/RAM (butuh metrics-server)

# Log & masuk ke container
kubectl logs <pod> -n <ns>
kubectl logs <pod> -n <ns> -f                 # ikuti terus
kubectl logs <pod> -n <ns> --previous         # log container sebelum crash
kubectl logs deploy/<d> -n <ns>               # log dari salah satu pod deployment
kubectl exec -it <pod> -n <ns> -- sh

# Deploy & rollback
kubectl apply -f <file-atau-folder>
kubectl set image deployment/<d> <container>=<image>:<tag> -n <ns>
kubectl rollout status deployment/<d> -n <ns>
kubectl rollout history deployment/<d> -n <ns>
kubectl rollout undo deployment/<d> -n <ns> [--to-revision=N]
kubectl rollout restart deployment/<d> -n <ns>   # restart semua pod (mis. setelah ubah ConfigMap)
kubectl scale deployment/<d> --replicas=N -n <ns>

# Jaringan
kubectl port-forward svc/<svc> 8080:80 -n <ns>
kubectl get endpointslices -n <ns> -l kubernetes.io/service-name=<svc>   # pod mana di belakang Service
kubectl run tmp --rm -it --image=curlimages/curl -- sh                    # pod sementara untuk tes

# Belajar sendiri
kubectl explain deployment.spec.strategy
kubectl api-resources
kubectl auth can-i <verb> <resource> -n <ns>
```

Singkatan yang dipakai di mana-mana: `po` = pods, `deploy` = deployments, `rs` = replicasets, `svc` = services, `ing` = ingress, `ns` = namespaces, `cm` = configmaps.

---

## Arti status pod dan apa yang harus dicek

| Status | Artinya | Cek ini dulu | Penyebab umum |
|---|---|---|---|
| `Pending` | Belum dapat node | `describe pod` → Events | RAM/CPU worker tidak cukup untuk `requests`; PVC belum terikat; semua node ber-taint |
| `ContainerCreating` (lama) | Node sedang menyiapkan | `describe pod` | Image besar masih ditarik; volume/Secret/ConfigMap yang dirujuk tidak ada |
| `ErrImagePull` / `ImagePullBackOff` | Gagal menarik image | `describe pod` → pesan pull | Tag salah atau belum di-push; registry mati; registry butuh login (`imagePullSecrets`); node tidak mengizinkan registry HTTP |
| `CrashLoopBackOff` | Container start lalu mati, berulang | `logs --previous` | Env/Secret kurang, DB tidak terjangkau, error saat start, port salah sehingga liveness gagal |
| `Running` tapi `READY 0/1` | Jalan tapi readinessProbe gagal | `describe pod` → Readiness probe failed | Path atau port probe salah; aplikasi lambat start (naikkan `initialDelaySeconds`) |
| `OOMKilled` (di `describe`, Last State) | Memori melewati `limits.memory` | `describe pod`, `top pod` | Limit terlalu kecil atau memory leak. Untuk Node.js atur `--max-old-space-size` di bawah limit (contoh di manifest `landing-cms`) |
| `Evicted` | Diusir kubelet | `describe pod` | Disk atau memori node penuh |
| `Terminating` (macet) | Tidak mau berhenti | `describe pod` | Node mati atau ada finalizer. Jangan langsung `--force`, tanya dulu |
| `Completed` | Selesai dengan sukses | — | Normal untuk Job/CronJob |

## Alur mencari masalah: "aplikasinya error"

Telusuri dari luar ke dalam:

```
Browser / curl
  → Ingress ada & host/path benar?        kubectl get ingress -n <ns>; kubectl describe ingress <i>
  → Service menunjuk ke pod?              kubectl get endpointslices -l kubernetes.io/service-name=<svc>
                                          (kosong = label selector Service tidak cocok dengan label pod,
                                           atau semua pod belum Ready)
  → Pod Running & Ready?                  kubectl get pods -o wide
  → Log aplikasi?                         kubectl logs <pod> [--previous]
  → Dependensi di luar cluster?           dari pod: koneksi ke DB 10.20.0.37:5434, MinIO 10.20.0.37:9000, Redis 10.20.0.61
```

Kalau masalahnya di node:

```bash
kubectl get nodes                      # ada yang NotReady?
kubectl describe node <node>           # Conditions: MemoryPressure, DiskPressure, ...
# di node itu sendiri:
sudo systemctl status kubelet containerd
sudo journalctl -u kubelet --since "15 min ago"
sudo crictl ps                         # container yang jalan menurut containerd
sudo crictl logs <container-id>
```

---

## Masalah umum saat membangun lab

| Gejala | Penyebab & solusi |
|---|---|
| `kubeadm init` gagal di preflight soal swap | Lupa `swapoff -a` |
| Node `NotReady` terus setelah init | CNI belum terpasang atau pod Calico belum Running (`kubectl get pods -n calico-system`) |
| Pod sering restart tanpa alasan, kubelet log menyebut cgroup | `SystemdCgroup = true` belum diset di containerd → ubah lalu `systemctl restart containerd kubelet` |
| `kubeadm join` error token | Token kedaluwarsa (24 jam): `sudo kubeadm token create --print-join-command` |
| `http: server gave HTTP response to HTTPS client` saat pull | `hosts.toml` registry belum ada di node, atau `config_path` containerd belum diset |
| Hal yang sama saat `docker push` | `insecure-registries` belum ada di `/etc/docker/daemon.json` |
| `EXTERNAL-IP <pending>` | MetalLB belum terpasang atau `IPAddressPool` belum dibuat |
| IP MetalLB tidak bisa di-curl dari Mac | Rentang IP di luar subnet VM; coba curl dari dalam VM dulu |
| Ingress 404 dari nginx | Header `Host` tidak cocok dengan `host:` di Ingress, atau `ingressClassName` salah |
| Calico bermasalah setelah init | `pod-network-cidr` bentrok dengan subnet VM |

## Masalah umum di GitLab CI

| Gejala | Penyebab & solusi |
|---|---|
| Job "pending / stuck", tidak ada runner | Tag job tidak cocok dengan tag runner, atau runner offline (`sudo gitlab-runner status`, `sudo gitlab-runner verify`) |
| `ERROR: Job failed: prepare environment: exit status 1` | File `.bash_logout` milik user gitlab-runner → `sudo rm /home/gitlab-runner/.bash_logout` |
| `permission denied ... docker.sock` | User gitlab-runner belum masuk grup docker → `usermod -aG docker gitlab-runner` lalu restart runner |
| `deployments.apps "x" not found` | Deployment belum pernah dibuat. Pipeline hanya `set image`, jadi `kubectl apply` manifest dulu |
| `Forbidden: User "system:serviceaccount:..." cannot ...` | Role kurang izin, atau namespace salah. Cek dengan `kubectl auth can-i ... --as=...` |
| `rollout status` timeout | Pod baru tidak pernah Ready. Lihat `describe` dan `logs` pod baru |

---

## Cek versi yang sedang jalan di kantor

Read-only, untuk dijalankan sendiri dengan hati-hati. `admin.conf` adalah akses cluster-admin, jadi jalankan **hanya** perintah `get` / `describe` / `logs` / `history`.

Dari CP1:

```bash
ssh -p <port> <akun>@10.20.0.10
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf -n app-backend  get deploy lx-api -o wide
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf -n app-frontend get deploy fe-layanan-x -o wide
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf -n app-backend  get rs -o wide | grep lx
sudo kubectl --kubeconfig /etc/kubernetes/admin.conf -n app-backend  get pods -o wide | grep lx
```

Bagian setelah `:` di kolom `IMAGES` adalah hash commit. Cocokkan di laptop:

```bash
cd ~/work/app-rebuild/backend/layanan-x-api
git fetch origin
git log -1 <hash>
git branch -r --contains <hash>      # seharusnya origin/main
```

Cara cadangan langsung di worker (kalau `kubectl` tidak tersedia):

```bash
sudo crictl ps --name lx-api
sudo crictl inspect <container-id> | grep '"image"'
```

---

## Peta IP kantor (production)

| Host | IP | Peran |
|---|---|---|
| CP1–CP3 | 10.20.0.10–102 | Control plane, VIP API **10.20.0.38:6443** |
| WK1–WK5 | 10.20.0.13–17 | Worker (pod aplikasi) |
| PGPool1–2 | 10.20.0.18–19 | PgPool |
| Registry | 10.20.0.22 | Registry image `:5000` + gitlab-runner |
| PGnode1–3 | 10.20.0.23–25 | PostgreSQL Patroni |
| HAproxy1–2 | 10.20.0.26–27 | HAProxy + keepalived, VIP **10.20.0.37** |
| MINIO1–3 | 10.20.0.28–30 | MinIO distributed (2 disk per node) |
| REBUILDredis | 10.20.0.61 | Redis |

SSH memakai port khusus (bukan 22) dan harus lewat VPN. Kredensial ada di catatan VM, bukan di sini.

---

## Kamus singkat

| Istilah | Arti singkat |
|---|---|
| CNI | Plugin jaringan pod (Calico) |
| CRI | Antarmuka kubelet ke runtime (containerd) |
| CSI | Antarmuka ke penyimpanan (Longhorn) |
| DaemonSet | Pod yang wajib ada di setiap node |
| StatefulSet | Seperti Deployment, tapi pod punya identitas dan disk tetap (untuk database di dalam k8s) |
| Taint / Toleration | "Larangan" pada node / "izin" pada pod untuk melanggarnya |
| Requests / Limits | Jatah minimum / batas maksimum CPU & RAM |
| Probe | Cek kesehatan: liveness (restart?) dan readiness (boleh menerima traffic?) |
| Rolling update | Ganti versi pod sedikit demi sedikit |
| Desired state | Keadaan yang kamu deklarasikan. Controller terus menyamakan kenyataan dengannya |
| VIP | Virtual IP yang bisa pindah antar-server (keepalived/VRRP) |
| Quorum | Mayoritas anggota etcd yang harus setuju |
| GitOps | Git sebagai satu-satunya sumber kebenaran, cluster menyesuaikan diri ke isi Git |
