# Troubleshooting log

> Every problem I met while building this project, what caused it, how I fixed it and what I took away.
> 🔴 = it actually broke something · 🟡 = caught early (in review, while reading the code, or in design)

[← README](../README.md) · [Command reference →](COMMANDS.md)

---

## Index

| # | Problem | Phase | |
|---|---|---|---|
| 1 | [Docker build fails resolving docker/dockerfile:1](#1-docker-build-fails-resolving-dockerdockerfile1) | 1 | 🔴 |
| 2 | [go mod download fails with 403 Forbidden](#2-go-mod-download-fails-with-403-forbidden) | 1 | 🔴 |
| 3 | [Images much bigger than expected](#3-images-much-bigger-than-expected) | 1 | 🔴 |
| 4 | [Which database does the auth service use?](#4-which-database-does-the-auth-service-use) | 0 / 1 | 🟡 |
| 5 | [Core returns 401 although the token is valid](#5-core-returns-401-although-the-token-is-valid) | 1 / 2 | 🔴 |
| 6 | [actuator/health returns 500 and cannot be used for probes](#6-actuatorhealth-returns-500-and-cannot-be-used-for-probes) | 2 | 🟡 |
| 7 | [I designed for Kind, but a real cluster was available](#7-i-designed-for-kind-but-a-real-cluster-was-available) | 2 | 🔴 |
| 8 | [Pods cannot find the images I built](#8-pods-cannot-find-the-images-i-built) | 2 | 🔴 |
| 9 | [No StorageClass: nothing can provision the volumes](#9-no-storageclass-nothing-can-provision-the-volumes) | 2 | 🟡 |
| 10 | [quantities must match the regular expression](#10-quantities-must-match-the-regular-expression) | 2 | 🔴 |
| 11 | [user-manager Pod stuck in ErrImagePull](#11-user-manager-pod-stuck-in-errimagepull) | 3 | 🔴 |
| 12 | [metrics-server image cannot be pulled](#12-metrics-server-image-cannot-be-pulled) | 4 | 🔴 |
| 13 | [metrics-server: x509 certificate errors](#13-metrics-server-x509-certificate-errors) | 4 | 🔴 |
| 14 | [kubectl and the cluster differ by four minor versions](#14-kubectl-and-the-cluster-differ-by-four-minor-versions) | 4 | 🔴 |
| 15 | [Workers are small: memory capacity limits autoscaling](#15-workers-are-small-memory-capacity-limits-autoscaling) | 4 | 🔴 |
| 16 | [HPA and the replicas field fight](#16-hpa-and-the-replicas-field-fight) | 4 | 🟡 |
| 17 | [NGINX does not see Pods created by the autoscaler](#17-nginx-does-not-see-pods-created-by-the-autoscaler) | 4 | 🟡 |
| 18 | [Health checks pass too early (socket versus TCP)](#18-health-checks-pass-too-early-socket-versus-tcp) | 1 | 🟡 |
| 19 | [Compose draft: published ports unreachable, 0.0.0.0 binding, missing variables](#19-compose-draft-published-ports-unreachable-0000-binding-missing-variables) | 1 | 🟡 |
| 20 | [A NodePort Service exposes every port it lists](#20-a-nodeport-service-exposes-every-port-it-lists) | 2 | 🟡 |
| 21 | [NetworkPolicy selectors: AND versus OR](#21-networkpolicy-selectors-and-versus-or) | 2 | 🟡 |
| 22 | [Repeated SSH password prompts](#22-repeated-ssh-password-prompts) | 2 | 🔴 |
| 23 | [Mirrors, image names and the latest tag](#23-mirrors-image-names-and-the-latest-tag) | 2 | 🟡 |
| 24 | [Windows binary and a placeholder env file inside the repository](#24-windows-binary-and-a-placeholder-env-file-inside-the-repository) | 1 | 🟡 |

---

### 1. Docker build fails resolving docker/dockerfile:1

**Phase:** 1 · 🔴 hit while working

**Symptom**

```text
failed to resolve source metadata for docker.io/docker/dockerfile:1:
failed to do request: Head "https://registry-1.docker.io/v2/docker/dockerfile/manifests/1": dial tcp ...:443: connect: connection refused
```
The very first build had worked; later builds failed before reading a single instruction.

**Cause**

The first line of my Dockerfiles was `# syntax=docker/dockerfile:1`. That directive tells BuildKit to download the
*Dockerfile frontend image* from Docker Hub and to re-check it on **every** build. It is not covered by the registry mirror
configured for normal images, and Docker Hub is not reachable directly.

**Fix**

Delete the line from both Dockerfiles. Current Docker releases ship a frontend that already supports everything used here,
including `RUN --mount=type=cache`.

> 💡 **Lesson:** Every external reference in a build is a dependency. Drop the optional ones, especially on a restricted network.

---

### 2. go mod download fails with 403 Forbidden

**Phase:** 1 · 🔴 hit while working

**Symptom**

```text
go: google.golang.org/genproto@v0.0.0-20220519153652-3a47de7e79bd:
reading https://proxy.golang.org/google.golang.org/genproto/@v/....zip: 403 Forbidden
```
(`docker build` of the auth service stops at `RUN go mod download`.)

**Cause**

`proxy.golang.org` refuses requests coming from Iran, so no module can be downloaded.

**Fix**

Use Go module mirrors and turn off the checksum database (also unreachable). Integrity is still verified against the
committed `go.sum`:

```dockerfile
ARG GOPROXY=https://mirror.abrha.net/repository/go/|https://mirror-go.runflare.com|https://package-mirror.liara.ir/repository/go/|direct
ARG GOSUMDB=off
```

Two details that matter:

- Separate the entries with **`|`**, not `,`. A comma only falls through to the next entry on `404`/`410`; a pipe falls
  through on *any* error, which is what happens when a mirror is down or returns `403`.
- The last entry is `direct`, not `directs` (a typo in my first draft). An unknown keyword makes Go reject the setting.

Because they are `ARG`s they can be overridden without editing the file:
`docker build --build-arg GOPROXY='https://mirror-go.runflare.com|direct' .`

> 💡 **Lesson:** Know which package registries are reachable *before* writing the Dockerfile, and make the mirror list overridable.

---

### 3. Images much bigger than expected

**Phase:** 1 · 🔴 hit while working

**Symptom**

```text
IMAGE         DISK USAGE   CONTENT SIZE
auth:phase1       55.9MB         16.1MB
core:phase1        556MB          210MB
```
The core image was far larger than a JRE plus one jar should be.

**Cause**

I had added `RUN chown app:app app.jar` (and `chown -R app:app /app` for auth) after `COPY`. Docker layers are immutable:
changing a file's owner writes **a complete second copy** of the file into a new layer, and the first copy stays in the
image underneath.

**Fix**

Remove the `chown` lines. The runtime user only needs to read and execute the files, which are world-readable already.
(If ownership is really needed, use `COPY --chown=user:group`, which adds no layer.)

| Image | Before | After |
|---|---|---|
| auth | 55.9 MB | **34 MB** |
| core | 556 MB | **406 MB** |

`docker history <image>` shows two layers of the same size when this happens.

> 💡 **Lesson:** Never `chown`/`chmod` a big file in a separate layer. Look at `docker history` when an image is heavier than expected.

---

### 4. Which database does the auth service use?

**Phase:** 0 / 1 · 🟡 caught early

**Symptom**

The assignment text, the sample `envs/dev.env` and the repository's own `docker-compose.yml` disagreed (MySQL on page 1 of
the PDF, PostgreSQL placeholders in the sample files), and a note suggested page 1 had "mysql" by mistake.

**Cause**

The sample files are leftovers from an earlier version of the project.

**Fix**

Read `app/db/db.go`: it opens the connection with `gorm.io/driver/mysql`. **Auth uses MySQL**, Core uses PostgreSQL, so
the DSN has to be in MySQL form: `user:pass@tcp(host:3306)/db?charset=utf8mb4&parseTime=True&loc=Local`. The PDF was
right; the sample files were stale.

> 💡 **Lesson:** The code is the source of truth. Read it before building the infrastructure around it.

---

### 5. Core returns 401 although the token is valid

**Phase:** 1 / 2 · 🔴 hit while working

**Symptom**

```bash
curl -i localhost:8080/mail/configs -H "Authorization: Bearer $TOKEN"     # 401  "unauthorized"
curl -i localhost:8080/mail/configs                                       # 500
```
Logging in at the auth service worked fine.

**Cause**

Core's security filter forwards the `Authorization` header **unchanged** to the auth service's gRPC `Validate`, which
expects the **raw token**. Only the auth service's HTTP endpoint `/user/validate` strips the `Bearer ` prefix. Swagger
UI in Core is also configured as an API-key header, not a bearer scheme. The `500` without a header comes from
building the gRPC request with a `null` token.

**Fix**

Send the raw token to Core: `-H "Authorization: $TOKEN"` (and `Bearer <token>` only to the auth service).

A `401` here is actually good news: it proves the gRPC call from Core to Auth works (a broken connection would have
produced a `500`).

> 💡 **Lesson:** Test an integration with the exact header format the *receiving* code expects, not the one a neighbouring service uses.

---

### 6. actuator/health returns 500 and cannot be used for probes

**Phase:** 2 · 🟡 caught early

**Symptom**

```bash
curl -i localhost:8080/actuator/health        # 500, JSON error
```

**Cause**

`/actuator/**` is `permitAll` in the security configuration, but the custom authentication filter still validates a token
for every URL that does not contain `swagger`, `docs` or `webhook`.

**Fix**

Use `GET /docs` for Kubernetes probes: it answers `302`/`200` without a token, and kubelet treats 200-399 as success.
(Auth has no health endpoint, so a TCP probe on 8082 is used.)

> 💡 **Lesson:** Check what a probe endpoint requires before choosing it.

---

### 7. I designed for Kind, but a real cluster was available

**Phase:** 2 · 🔴 hit while working

**Symptom**

Phase 2 started with a `kind-config.yaml`, `kind create cluster` and `kind load docker-image` instructions. The cluster
already existed.

**Cause**

The assignment says Kind is *preferred* and I took it as the plan without asking what environment I was designing for.

**Fix**

Dropped Kind, deleted the config and inspected the real cluster instead:

```bash
kubectl get nodes -o wide --show-labels
kubectl version
kubectl get storageclass
kubectl get pods -A
```
The answers (Cilium, no StorageClass, containerd, two workers, restricted internet) changed storage, networking, image
delivery and node placement.

> 💡 **Lesson:** Ask about, or inspect, the target environment first: nodes, CNI, storage, registry access.

---

### 8. Pods cannot find the images I built

**Phase:** 2 · 🔴 hit while working

**Symptom**

Application Pods would fail with `ErrImagePull`; `kind load docker-image` (the tool that solves this for Kind) does not
apply to a normal cluster.

**Cause**

`docker build` stores the image in *your* Docker. Cluster nodes run **containerd** with a separate image store, and cannot
pull `auth:phase1` from anywhere because it exists in no registry. Public images are also unreliable from the nodes
(Docker Hub is blocked).

**Fix**

Export from Docker and import into containerd's **`k8s.io`** namespace on every worker
([`tools/load-image.sh`](../tools/load-image.sh) automates it):

```bash
docker save auth:phase1 core:phase1 -o fum-images.tar
scp fum-images.tar root@192.168.10.6:/tmp/
ssh root@192.168.10.6 'ctr -n k8s.io images import /tmp/fum-images.tar && rm /tmp/fum-images.tar'
ssh root@192.168.10.6 'ctr -n k8s.io images ls -q | grep -E "auth|core"'
```
Without `-n k8s.io` the image lands in the `default` containerd namespace, which the kubelet does not read. With
`imagePullPolicy: IfNotPresent` the Pod then starts from the local image. A private registry would be the long-term fix.

> 💡 **Lesson:** A cluster's nodes and your workstation do not share an image store. Decide how images reach the nodes before writing manifests.

---

### 9. No StorageClass: nothing can provision the volumes

**Phase:** 2 · 🟡 caught early

**Symptom**

`kubectl get storageclass` → `No resources found`. A PVC would stay `Pending` forever.

**Cause**

The cluster has no dynamic provisioner.

**Fix**

Create the storage by hand: a `StorageClass` with `provisioner: kubernetes.io/no-provisioner` and
`volumeBindingMode: WaitForFirstConsumer`, three `hostPath` PersistentVolumes (`type: DirectoryOrCreate`, so kubelet creates
the folder and no manual `mkdir` is needed), `persistentVolumeReclaimPolicy: Retain`, and PVCs that name their PV with
`volumeName`. Because `hostPath` data sits on one node, each database Deployment is pinned there with a `nodeSelector`.

> 💡 **Lesson:** Adjust the design to what the cluster actually provides; make the node-affinity of local data explicit.

---

### 10. quantities must match the regular expression

**Phase:** 2 · 🔴 hit while working

**Symptom**

```text
Error from server (BadRequest): error when creating "k8s/20-core/deployment-activemq.yaml":
quantities must match the regular expression '^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$'
```
The other manifests of the same `kubectl apply` were created normally.

**Cause**

A value under `resources:` was not a valid Kubernetes quantity. Valid units are `m` for CPU and `Ki Mi Gi` (binary) or
`K M G` (decimal) for memory; **`MB`, `GB`, `Mb`, `Gib` are invalid**, and so are curly quotes or invisible characters that
a copy-and-paste can introduce. The file as generated (checked byte by byte) was valid, so the copy on the machine had
been altered; the exact edit was not identified.

**Fix**

Restore the original values (`cpu: 100m`, `memory: 512Mi`, `cpu: "1"`, `memory: 1536Mi`) or re-download the file, and
validate before applying:

```bash
kubectl apply --dry-run=client -f k8s/20-core/deployment-activemq.yaml
```
`kubectl apply` keeps going after an error, so check which objects exist (`kubectl get deploy -n core`) and re-apply the folder.

> 💡 **Lesson:** Dry-run manifests before applying them, and `cat -A` a file when a value looks right but is rejected.

---

### 11. user-manager Pod stuck in ErrImagePull

**Phase:** 3 · 🔴 hit while working

**Symptom**

```text
Failed to pull image "user-manager:phase3": ... "docker.io/library/user-manager:phase3": ... pull access denied,
repository does not exist or may require authorization: server message: insufficient_scope: authorization failed
```
`docker images | grep user-manager` and `ctr -n k8s.io images ls | grep user-manager` on the node were both **empty**.

**Cause**

The image was never built and never imported, so the kubelet fell back to `docker.io/library/...`. The error text is
misleading: Docker Hub answers `insufficient_scope` for a repository that does not exist, which is not an
authentication problem. The manifests were fine.

**Fix**

```bash
cd phase3-microservice/user-manager
docker build -t user-manager:phase3 .
docker save user-manager:phase3 -o user-manager.tar
for n in 192.168.10.6 192.168.10.7; do
  scp user-manager.tar root@$n:/tmp/
  ssh root@$n 'ctr -n k8s.io images import /tmp/user-manager.tar && rm /tmp/user-manager.tar'
done
kubectl delete pod -n user-manager -l app=user-manager        # a fresh Pod retries immediately
```
Diagnosis in two commands: *is the image in my Docker?* and *is it in containerd on the node?*

> 💡 **Lesson:** `ErrImagePull` on your own image almost always means the image never reached that node's containerd.

---

### 12. metrics-server image cannot be pulled

**Phase:** 4 · 🔴 hit while working

**Symptom**

The `metrics-server` Pod stayed in `ErrImagePull` after applying the official `components.yaml`.

**Cause**

The image lives on `registry.k8s.io`, which redirects to storage hosts that are not reachable from the restricted network.

**Fix**

Pull it directly on every node once the node had network access, or import it like any other image:

```bash
ctr -n k8s.io image pull registry.k8s.io/metrics-server/metrics-server:v0.9.0       # on each node
# or from the workstation:
./tools/load-image.sh registry.k8s.io/metrics-server/metrics-server:v0.9.0
```
The official manifest uses `imagePullPolicy: IfNotPresent`, so the pre-loaded image is used. Pin the Pod to a worker
(`nodeSelector`) if it could land on a node that lacks the image.

> 💡 **Lesson:** Images of cluster add-ons are subject to the same restrictions as your own.

---

### 13. metrics-server: x509 certificate errors

**Phase:** 4 · 🔴 hit while working

**Symptom**

```text
Failed to scrape node ... Get "https://192.168.10.6:10250/metrics/resource": tls: failed to verify certificate: x509: cannot validate certificate ...
"Failed probe" probe="metric-storage-ready" err="no metrics to serve"
Readiness probe failed: HTTP probe failed with statuscode: 500
```
and `kubectl top nodes` → `error: Metrics API not available`; HPAs showed `<unknown>`.

**Cause**

metrics-server reads CPU/memory from each kubelet over HTTPS (port 10250). The kubelet serving certificates of a kubeadm
cluster are self-signed and do not contain the node IPs, so verification fails, nothing is collected, and every symptom
above follows from that single cause.

**Fix**

```bash
kubectl patch deployment metrics-server -n kube-system --type=json -p='[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}
]'
kubectl rollout status deploy/metrics-server -n kube-system
kubectl top nodes
```
`--type=json` with `add ... /args/-` **appends** one flag. A `merge` patch would replace the whole `args` list and break the
container. The production-grade fix is `serverTLSBootstrap: true` in the kubelet configuration plus approving the CSRs.

> 💡 **Lesson:** Read the first error in the log; the rest is usually fallout.

---

### 14. kubectl and the cluster differ by four minor versions

**Phase:** 4 · 🔴 hit while working

**Symptom**

```text
WARNING: version difference between client (1.32) and server (1.36) exceeds the supported minor version skew of +/-1
```

**Cause**

The workstation's `kubectl` was much older than the API server.

**Fix**

Everything used here kept working, but it is unsupported. Install a `kubectl` within one minor version of the server.

> 💡 **Lesson:** Keep client tools within the supported skew; unsupported combinations fail in confusing ways.

---

### 15. Workers are small: memory capacity limits autoscaling

**Phase:** 4 · 🔴 hit while working

**Symptom**

`kubectl describe node worker1 worker2 | grep -A8 "Allocated resources"`:

| Node | requests | limits |
|---|---|---|
| worker1 | 818Mi (44 %) | 2432Mi (132 %) |
| worker2 | 1290Mi (70 %) | 4Gi (223 %) |

**Cause**

Each worker has only about 1.8 GiB. The scheduler counts `requests`: a 2nd `core` replica barely fits on worker2 and a 3rd
cannot. Even when it fits, a limit total above 200 % means OOM kills or evictions under real load (three JVMs plus
ActiveMQ's 1 GiB heap).

**Fix**

Either give worker2 about 4 GiB of RAM (recommended), or lower `maxReplicas` and the `requests`. A replica
that stays `Pending` with `Insufficient memory` in `kubectl describe pod` is a capacity signal, not a manifest error.

> 💡 **Lesson:** Do the arithmetic (replicas × requests per node) before turning on an autoscaler.

---

### 16. HPA and the replicas field fight

**Phase:** 4 · 🟡 caught early

**Symptom**

After the HPA scaled a Deployment, a later `kubectl apply` of the original manifest set it back to one replica.

**Cause**

The Deployment manifests contained `replicas: 1`. Applying them rewrites the replica count, undoing the autoscaler.

**Fix**

Remove `replicas` from every Deployment that has an HPA (the databases keep `replicas: 1`):

```bash
sed -i '/^  replicas: 1$/d' phase2-kubernetes/k8s/10-auth/deployment-auth.yaml \
  phase2-kubernetes/k8s/20-core/deployment-core.yaml phase3-microservice/k8s/deployment.yaml
```
The next apply resets the count once; the HPA brings it back to `minReplicas`.

> 💡 **Lesson:** Exactly one controller should own the replica count.

---

### 17. NGINX does not see Pods created by the autoscaler

**Phase:** 4 · 🟡 caught early

**Symptom**

Design problem rather than an incident: with a normal `upstream` block or `proxy_pass http://service:port`, NGINX
resolves the name **once at start-up** and keeps the addresses forever.

**Cause**

NGINX's open-source resolver behaviour: names in static configuration are resolved at load time only.

**Fix**

Three pieces together (see [Phase 4](04-phase4-autoscaling-gateway.md#step-43--the-nginx-gateway)):

1. **Headless Services** (`clusterIP: None`) so DNS returns the IP of every *ready* Pod;
2. `resolver kube-dns.kube-system.svc.cluster.local valid=5s;`
3. `set $backend "svc.ns.svc.cluster.local:port"; proxy_pass http://$backend;` (a variable forces run-time resolution).

> 💡 **Lesson:** With autoscaling, the load balancer must follow DNS/endpoints, not a snapshot.

---

### 18. Health checks pass too early (socket versus TCP)

**Phase:** 1 · 🟡 caught early

**Symptom**

Design problem caught in review: a dependent service could start while its database was still initialising.

**Cause**

During first-time initialisation MySQL and PostgreSQL run a **temporary** server that listens on the unix socket only.
`mysqladmin ping` / `pg_isready` without a host talk to that socket, report success, and `depends_on: service_healthy` releases
the application too soon.

**Fix**

Force TCP: `mysqladmin ping -h 127.0.0.1 ...` and `pg_isready -h 127.0.0.1 ...`. Also write `$$MYSQL_ROOT_PASSWORD` instead of
`${...}` so the secret is read inside the container and not substituted into the Compose model (and visible in `docker inspect`).

> 💡 **Lesson:** A health check must test the same path the clients will use.

---

### 19. Compose draft: published ports unreachable, 0.0.0.0 binding, missing variables

**Phase:** 1 · 🟡 caught early

**Symptom**

Findings from reviewing a draft `docker-compose.yml` before running it:

- the Auth HTTP port could not be reached from the host,
- ports were published on all interfaces,
- Auth had no `JWT_SECRET_KEY` and was given Core's variables,
- Core carried three `SPRING_DATASOURCE_*` variables.

**Cause**

- Every network was `internal: true`. An internal network has no gateway, and a port can only be published by a container
  attached to at least one **non-internal** network.
- `"8082:8082"` publishes on `0.0.0.0`; the LAN can reach it.
- The auth service reads `PORT`, `DB_URL` and `JWT_SECRET_KEY`; `AUTH_*` variables belong to Core.
- Core reads only `project.spring.datasource.hikari.*`, so `spring.datasource.*` had no effect.

**Fix**

Keep `auth-net` and `core-net` as ordinary networks; make only the auth↔core gRPC network `internal`. Publish with
`127.0.0.1:8082:8082`. Give Auth exactly `PORT`, `DB_URL`, `JWT_SECRET_KEY`. Remove unused variables. The result is the
[final compose file](../phase1-docker/docker-compose.yml).

> 💡 **Lesson:** Review a configuration against the code that consumes it; unused settings are a sign something is misunderstood.

---

### 20. A NodePort Service exposes every port it lists

**Phase:** 2 · 🟡 caught early

**Symptom**

Design problem: the auth Service needs HTTP 8082 *outside* the cluster and gRPC 50051 *inside only*.

**Cause**

A `NodePort` Service opens **all** of its ports on **every** node.

**Fix**

Two Services: `auth` (ClusterIP, ports 8082 and 50051) for in-cluster traffic, and `auth-http` (NodePort 30082, port 8082
only) for external HTTP.

> 💡 **Lesson:** Separate internal and external exposure into separate Services.

---

### 21. NetworkPolicy selectors: AND versus OR

**Phase:** 2 · 🟡 caught early

**Symptom**

Design problem: the gRPC rule must allow **only** the core Pods.

**Cause**

Inside one `from:` list item, `namespaceSelector` **and** `podSelector` are combined with AND. As two separate items
(two dashes) they are combined with OR, which would allow *every* Pod of the `core` namespace *or* any Pod labelled
`app=core` in any namespace the policy sees.

**Fix**

```yaml
from:
  - namespaceSelector: { matchLabels: { kubernetes.io/metadata.name: core } }
    podSelector:       { matchLabels: { app: core } }          # same item → AND
```
Verified with an isolation matrix (`nc -z`) from the auth Pod, the core Pod and a throw-away Pod in `default`.

> 💡 **Lesson:** Indentation and list dashes change the meaning of a NetworkPolicy. Always test denials, not only successes.

---

### 22. Repeated SSH password prompts

**Phase:** 2 · 🔴 hit while working

**Symptom**

Importing one image on two workers asked for the root password four times (two `scp`, two `ssh`).

**Cause**

Password authentication on every connection.

**Fix**

```bash
ssh-copy-id root@192.168.10.6
ssh-copy-id root@192.168.10.7
```

> 💡 **Lesson:** Set up key-based SSH once; it pays back on every later image import.

---

### 23. Mirrors, image names and the latest tag

**Phase:** 2 · 🟡 caught early

**Symptom**

Design problem: nodes cannot reach Docker Hub, and Kubernetes treats `:latest` specially.

**Cause**

A Pod referencing `mysql:latest` defaults to `imagePullPolicy: Always` and goes to Docker Hub every time. Mirrors use their
own host name (`docker.arvancloud.ir/library/mysql`).

**Fix**

Reference public images through the mirror and set `imagePullPolicy: IfNotPresent` explicitly. Prove the node can reach it first:

```bash
ssh root@192.168.10.6 'ctr -n k8s.io images pull docker.arvancloud.ir/library/alpine:3.20 && echo PULL-OK'
```
Official images need the `library/` segment with `ctr`; user images (e.g. `webcenter/activemq`) do not.

> 💡 **Lesson:** Test connectivity from the node itself, with the tool the node uses (`ctr`), not from your workstation.

---

### 24. Windows binary and a placeholder env file inside the repository

**Phase:** 1 · 🟡 caught early

**Symptom**

The auth repository contains `main.exe` (about 24 MB, a Windows build) and an `envs/dev.env` with a real-looking JWT secret.

**Cause**

Build artefacts and sample secrets were committed.

**Fix**

`main.exe` and `envs/` are listed in `.dockerignore`, so the 24 MB file never enters the build context. The image creates its
own `envs/dev.env` with empty values; real values come from the environment (Compose `environment:` / Kubernetes
`Secret`).

> 💡 **Lesson:** Keep secrets out of images and build contexts.

---

[← README](../README.md) · [Command reference →](COMMANDS.md)
