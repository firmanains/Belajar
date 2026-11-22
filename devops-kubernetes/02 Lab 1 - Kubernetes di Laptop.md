# 02 Lab 1: Kubernetes di Laptop (kind)

Kembali ke [[01 Arsitektur Cluster Kantor]] · Lanjut ke [[03 Lab 2 - Cluster seperti Kantor]]

---

**Tujuan:** merasakan konsep CP vs WK, Deployment, Service, scaling, self-healing, rolling update, dan rollback dalam satu sore, tanpa repot menyiapkan VM.

**Alat:** `kind` (*Kubernetes IN Docker*). Setiap "node" sebenarnya sebuah container Docker. Ini bukan setup production, tapi perilaku Kubernetes-nya asli.

**Yang sudah ada di Mac kamu** (dicek 2026-11-03): Apple Silicon (arm64), RAM 24 GB, OrbStack (menyediakan `docker`), dan `kubectl`. Yang belum ada: `kind`.

---

## Langkah 0: Persiapan

```bash
brew install kind
docker info | head -5          # pastikan OrbStack/Docker sudah jalan
kind version
kubectl version --client
```

Buat folder kerja:

```bash
mkdir -p ~/belajar-k8s/hello-api && cd ~/belajar-k8s
```

---

## Langkah 1: Buat cluster 1 CP + 2 WK

Buat file `~/belajar-k8s/kind-config.yaml`:

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane   # padanan CP1 di kantor
  - role: worker          # padanan WK1
  - role: worker          # padanan WK2
```

```bash
kind create cluster --name belajar --config kind-config.yaml
```

Proses ini 1–2 menit. Setelah selesai, `kind` otomatis mengatur `~/.kube/config` supaya `kubectl` mengarah ke cluster ini.

```bash
kubectl config current-context      # harus: kind-belajar
kubectl get nodes -o wide
```

Hasilnya kira-kira:

```
NAME                    STATUS   ROLES           VERSION
belajar-control-plane   Ready    control-plane   v1.3x.x
belajar-worker          Ready    <none>          v1.3x.x
belajar-worker2         Ready    <none>          v1.3x.x
```

**Lihat otak cluster:**

```bash
kubectl get pods -n kube-system -o wide
```

Perhatikan bahwa `etcd-...`, `kube-apiserver-...`, `kube-scheduler-...`, dan `kube-controller-manager-...` **hanya** ada di node `control-plane`. Persis sama dengan CP1–CP3 kantor. Sementara `kube-proxy` dan `kindnet` (CNI bawaan kind) ada di setiap node, karena keduanya DaemonSet.

**Lihat taint di CP:**

```bash
kubectl describe node belajar-control-plane | grep -i taint
```

`node-role.kubernetes.io/control-plane:NoSchedule` inilah yang mencegah pod aplikasi jalan di CP.

---

## Langkah 2: Buat aplikasi kecil

Kita buat API super kecil tanpa dependency supaya fokus ke Kubernetes, bukan ke Node.js.

`~/belajar-k8s/hello-api/server.js`:

```js
const http = require('http');
const os = require('os');

const version = process.env.APP_VERSION || 'dev';
const greeting = process.env.GREETING || 'halo';

http.createServer((req, res) => {
  if (req.url === '/healthz') return res.end('ok');
  res.setHeader('content-type', 'application/json');
  res.end(JSON.stringify({ message: `${greeting} dari kubernetes`, version, pod: os.hostname() }) + '\n');
}).listen(process.env.PORT || 3000, () => console.log(`listening, version ${version}`));
```

Field `pod: os.hostname()` berisi nama pod. Ini akan membantumu melihat request mana dilayani pod mana.

`~/belajar-k8s/hello-api/Dockerfile`:

```dockerfile
FROM node:22-alpine
WORKDIR /app
COPY server.js .
ARG APP_VERSION=dev
ENV APP_VERSION=$APP_VERSION
EXPOSE 3000
USER node
CMD ["node", "server.js"]
```

Build dan masukkan ke cluster:

```bash
cd ~/belajar-k8s/hello-api
docker build -t hello-api:v1 --build-arg APP_VERSION=v1 .
kind load docker-image hello-api:v1 --name belajar
```

> Di Lab 1 kita belum punya registry, jadi `kind load` menyalin image langsung ke setiap node. Di kantor, langkah ini digantikan `docker push` ke registry 10.20.0.22:5000, lalu worker menarik image dari sana.

---

## Langkah 3: Tulis manifest Kubernetes

`~/belajar-k8s/hello-api/k8s.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: demo
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-api
  namespace: demo
spec:
  replicas: 3                       # mau 3 salinan
  selector:
    matchLabels:
      app: hello-api                # Deployment mengelola pod berlabel ini
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0             # jangan kurangi pod lama sebelum yang baru siap
      maxSurge: 1                   # boleh tambah 1 pod ekstra selama update
  template:                         # "cetakan" pod
    metadata:
      labels:
        app: hello-api
    spec:
      containers:
        - name: hello-api           # nama container, dipakai di `set image hello-api=...`
          image: hello-api:v1
          ports:
            - containerPort: 3000
          readinessProbe:           # pod baru menerima traffic hanya setelah ini sukses
            httpGet: { path: /healthz, port: 3000 }
            periodSeconds: 3
          livenessProbe:            # kalau ini gagal terus, container di-restart
            httpGet: { path: /healthz, port: 3000 }
            initialDelaySeconds: 5
          resources:
            requests: { cpu: 50m, memory: 64Mi }    # jatah minimum, dipakai scheduler
            limits:   { cpu: 200m, memory: 128Mi }  # batas atas; lewat batas memori = OOMKilled
---
apiVersion: v1
kind: Service
metadata:
  name: hello-api
  namespace: demo
spec:
  selector:
    app: hello-api                  # kirim traffic ke pod berlabel ini
  ports:
    - port: 80                      # port Service
      targetPort: 3000              # port container
```

Cara membacanya:
- **Label** `app: hello-api` adalah "lem" yang menghubungkan semuanya: Deployment → pod → Service.
- `requests` adalah jatah yang dijamin. Scheduler hanya menaruh pod di node yang sisa kapasitasnya cukup.
- `cpu: 50m` artinya 50 *millicore* = 5% dari 1 core.

Terapkan:

```bash
kubectl apply -f k8s.yaml
kubectl get pods -n demo -o wide
```

Lihat kolom `NODE`: pod tersebar di `belajar-worker` dan `belajar-worker2`, tidak ada yang di control-plane.

Supaya tidak perlu mengetik `-n demo` terus:

```bash
kubectl config set-context --current --namespace=demo
```

---

## Langkah 4: Panggil lewat Service (dan lihat load balancing)

Jalankan pod sementara berisi `curl` di dalam cluster:

```bash
kubectl run curl --rm -it --image=curlimages/curl -- sh
```

Di dalam shell pod itu:

```sh
for i in 1 2 3 4 5 6; do curl -s http://hello-api; done
curl -s http://hello-api.demo.svc.cluster.local
exit
```

Perhatikan field `pod` berganti-ganti: Service membagi request ke ketiga pod. Nama `hello-api.demo.svc.cluster.local` disediakan CoreDNS. Dengan cara yang sama `api-gateway` di kantor memanggil `lx-api`.

Alternatif dari Mac (hanya ke satu pod, untuk tes cepat):

```bash
kubectl port-forward svc/hello-api 8080:80
# terminal lain:
curl localhost:8080
```

---

## Langkah 5: Self-healing

Buka terminal kedua dan pantau:

```bash
kubectl get pods -w
```

Di terminal pertama, hapus satu pod:

```bash
kubectl delete pod <nama-salah-satu-pod>
```

Di terminal kedua kamu akan melihat pod baru langsung dibuat. Kamu tidak menyuruhnya. Controller melihat "keinginan 3, kenyataan 2", lalu memperbaikinya.

**Simulasi worker mati:**

```bash
docker stop belajar-worker2
kubectl get nodes -w        # tunggu belajar-worker2 jadi NotReady (±40–50 detik)
```

Pod di node itu tidak langsung pindah. Kubernetes menunggu ±5 menit (bisa jadi node-nya cuma sebentar putus). Setelah itu pod dibuat ulang di `belajar-worker`. Pantau dengan `kubectl get pods -o wide -w`.

Nyalakan lagi:

```bash
docker start belajar-worker2
```

Pod **tidak** otomatis kembali seimbang. Pod yang sudah jalan dibiarkan di tempatnya.

---

## Langkah 6: Scaling

```bash
kubectl scale deployment hello-api --replicas=6
kubectl get pods -o wide
kubectl scale deployment hello-api --replicas=3
```

> Di production, `replicas` sebaiknya diubah di file YAML lalu `kubectl apply`, supaya file dan cluster tidak berbeda (*drift*). `kubectl scale` cocok untuk darurat.

---

## Langkah 7: Rolling update (ini persis yang dilakukan CI kantor)

Ubah `server.js`, misalnya ganti `dari kubernetes` menjadi `dari kubernetes versi baru`. Lalu:

```bash
docker build -t hello-api:v2 --build-arg APP_VERSION=v2 .
kind load docker-image hello-api:v2 --name belajar
```

Di terminal kedua, jalankan curl terus-menerus:

```bash
kubectl run curl --rm -it --image=curlimages/curl -- sh -c 'while true; do curl -s http://hello-api.demo; sleep 0.5; done'
```

Di terminal pertama, deploy:

```bash
kubectl set image deployment/hello-api hello-api=hello-api:v2
kubectl rollout status deployment/hello-api
```

Perhatikan terminal kedua: `version` berubah dari `v1` ke `v2` sedikit demi sedikit, **tanpa ada request gagal**. Itulah zero-downtime deploy.

Bandingkan dengan perintah di CI kantor:

```
kubectl set image deployment/lx-api lx-api=<registry>/app/backend/api-layanan-x:<sha> -n app-backend
kubectl rollout status deployment/lx-api -n app-backend
```

Sama persis, hanya beda nama.

Lihat ReplicaSet-nya:

```bash
kubectl get rs -o wide
```

Ada 2 ReplicaSet: yang lama (DESIRED 0) dan yang baru (DESIRED 3). Yang lama disimpan untuk rollback.

---

## Langkah 8: Deploy gagal dan rollback

Deploy image yang **tidak ada**:

```bash
kubectl set image deployment/hello-api hello-api=hello-api:v99
kubectl get pods
```

Akan muncul satu pod baru berstatus `ErrImagePull` / `ImagePullBackOff`. **Tapi 3 pod lama masih Running dan melayani**, karena `maxUnavailable: 0` melarang Kubernetes mematikan pod lama sebelum pod baru siap. Cek curl di terminal kedua: masih jalan.

Cari tahu kenapa gagal:

```bash
kubectl describe pod <nama-pod-yang-error>    # lihat bagian Events paling bawah
```

Rollback:

```bash
kubectl rollout history deployment/hello-api
kubectl rollout undo deployment/hello-api                 # kembali ke versi sebelumnya
kubectl rollout undo deployment/hello-api --to-revision=1 # atau ke revisi tertentu
```

**Coba juga aplikasi yang crash.** Tambahkan `process.exit(1);` di baris pertama `server.js`, build sebagai `v3`, `kind load`, lalu `set image`. Statusnya akan `CrashLoopBackOff`. Lihat log-nya:

```bash
kubectl logs <nama-pod> --previous     # log dari container yang sudah mati
```

Lalu rollback lagi dan hapus `process.exit(1)`.

---

## Langkah 9: ConfigMap dan Secret

Konfigurasi sebaiknya tidak di-hardcode di image. Satu image yang sama bisa dipakai untuk staging dan production dengan konfigurasi berbeda.

```bash
kubectl create configmap hello-config --from-literal=GREETING=selamat-sore
kubectl create secret generic hello-secret --from-literal=S3_SECRET_KEY=rahasia123
```

Tambahkan ke container di `k8s.yaml` (sejajar dengan `ports:`):

```yaml
          envFrom:
            - configMapRef:
                name: hello-config
            - secretRef:
                name: hello-secret
```

```bash
kubectl apply -f k8s.yaml
kubectl run curl2 --rm -it --image=curlimages/curl -- curl -s http://hello-api.demo
```

(Pakai nama `curl2` karena pod `curl` dari langkah 7 mungkin masih jalan. Hentikan yang lama dengan Ctrl+C.)

Pesannya sekarang memakai `selamat-sore`.

Lihat bagaimana Secret disimpan:

```bash
kubectl get secret hello-secret -o yaml
echo 'cmFoYXNpYTEyMw==' | base64 -d
```

> **Penting:** Secret hanya di-*encode* base64, **bukan dienkripsi**. Siapa pun yang bisa `get secret` bisa membacanya. Karena itu izin akses ke cluster harus dibatasi, dan Secret jangan pernah di-commit ke Git dalam bentuk polos.

Kalau ConfigMap diubah, pod **tidak** otomatis membaca nilai baru. Lakukan restart:

```bash
kubectl rollout restart deployment/hello-api
```

---

## Langkah 10: Bersih-bersih

```bash
kind delete cluster --name belajar
```

---

## Cek pemahaman

- [ ] Kenapa pod tidak pernah jatuh ke node `control-plane`?
- [ ] Apa yang menghubungkan Service dengan pod-nya?
- [ ] Saat deploy `v99` gagal, kenapa layanan tetap hidup? Setting mana yang membuatnya begitu?
- [ ] Apa beda `readinessProbe` dan `livenessProbe`?
- [ ] Apa beda `kubectl logs` dan `kubectl logs --previous`?
- [ ] Kenapa Secret tidak boleh dianggap aman hanya karena base64?

Lanjut ke [[03 Lab 2 - Cluster seperti Kantor]].
