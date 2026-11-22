# 03 Lab 2: Membangun Cluster seperti Kantor (kubeadm di VM)

Kembali ke [[02 Lab 1 - Kubernetes di Laptop]] · Lanjut ke [[04 Lab 3 - CI-CD Deploy dari main]]

---

**Tujuan:** membangun cluster dengan cara yang **sama** seperti kantor: VM Ubuntu, containerd, kubeadm, Calico, MetalLB, ingress-nginx, registry sendiri, dan MinIO di luar cluster.

Di Lab 1, `kind` menyembunyikan banyak hal. Di sini kamu akan melihat dan mengerjakan sendiri semuanya.

## Topologi lab vs kantor

| VM lab | Peran | Padanan di kantor | CPU / RAM / Disk |
|---|---|---|---|
| `lab-cp1` | Control plane (+ runner deploy di Lab 3) | CP1–CP3 (10.20.0.10–12) | 2 / 4 GB / 20 GB |
| `lab-wk1` | Worker | WK1–WK5 (10.20.0.13–17) | 2 / 3 GB / 20 GB |
| `lab-wk2` | Worker | | 2 / 3 GB / 20 GB |
| `lab-infra` | Registry + MinIO (+ runner build di Lab 3) | Registry 10.20.0.22, MINIO1–3, HAProxy | 2 / 3 GB / 30 GB |

Total RAM ±13 GB dari 24 GB di Mac. Masih aman, tapi tutup aplikasi berat saat lab.

Alatnya **Multipass** (VM Ubuntu ringan dari Canonical). Berbeda dengan kind, setiap node di sini adalah VM sungguhan dengan kernel sendiri.

---

## Langkah 0: Buat VM

Di Mac:

```bash
brew install --cask multipass

multipass launch 24.04 --name lab-cp1   --cpus 2 --memory 4G --disk 20G
multipass launch 24.04 --name lab-wk1   --cpus 2 --memory 3G --disk 20G
multipass launch 24.04 --name lab-wk2   --cpus 2 --memory 3G --disk 20G
multipass launch 24.04 --name lab-infra --cpus 2 --memory 3G --disk 30G

multipass list
```

Catat IP-nya (biasanya `192.168.64.x`). Isi tabel ini untuk dirimu sendiri:

| VM | IP |
|---|---|
| lab-cp1 | 192.168.64.__ |
| lab-wk1 | 192.168.64.__ |
| lab-wk2 | 192.168.64.__ |
| lab-infra | 192.168.64.__ |

Masuk ke VM dengan `multipass shell <nama>`, keluar dengan `exit`.

> ⚠️ Kubernetes yang dibangun dengan kubeadm terikat ke IP node. Kalau IP VM berubah (jarang terjadi, tapi bisa kalau VM dihapus dan dibuat ulang), cluster rusak dan harus dibangun ulang. Di kantor IP-nya statis.

---

## Langkah 1: Siapkan ketiga node Kubernetes

Lakukan di **lab-cp1, lab-wk1, dan lab-wk2** (bukan lab-infra). Untuk node pertama, ketik satu per satu dan baca penjelasannya. Untuk node kedua dan ketiga boleh pakai script (lihat akhir langkah ini).

```bash
multipass shell lab-cp1
```

### 1a. Matikan swap

```bash
sudo swapoff -a
sudo sed -i '/ swap / s/^/#/' /etc/fstab
```

Secara default kubelet menolak jalan kalau swap aktif, karena perhitungan memori pod jadi tidak akurat.

### 1b. Modul kernel dan setting jaringan

```bash
cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
EOF
sudo modprobe overlay
sudo modprobe br_netfilter

cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF
sudo sysctl --system
```

- `overlay`: filesystem berlapis yang dipakai image container.
- `br_netfilter` + `bridge-nf-call-iptables`: supaya traffic antar-pod lewat bridge bisa diatur iptables (dibutuhkan kube-proxy).
- `ip_forward`: supaya node boleh meneruskan paket ke pod. Tanpa ini pod tidak bisa dijangkau.

### 1c. Pasang containerd (container runtime)

```bash
sudo apt-get update
sudo apt-get install -y containerd
containerd --version          # harus 1.7.x (Kantor: 1.7.28)

sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml > /dev/null
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
sudo sed -i 's#config_path = ""#config_path = "/etc/containerd/certs.d"#' /etc/containerd/config.toml
grep -nE 'SystemdCgroup|config_path' /etc/containerd/config.toml   # pastikan sudah berubah
sudo systemctl restart containerd
```

- `SystemdCgroup = true`: containerd dan kubelet harus memakai pengatur cgroup yang sama (systemd). Kalau beda, pod sering restart tanpa alasan jelas.
- `config_path`: folder untuk setting registry per-host. Kita butuh ini supaya node bisa menarik image dari registry HTTP di lab-infra.

> Kalau `containerd --version` menunjukkan 2.x, struktur config-nya berbeda. Cari panduan "containerd 2 SystemdCgroup config_path".

### 1d. Pasang kubeadm, kubelet, kubectl (v1.34, sama dengan kantor)

```bash
sudo apt-get install -y apt-transport-https ca-certificates curl gpg
sudo mkdir -p -m 755 /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.34/deb/Release.key \
  | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.34/deb/ /' \
  | sudo tee /etc/apt/sources.list.d/kubernetes.list
sudo apt-get update
sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl
sudo systemctl enable --now kubelet
```

- **kubeadm**: alat untuk membuat cluster (init / join / upgrade).
- **kubelet**: agen di setiap node. Saat ini ia akan restart terus, dan itu **normal** karena belum ada cluster.
- **kubectl**: CLI.
- `apt-mark hold`: mencegah `apt upgrade` menaikkan versi Kubernetes tanpa sengaja. Upgrade cluster harus dilakukan terencana dengan `kubeadm upgrade`.

Supaya `crictl` (alat lihat container di node) langsung tahu alamat containerd:

```bash
echo 'runtime-endpoint: unix:///run/containerd/containerd.sock' | sudo tee /etc/crictl.yaml
```

### 1e. Ulangi di worker

Simpan semua command 1a–1d dalam file `~/belajar-k8s/prep-node.sh` di Mac (tambahkan `set -e` di baris paling atas), lalu:

```bash
multipass transfer ~/belajar-k8s/prep-node.sh lab-wk1:/home/ubuntu/
multipass transfer ~/belajar-k8s/prep-node.sh lab-wk2:/home/ubuntu/
multipass exec lab-wk1 -- bash /home/ubuntu/prep-node.sh
multipass exec lab-wk2 -- bash /home/ubuntu/prep-node.sh
```

> 💡 **Snapshot dulu.** Sebelum lanjut, simpan keadaan VM supaya kalau rusak bisa diulang dari sini:
> ```bash
> multipass stop lab-cp1 lab-wk1 lab-wk2
> multipass snapshot lab-cp1 --name siap-init
> multipass snapshot lab-wk1 --name siap-init
> multipass snapshot lab-wk2 --name siap-init
> multipass start lab-cp1 lab-wk1 lab-wk2
> # restore nanti: multipass restore lab-cp1.siap-init
> ```

---

## Langkah 2: Nyalakan control plane

Di **lab-cp1**:

```bash
sudo kubeadm init --pod-network-cidr=10.244.0.0/16
```

`--pod-network-cidr` adalah rentang IP untuk pod. Pilih rentang yang **tidak bentrok** dengan jaringan VM (192.168.64.x). Kantor memakai `10.244.0.0/16`.

kubeadm akan:
1. Membuat sertifikat TLS untuk semua komponen (`/etc/kubernetes/pki/`).
2. Menulis manifest *static pod* untuk apiserver, etcd, scheduler, dan controller-manager di `/etc/kubernetes/manifests/`. Kubelet langsung menjalankan isi folder ini tanpa perlu API server.
3. Membuat kubeconfig admin di `/etc/kubernetes/admin.conf`.
4. Mencetak perintah `kubeadm join ...` di akhir. **Salin dan simpan perintah ini.**

Pasang kubeconfig untuk user `ubuntu`:

```bash
mkdir -p $HOME/.kube
sudo cp /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config

kubectl get nodes
```

Statusnya **`NotReady`**. Itu benar, karena belum ada jaringan pod (CNI).

Coba lihat static pod-nya:

```bash
ls /etc/kubernetes/manifests/
kubectl get pods -n kube-system
```

Pod `coredns` masih `Pending` karena menunggu CNI.

> Di kantor file yang sama ada di CP1–CP3: `/etc/kubernetes/manifests/kube-apiserver.yaml` dan seterusnya. Itulah kenapa kemarin di proses CP1 terlihat `kube-apiserver --advertise-address=10.20.0.10 ...`.

---

## Langkah 3: Pasang jaringan pod (Calico, sama dengan kantor)

Masih di lab-cp1:

```bash
CALICO=v3.30.3   # cek versi terbaru di https://github.com/projectcalico/calico/releases
kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/$CALICO/manifests/tigera-operator.yaml

curl -fsSLO https://raw.githubusercontent.com/projectcalico/calico/$CALICO/manifests/custom-resources.yaml
grep -n cidr custom-resources.yaml
sed -i 's#192.168.0.0/16#10.244.0.0/16#' custom-resources.yaml
grep -n cidr custom-resources.yaml   # harus 10.244.0.0/16
kubectl create -f custom-resources.yaml

watch kubectl get pods -n calico-system   # tunggu semua Running, keluar dengan Ctrl+C
kubectl get nodes                          # sekarang Ready
```

**Kenapa harus mengganti cidr?** Default Calico adalah `192.168.0.0/16`, yang bentrok dengan jaringan VM (`192.168.64.x`) dan juga jaringan kantor (`10.20.0.x`). Kalau bentrok, paket ke VM lain dikira paket ke pod, dan jaringan kacau.

---

## Langkah 4: Gabungkan worker

Di **lab-wk1** dan **lab-wk2**, jalankan perintah join yang tadi disimpan (pakai `sudo`):

```bash
sudo kubeadm join 192.168.64.__:6443 --token xxxxx --discovery-token-ca-cert-hash sha256:xxxxx
```

Kalau perintahnya hilang atau token-nya kedaluwarsa (berlaku 24 jam), buat ulang di lab-cp1:

```bash
sudo kubeadm token create --print-join-command
```

Kembali ke lab-cp1:

```bash
kubectl get nodes -o wide
kubectl label node lab-wk1 node-role.kubernetes.io/worker=
kubectl label node lab-wk2 node-role.kubernetes.io/worker=
kubectl get nodes        # kolom ROLES sekarang terisi "worker" (cuma kosmetik)
```

**Pakai kubectl dari Mac** (lebih nyaman):

```bash
multipass exec lab-cp1 -- sudo cat /etc/kubernetes/admin.conf > ~/.kube/lab-config
export KUBECONFIG=~/.kube/lab-config
kubectl get nodes
```

> `admin.conf` adalah kunci **cluster-admin**: bisa melakukan apa saja. Jangan dibagikan atau di-commit. Di kantor file ini hanya bisa dibaca root, karena itu kemarin perlu `sudo kubectl --kubeconfig /etc/kubernetes/admin.conf`.

---

## Langkah 5: Siapkan lab-infra (registry + MinIO)

Ini padanan VM Registry (10.20.0.22) dan MINIO1–3 di kantor. Sengaja **di luar cluster**.

```bash
multipass shell lab-infra
sudo apt-get update && sudo apt-get install -y docker.io
sudo usermod -aG docker ubuntu
exit
multipass shell lab-infra      # masuk ulang supaya grup docker berlaku
INFRA_IP=$(hostname -I | awk '{print $1}'); echo $INFRA_IP
```

### 5a. Registry

```bash
docker run -d --name registry --restart=always \
  -p 5000:5000 -v /srv/registry:/var/lib/registry registry:2
```

Registry ini memakai HTTP polos (sama seperti kantor). Docker menolak push ke HTTP kecuali diizinkan:

```bash
echo "{\"insecure-registries\": [\"$INFRA_IP:5000\"]}" | sudo tee /etc/docker/daemon.json
sudo systemctl restart docker
```

Kirim folder aplikasi dari Lab 1 (jalankan di Mac):

```bash
multipass transfer -r ~/belajar-k8s/hello-api lab-infra:/home/ubuntu/
```

Kembali di lab-infra, build dan push:

```bash
cd ~/hello-api
docker build -t $INFRA_IP:5000/demo/hello-api:v1 --build-arg APP_VERSION=v1 .
docker push $INFRA_IP:5000/demo/hello-api:v1

curl http://$INFRA_IP:5000/v2/_catalog                 # {"repositories":["demo/hello-api"]}
curl http://$INFRA_IP:5000/v2/demo/hello-api/tags/list # {"name":"demo/hello-api","tags":["v1"]}
```

Format nama image: `<alamat-registry>/<path>/<nama>:<tag>`. Bandingkan dengan kantor: `10.20.0.22:5000/app/backend/api-layanan-x:<sha>`.

### 5b. Izinkan node k8s menarik dari registry HTTP

Di **lab-cp1, lab-wk1, lab-wk2** (ganti `INFRA_IP`):

```bash
INFRA_IP=192.168.64.__
sudo mkdir -p /etc/containerd/certs.d/$INFRA_IP:5000
cat <<EOF | sudo tee /etc/containerd/certs.d/$INFRA_IP:5000/hosts.toml
server = "http://$INFRA_IP:5000"

[host."http://$INFRA_IP:5000"]
  capabilities = ["pull", "resolve"]
EOF
```

Tes dari worker: `sudo crictl pull $INFRA_IP:5000/demo/hello-api:v1`

### 5c. MinIO

Di lab-infra:

```bash
docker run -d --name minio --restart=always \
  -p 9000:9000 -p 9001:9001 -v /srv/minio:/data \
  -e MINIO_ROOT_USER=admin -e MINIO_ROOT_PASSWORD='ganti-dengan-password-panjang' \
  quay.io/minio/minio server /data --console-address ":9001"
```

- Port **9000** = S3 API (dipakai aplikasi). Port **9001** = web console. Sama dengan kantor.
- Lab ini MinIO 1 node 1 disk. Kantor memakai 3 node × 2 disk (mode distributed). Cara aplikasi memakainya sama persis.

Buat bucket dan user khusus aplikasi memakai `mc` (MinIO client):

```bash
docker run --rm --network host --entrypoint sh minio/mc -c '
  mc alias set lab http://127.0.0.1:9000 admin "ganti-dengan-password-panjang" &&
  mc mb lab/belajar &&
  mc admin user add lab hello-app "password-app-yang-panjang" &&
  mc admin policy attach lab readwrite --user hello-app &&
  mc ls lab'
```

> Aplikasi **jangan** memakai akun root MinIO. Buat user/access key sendiri per aplikasi. Itulah fungsi `S3_ACCESS_KEY` / `S3_SECRET_KEY` di env kantor.

Buka console dari browser Mac: `http://<INFRA_IP>:9001`.

---

## Langkah 6: MetalLB (IP untuk Service LoadBalancer)

Di cloud, `type: LoadBalancer` otomatis mendapat IP dari AWS/GCP. Di server sendiri seperti kantor, tidak ada yang memberi IP itu, sehingga Service-nya macet di `<pending>` selamanya. MetalLB mengisi kekosongan itu: ia mengambil IP dari daftar yang kamu tentukan, lalu "mengumumkan" lewat ARP bahwa IP itu ada di salah satu node.

Dari Mac (`KUBECONFIG=~/.kube/lab-config`) atau di lab-cp1:

```bash
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.14.9/config/manifests/metallb-native.yaml
kubectl wait -n metallb-system --for=condition=ready pod -l app=metallb --timeout=180s
```

Buat `metallb-pool.yaml`. Pilih rentang IP di subnet VM yang tidak dipakai VM lain:

```yaml
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: lab-pool
  namespace: metallb-system
spec:
  addresses:
    - 192.168.64.240-192.168.64.250
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: lab-l2
  namespace: metallb-system
spec:
  ipAddressPools:
    - lab-pool
```

```bash
kubectl apply -f metallb-pool.yaml
```

Di kantor, proses `controller --port=7472` (di WK1) dan `speaker --port=7472` (di semua node) adalah MetalLB.

---

## Langkah 7: ingress-nginx (pintu masuk HTTP)

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.12.1/deploy/static/provider/cloud/deploy.yaml
kubectl wait -n ingress-nginx --for=condition=ready pod -l app.kubernetes.io/component=controller --timeout=180s
kubectl get svc -n ingress-nginx ingress-nginx-controller
```

Kolom `EXTERNAL-IP` sekarang berisi IP dari MetalLB, misalnya `192.168.64.240`. Inilah satu-satunya pintu masuk untuk semua aplikasi HTTP di cluster.

> Ingat: ingress-nginx sudah pensiun sejak Maret 2026. Kita tetap memakainya karena kantor memakainya. Setelah lab ini selesai, coba ulangi langkah 7 dan 8 memakai Gateway API (misalnya Envoy Gateway) sebagai latihan lanjutan.

---

## Langkah 8: Deploy hello-api seperti aplikasi kantor

Buat folder `~/belajar-k8s/hello-api/k8s/` di Mac. Manifest **disimpan di Git** bersama kode (praktik yang lebih baik dari kantor).

`k8s/00-namespace.yaml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: demo
```

`k8s/10-config.yaml` (yang tidak rahasia):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hello-config
  namespace: demo
data:
  GREETING: "halo"
  S3_ENDPOINT: "http://192.168.64.__:9000"   # IP lab-infra
  S3_BUCKET: "belajar"
```

Secret dibuat lewat command, **bukan** file di Git:

```bash
kubectl apply -f k8s/00-namespace.yaml
kubectl -n demo create secret generic hello-s3 \
  --from-literal=S3_ACCESS_KEY=hello-app \
  --from-literal=S3_SECRET_KEY='password-app-yang-panjang'
```

`k8s/20-deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-api
  namespace: demo
spec:
  replicas: 3
  revisionHistoryLimit: 5          # simpan 5 ReplicaSet lama untuk rollback
  selector:
    matchLabels:
      app: hello-api
  strategy:
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
  template:
    metadata:
      labels:
        app: hello-api
    spec:
      containers:
        - name: hello-api
          image: 192.168.64.__:5000/demo/hello-api:v1   # IP lab-infra
          ports:
            - containerPort: 3000
          envFrom:
            - configMapRef: { name: hello-config }
            - secretRef:    { name: hello-s3 }
          readinessProbe:
            httpGet: { path: /healthz, port: 3000 }
            periodSeconds: 3
          livenessProbe:
            httpGet: { path: /healthz, port: 3000 }
            initialDelaySeconds: 5
          resources:
            requests: { cpu: 50m, memory: 64Mi }
            limits:   { cpu: 200m, memory: 128Mi }
```

`k8s/30-service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: hello-api
  namespace: demo
spec:
  type: ClusterIP            # hanya bisa diakses dari dalam cluster; dari luar lewat Ingress
  selector:
    app: hello-api
  ports:
    - port: 80
      targetPort: 3000
```

`k8s/40-ingress.yaml`:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: hello-api
  namespace: demo
spec:
  ingressClassName: nginx
  rules:
    - host: hello.lab.local
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: hello-api
                port:
                  number: 80
```

Terapkan semuanya (kubectl membaca file sesuai urutan nama, karena itu ada awalan angka):

```bash
kubectl apply -f k8s/
kubectl -n demo get deploy,rs,pods,svc,ingress -o wide
```

Tes dari Mac:

```bash
curl -H 'Host: hello.lab.local' http://192.168.64.240/
# atau supaya bisa dibuka di browser:
echo '192.168.64.240 hello.lab.local' | sudo tee -a /etc/hosts
curl http://hello.lab.local/
```

Jalur request-nya sama dengan kantor: **Mac → IP MetalLB → ingress-nginx → Service `hello-api` → salah satu pod**.

---

## Langkah 9: Pastikan pod bisa menjangkau MinIO

Aplikasi NestJS kantor memakai SDK S3 dengan `S3_ENDPOINT`. Kita cukup membuktikan bahwa pod bisa sampai ke MinIO di luar cluster:

```bash
kubectl -n demo run mc --rm -it --image=minio/mc --command -- sh
```

Di dalam pod:

```sh
mc alias set lab http://192.168.64.__:9000 hello-app 'password-app-yang-panjang'
echo "halo dari pod" > /tmp/tes.txt
mc cp /tmp/tes.txt lab/belajar/
mc ls lab/belajar
exit
```

Cek juga di console MinIO (`http://<INFRA_IP>:9001`): file `tes.txt` muncul.

Pod memakai IP 10.244.x.x, sedangkan MinIO ada di 192.168.64.x. Calico melakukan NAT supaya paket dari pod keluar dengan IP node. Di kantor, dengan cara inilah pod menjangkau `10.20.0.37:9000` dan `:5434`.

---

## Langkah 10 (opsional, lanjutan): Control plane HA seperti kantor

Kantor memakai 3 CP dan VIP `10.20.0.38` yang dipegang keepalived. Untuk mencobanya kamu perlu 2 VM CP tambahan (`lab-cp2`, `lab-cp3`, masing-masing 2 CPU / 2.5 GB) dan **membangun ulang** cluster, karena alamat VIP harus ditentukan sejak `kubeadm init`.

Ringkasan langkahnya:

1. Siapkan lab-cp1..3 dengan Langkah 1.
2. Pasang keepalived di ketiga CP (`sudo apt-get install -y keepalived`), isi `/etc/keepalived/keepalived.conf`:
   ```
   vrrp_instance K8S_API {
       state MASTER              # BACKUP di cp2 dan cp3
       interface enp0s1          # cek nama interface dengan: ip -br a
       virtual_router_id 51
       priority 101              # 100 di cp2, 99 di cp3
       advert_int 1
       authentication {
           auth_type PASS
           auth_pass lab12345
       }
       virtual_ipaddress {
           192.168.64.200/24     # VIP, pilih IP yang tidak dipakai
       }
   }
   ```
   `sudo systemctl enable --now keepalived`, lalu cek `ip -br a` di cp1: IP .200 muncul.
3. Init di cp1 memakai VIP:
   ```bash
   sudo kubeadm init --control-plane-endpoint=192.168.64.200:6443 \
     --upload-certs --pod-network-cidr=10.244.0.0/16
   ```
   Output-nya mencetak **dua** perintah join: satu dengan `--control-plane --certificate-key ...` (untuk cp2/cp3) dan satu biasa (untuk worker).
4. Jalankan join control-plane di cp2 dan cp3, lalu join biasa di worker.
5. Uji: `multipass stop lab-cp1`. VIP pindah ke cp2 dan `kubectl get nodes` masih jalan (arahkan kubeconfig ke VIP).

Konfigurasi ini memindahkan VIP hanya kalau **mesin** CP mati. Versi yang lebih baik menambahkan `track_script` yang mengecek apakah kube-apiserver lokal masih sehat.

---

## Pemetaan akhir: lab kamu vs kantor

| Komponen | Lab | Kantor |
|---|---|---|
| Hypervisor | Multipass di Mac | VM on-premise (10.20.0.x) |
| OS | Ubuntu 24.04 | Ubuntu |
| Runtime | containerd 1.7 | containerd 1.7.28 |
| Kubernetes | kubeadm v1.34 | kubeadm v1.34.2 |
| Control plane | 1 CP (opsional 3 + VIP) | 3 CP + VIP keepalived 10.20.0.38 |
| Worker | 2 | 5 |
| CNI | Calico, 10.244.0.0/16 | Calico, 10.244.0.0/16, IPIP |
| LoadBalancer | MetalLB L2 | MetalLB |
| Ingress | ingress-nginx | ingress-nginx |
| Storage di cluster | (tidak ada) | Longhorn v1.7.3 |
| Registry | registry:2 di lab-infra | registry di 10.20.0.22:5000 |
| Object storage | MinIO 1 node | MinIO 3 node × 2 disk via HAProxy VIP 10.20.0.37 |
| Database | (tidak ada) | Patroni PostgreSQL via HAProxy VIP 10.20.0.37 |

---

## Bersih-bersih

```bash
multipass delete --purge lab-cp1 lab-wk1 lab-wk2 lab-infra
```

Tapi jangan dihapus dulu, Lab 3 masih memakai cluster ini.

## Cek pemahaman

- [ ] Kenapa node `NotReady` sebelum Calico dipasang?
- [ ] Apa isi `/etc/kubernetes/manifests/`, dan siapa yang menjalankannya?
- [ ] Kenapa `--pod-network-cidr` tidak boleh bentrok dengan jaringan VM?
- [ ] Tanpa MetalLB, apa yang terjadi pada Service `type: LoadBalancer` di server on-premise?
- [ ] Bagaimana pod (IP 10.244.x.x) bisa menjangkau MinIO (IP 192.168.64.x)?
- [ ] Kenapa VIP harus ditentukan saat `kubeadm init`, bukan belakangan?

Lanjut ke [[04 Lab 3 - CI-CD Deploy dari main]].
