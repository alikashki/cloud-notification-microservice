# 02 · Phase 2 · Kubernetes

> Move the stack from Docker Compose to Kubernetes: namespaces, hand-made persistent storage, ConfigMaps and Secrets,
> Deployments with probes and limits, Services, and NetworkPolicies that reproduce the Phase 1 network rules.

[← Phase 1](01-phase1-docker.md) · [README](../README.md) · next: [Phase 3 · Microservice →](03-phase3-microservice.md)

**Files:** [`phase2-kubernetes/`](../phase2-kubernetes) · **Apply everything with:** [`apply.sh`](../phase2-kubernetes/apply.sh)

---

## The cluster

| | |
|---|---|
| Nodes | `cp1`, `cp2`, `cp3` (control plane, 192.168.10.1-3) · `worker1` (.6), `worker2` (.7) |
| Version / runtime | Kubernetes v1.36.3 · containerd 2.3.3 · Debian 13 |
| CNI | **Cilium** (enforces `NetworkPolicy`) |
| Storage | **No `StorageClass`**, no dynamic provisioner |
| Extras at the start | no metrics-server, no Ingress controller, no LoadBalancer |

Inspect your own cluster first, these answers decide the design:

```bash
kubectl get nodes -o wide --show-labels
kubectl version
kubectl get storageclass            # "No resources found" → we must create PVs by hand
kubectl get pods -A                 # which CNI? metrics-server? ingress?
```

## Requirements → solution

| Requirement | Solution |
|---|---|
| ≥ 3 nodes (1 master + 2 workers) | existing cluster: 3 control-plane + 2 workers |
| Deployments, Pods, Services | one Deployment + Service per component |
| Service and its DB in **separate namespaces** | namespace `auth` (auth + MySQL) and `core` (core + PostgreSQL + ActiveMQ) |
| ConfigMaps for settings | `auth-config`, `auth-db-config`, `core-config`, `core-db-config` |
| Secrets for credentials | `auth-db-secret`, `auth-secret`, `core-db-secret`, `activemq-secret`, `core-secret` |
| DB reachable only inside the cluster | `ClusterIP` Services, no NodePort on any data store |
| PVC + PV persistence | `StorageClass local-storage` + 3 PVs + 3 PVCs |
| Resource limits | `requests` and `limits` on every container |
| Each Deployment on a dedicated node | `nodeSelector: kubernetes.io/hostname` |
| Keep the Docker networking rules | `NetworkPolicy` (default-deny + explicit allows) |
| Not one single YAML file | one file per resource, one folder per namespace |

## Folder layout

```text
k8s/
├── 00-namespaces/   namespace-auth.yaml · namespace-core.yaml
├── 01-storage/      storageclass.yaml · pv-auth-db.yaml · pv-core-db.yaml · pv-activemq.yaml
├── 10-auth/         config · secret · pvc · deployments · services · np-*.yaml
└── 20-core/         config · secret · pvc · deployments · services · np-*.yaml
```

Numeric prefixes make the apply order obvious; [`apply.sh`](../phase2-kubernetes/apply.sh) applies ConfigMaps and
Secrets before everything else so Pods never start without their configuration.

## Step 2.1 · Namespaces and storage

### Storage concepts in one minute

| Object | Analogy | Role |
|---|---|---|
| **PersistentVolume (PV)** | a storage unit the building manager prepared | A piece of real storage; created by the cluster admin |
| **PersistentVolumeClaim (PVC)** | a tenant's request: "I need 5 GB" | A request for storage, written by the application owner, namespaced |
| **StorageClass** | the category of unit | A name that PVs and PVCs use to find each other |

Normally a provisioner creates a PV for every PVC. This cluster has none, so I create the PVs myself:

```yaml
# storageclass.yaml
provisioner: kubernetes.io/no-provisioner      # nobody creates volumes automatically
volumeBindingMode: WaitForFirstConsumer        # bind when the Pod is scheduled
```

```yaml
# pv-auth-db.yaml
spec:
  capacity: { storage: 5Gi }
  accessModes: [ReadWriteOnce]                  # one node reads/writes at a time
  persistentVolumeReclaimPolicy: Retain         # deleting the claim must NOT delete the data
  storageClassName: local-storage
  hostPath:
    path: /data/fum/auth-db
    type: DirectoryOrCreate                     # kubelet creates the folder → no manual mkdir over SSH
```

| PV | Size | Path on the node | Used by (pinned node) |
|---|---|---|---|
| `auth-db-pv` | 5 Gi | `/data/fum/auth-db` | MySQL → `worker1` |
| `core-db-pv` | 5 Gi | `/data/fum/core-db` | PostgreSQL → `worker2` |
| `activemq-pv` | 2 Gi | `/data/fum/activemq` | ActiveMQ → `worker2` |

> [!IMPORTANT]
> `hostPath` data lives on **one node**. The PV itself does not pin the Pod, so the Deployment's `nodeSelector` is what
> keeps the database on the node that holds its data. The `capacity` is only used for matching; `hostPath` does not
> enforce it.

```bash
kubectl apply -f k8s/00-namespaces/ -f k8s/01-storage/
kubectl get ns auth core ; kubectl get storageclass ; kubectl get pv     # PVs: Available
```

## Step 2.2 · Getting the images onto the nodes

Kubernetes **does not build images**, and cluster nodes use **containerd with its own image store**: an image you built
with `docker build` exists only in *your* Docker. The nodes also cannot reach Docker Hub reliably from this network.

The fix is to export from Docker and import into containerd's `k8s.io` namespace on every worker:

```bash
docker save auth:phase1 core:phase1 -o fum-images.tar
scp fum-images.tar root@192.168.10.6:/tmp/          # repeat for .7
ssh root@192.168.10.6 'ctr -n k8s.io images import /tmp/fum-images.tar && rm /tmp/fum-images.tar'
ssh root@192.168.10.6 'ctr -n k8s.io images ls -q | grep -E "auth:phase1|core:phase1"'
```

[`tools/load-image.sh`](../tools/load-image.sh) automates exactly this for any image. Two details:

- `-n k8s.io` matters: it is the containerd namespace the kubelet reads. Without it the image lands in `default` and
  the Pod still fails with `ErrImagePull`.
- With `imagePullPolicy: IfNotPresent` the kubelet uses the imported image. Public images (MySQL, PostgreSQL, ActiveMQ)
  are referenced through a mirror (`docker.arvancloud.ir/...`) that the nodes can reach; `latest` tags must set the
  policy explicitly because `latest` defaults to `Always`.

Before relying on a mirror, test from the node itself:

```bash
ssh root@192.168.10.6 'ctr -n k8s.io images pull docker.arvancloud.ir/library/alpine:3.20 && echo PULL-OK'
```

## Step 2.3 · ConfigMaps and Secrets

|  | ConfigMap | Secret |
|---|---|---|
| For | addresses, ports, database names | usernames, passwords, JWT key |
| Stored as | plain text | base64 (an encoding, **not** encryption) |

The assignment explicitly lists database **usernames** as sensitive, so they are in Secrets too.

| Namespace | Object | Keys |
|---|---|---|
| auth | ConfigMap `auth-config` | `PORT=":50051"` |
| auth | ConfigMap `auth-db-config` | `MYSQL_DATABASE` |
| auth | Secret `auth-db-secret` | `MYSQL_ROOT_PASSWORD`, `MYSQL_USER`, `MYSQL_PASSWORD` |
| auth | Secret `auth-secret` | `DB_URL` (full MySQL DSN), `JWT_SECRET_KEY` |
| core | ConfigMap `core-config` | JDBC URL, schema, ActiveMQ URL, `AUTH_GRPC_ADDRESS=auth.auth.svc.cluster.local`, … |
| core | ConfigMap `core-db-config` | `POSTGRES_DB` |
| core | Secrets `core-db-secret`, `activemq-secret`, `core-secret` | DB and broker credentials |

The key names are **exactly the environment-variable names** the applications read, so Deployments just use `envFrom`.
The same credential appears in more than one Secret because each Pod reads it under a different variable name; keep
the values identical.

Cross-namespace DNS: a Service is reachable as `<service>.<namespace>.svc.cluster.local`. Core therefore reaches the
auth gRPC server at `auth.auth.svc.cluster.local:50051`.

```bash
kubectl apply -f k8s/10-auth/config.yaml -f k8s/10-auth/secret.yaml -f k8s/20-core/config.yaml -f k8s/20-core/secret.yaml
kubectl get secret auth-secret -n auth -o jsonpath='{.data.JWT_SECRET_KEY}' | base64 -d   # base64 ≠ encryption
```

## Step 2.4 · Databases and ActiveMQ

For each of MySQL, PostgreSQL and ActiveMQ: a PVC, a Deployment and a ClusterIP Service.

| Component | Node | requests | limits | Probes |
|---|---|---|---|---|
| auth-db (MySQL) | worker1 | 100m / 512Mi | 1 CPU / 1Gi | `mysqladmin ping -h 127.0.0.1` |
| core-db (PostgreSQL) | worker2 | 100m / 256Mi | 500m / 512Mi | `pg_isready -h 127.0.0.1` |
| activemq | worker2 | 100m / 512Mi | 1 CPU / 1536Mi | TCP 61616 |

- **`strategy: Recreate`**: the default `RollingUpdate` starts the new Pod *before* stopping the old one, which would
  briefly run two database processes on one data directory.
- **`startupProbe`** gives first-time initialisation up to 3 minutes before liveness starts judging the container.
- **`requests`** is what the scheduler reserves; **`limits`** is the ceiling (CPU is throttled, memory overuse means
  `OOMKilled`). ActiveMQ's default heap is about 1 GiB, so its limit has to be higher than that.
- Services are `ClusterIP` only; the Service names (`auth-db`, `core-db`, `activemq`) are the host names already used in
  `DB_URL`, the JDBC URL and the broker URL.

## Step 2.5 · The two applications

| | auth | core |
|---|---|---|
| Node | worker1 | worker2 |
| `envFrom` | `auth-config`, `auth-secret` | `core-config`, `core-secret` |
| Init container | wait for `auth-db:3306` (`nc -z`) | wait for `core-db:5432` and `activemq:61616` |
| requests / limits | 50m·64Mi / 250m·256Mi | 250m·512Mi / 1 CPU·1Gi |
| Probes | TCP 8082 | HTTP `GET /docs` (`302`/`200` without a token) |

- **Init containers** run first, to completion. Both apps exit at start-up if their database is unreachable (the
  Kubernetes counterpart of `depends_on: service_healthy`). The init container reuses the app image (Alpine/BusyBox has
  `nc`) so no extra image is needed.
- **Probe choice for core:** `/actuator/health` would need a token (the security filter validates every URL except
  swagger/docs/webhook), so `/docs` is used.

### Why auth has two Services

| Service | Type | Ports | Purpose |
|---|---|---|---|
| `auth` | ClusterIP | 8082, **50051** | in-cluster access, including gRPC from core |
| `auth-http` | NodePort | 8082 → **30082** | external HTTP only |

A NodePort Service opens *every* port it lists on *every* node. Listing 50051 there would have exposed gRPC to the
network and violated the "only core may reach 50051" rule.

Core's NodePort is `30080`.

## Step 2.6 · NetworkPolicies (the Docker networks, again)

Kubernetes allows **all** Pod-to-Pod traffic by default. A Pod selected by any `NetworkPolicy` becomes closed for the
direction the policy covers, and then only explicit allows apply. Policies are **additive** (OR), and connections are
stateful (replies are automatic). Cilium enforces them.

Pattern in each namespace: **deny everything → allow DNS → allow only the required paths.**

| Policy | Effect |
|---|---|
| `default-deny-all` | blocks all ingress and egress for every Pod in the namespace |
| `allow-dns-egress` | every Pod may query CoreDNS (`kube-system`, `k8s-app=kube-dns`) on port 53 |
| `auth-db-ingress` | MySQL accepts only auth Pods on 3306 |
| `auth-traffic` | ingress: **50051 only from `core` Pods in namespace `core`**, 8082 from anywhere; egress: only `auth-db:3306` |
| `core-db-ingress` | PostgreSQL accepts only core Pods on 5432 |
| `activemq-ingress` | broker accepts only core Pods on 61616 |
| `core-traffic` | ingress: 8080 from anywhere; egress: `core-db:5432`, `activemq:61616`, **only gRPC 50051 of auth**, SMTP ports to the internet |

> [!CAUTION]
> `namespaceSelector` and `podSelector` inside the **same** `from:` item mean **AND** ("core Pods that are in the core
> namespace"). As two separate list items they mean **OR** and open far more than intended. This is the most common
> NetworkPolicy mistake.

The SMTP rule uses `ipBlock: 0.0.0.0/0`, which in Cilium matches only addresses *outside* the cluster, so Pods stay
unreachable. Remove it if you never send real e-mail.

## Step 2.7 · Verification

```bash
kubectl get pods,svc,pvc -n auth -o wide
kubectl get pods,svc,pvc -n core -o wide
kubectl get pv
```

Expected: every Pod `1/1 Running` on the intended node, every PVC and PV `Bound`, all data-store Services `ClusterIP`.

**Functionality through the NodePorts**

```bash
curl -i -X POST http://192.168.10.6:30082/user/register -H 'Content-Type: application/json' -d '{"email":"a@b.com","password":"Pass1234"}'
TOKEN=$(curl -s -X POST http://192.168.10.6:30082/user/login -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","password":"Pass1234"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')
curl -i http://192.168.10.6:30080/mail/configs -H "Authorization: $TOKEN"     # 200 = gRPC across namespaces works
```

Run the last request against **both** node IPs: NodePorts exist on every node, so traffic may have to cross to a Pod on
the other node.

**Isolation matrix** (`nc -z -w 3 host port`, `OK` = reachable)

| From | Expected `OK` | Expected `FAIL` |
|---|---|---|
| `auth` Pod | `auth-db:3306` | `core-db.core:5432`, `activemq.core:61616`, `core.core:8080` |
| `core` Pod | `core-db:5432`, `activemq:61616`, `auth.auth:50051` | `auth.auth:8082`, `auth-db.auth:3306` |
| a throw-away Pod in `default` | `auth.auth:8082`, `core.core:8080` (public ports) | `auth.auth:50051`, `auth-db`, `core-db`, `activemq` |

```bash
T='for t in "$@"; do h=${t%:*}; p=${t#*:}; nc -z -w 3 $h $p && echo "OK   $t" || echo "FAIL $t"; done'
kubectl exec -n auth deploy/auth -- sh -c "$T" _ auth-db:3306 core-db.core:5432 activemq.core:61616 core.core:8080
kubectl exec -n core deploy/core -- sh -c "$T" _ core-db:5432 activemq:61616 auth.auth:50051 auth.auth:8082 auth-db.auth:3306
```

**Persistence:** delete the database Pods, wait for the replacements, log in again.

```bash
kubectl delete pod -n auth -l app=auth-db ; kubectl delete pod -n core -l app=core-db ; kubectl delete pod -n core -l app=activemq
kubectl get pods -A -o wide | grep -E 'auth|core' | grep -v kube-system
curl -s -X POST http://192.168.10.6:30082/user/login -H 'Content-Type: application/json' -d '{"email":"a@b.com","password":"Pass1234"}'
ssh root@192.168.10.6 'ls /data/fum/auth-db | head'        # the data files on the node
```

Result: the same `userId` came back after the Pods were recreated. ✅

## Limitations

- Policies restrict *Pod* traffic. NodePorts are reachable from the whole node network; limit that with node firewalls.
- `hostPath` + `nodeSelector` means a node failure takes the data (and that database) down. Fine for this assignment.

[← Phase 1](01-phase1-docker.md) · [README](../README.md) · next: [Phase 3 · Microservice →](03-phase3-microservice.md)
