# 03 · Phase 3 · Your own microservice (`user-manager`)

> A small Go REST service that lists and deletes the users of the auth database, deployed in its own namespace and
> allowed to talk to **nothing but** that database.

[← Phase 2](02-phase2-kubernetes.md) · [README](../README.md) · next: [Phase 4 · Autoscaling & gateway →](04-phase4-autoscaling-gateway.md)

**Files:** [`phase3-microservice/`](../phase3-microservice) · source: [`user-manager/main.go`](../phase3-microservice/user-manager/main.go)

---

## What the assignment asks for

| Requirement | Implemented as |
|---|---|
| RESTful API, any language | Go, standard library `net/http` |
| `GET /auth/users`: **paginated** list of registered users | `?page=` and `?page_size=` (default 10, max 100) |
| `DELETE /auth/user`: delete by **email or user ID**, handle exceptions | `?id=` **or** `?email=` (exactly one) with explicit error codes |
| JSON responses | every response, including errors |
| Access to the auth DB **in a separate network**, and **no other service** | own namespace + NetworkPolicy (egress only to `auth-db:3306`) |
| Deployed with the other services; API reachable from outside | Deployment + NodePort `30090` (and through the gateway, see Phase 4) |

## Why Go

- One static binary and a ~20 MB image.
- **One** dependency: `github.com/go-sql-driver/mysql v1.7.0`, the very version the auth service already uses, so its
  `go.sum` lines are known-good.
- The Go module mirrors were already proven to work on the restricted network (see
  [Troubleshooting #2](TROUBLESHOOTING.md#2-go-mod-download-fails-with-403-forbidden)); I could not verify a PyPI mirror.

## API contract

### `GET /auth/users`

| Query | Default | Rules |
|---|---|---|
| `page` | `1` | integer ≥ 1 |
| `page_size` | `10` | integer 1-100 |

```bash
curl -s "http://<node>:30090/auth/users?page=2&page_size=5"
```
```json
{
  "page": 2,
  "page_size": 5,
  "total": 12,
  "total_pages": 3,
  "users": [ { "id": 6, "email": "user5@test.com" }, { "id": 7, "email": "user6@test.com" } ]
}
```

- Ordered by `id`. A page past the end returns an empty `users` array (not an error).
- The password hash is **never** selected, only `id` and `email`.

### `DELETE /auth/user`

```bash
curl -X DELETE "http://<node>:30090/auth/user?id=3"
curl -X DELETE "http://<node>:30090/auth/user?email=user3@test.com"
```
```json
{ "message": "user deleted", "deleted_count": 1 }
```

### `GET /healthz`

Pings the database. `200 {"status":"ok","database":"up"}` or `503 {"status":"degraded","database":"down"}`.

### Error handling ("handle exceptions")

| Situation | Status | Body |
|---|---|---|
| `page` / `page_size` not an integer or out of range | `400` | `{"error":"...","status":400}` |
| `DELETE` with both `id` and `email`, or with neither | `400` | exactly one query parameter is required |
| `id` not a positive integer | `400` | |
| user does not exist | `404` | `user not found` |
| wrong HTTP method (e.g. `POST /auth/users`) | `405` | with an `Allow` header |
| unknown path | `404` | `route not found` (JSON, not the default text page) |
| database error | `500` | generic `database error`; details only in the log |
| unexpected panic | `500` | recovered by middleware, JSON body |

## Code walkthrough

| Piece | Why |
|---|---|
| `mysql.NewConfig()` + `FormatDSN()` | builds the DSN from separate variables (`DB_HOST`, `DB_USER`, ...) so credentials never need manual escaping |
| `sql.Open` without a ping at start-up | `sql.Open` does **not** connect. The service starts even while MySQL is still coming up; `/healthz` reports the real state and the readiness probe keeps traffic away meanwhile |
| Parameterised queries (`?` placeholders) | no SQL injection through `email` or `id` |
| `context.WithTimeout(..., 5s)` on every query | a hung database cannot hang a request forever |
| `sql.NullString` for `email` | a `NULL` column does not break the scan |
| `rows.Err()` checked after the loop | catches errors that happen mid-iteration |
| `RowsAffected()` | the only reliable way to tell "deleted" from "no such user" |
| Recover + logging middleware | a bug becomes a JSON `500` plus a log line instead of a dropped connection |
| `signal.Notify` + `srv.Shutdown` | graceful shutdown on `SIGTERM`, which is what Kubernetes sends |

The service works on the table the auth service created: `users(id, email, password)`.

## Step 3.1 · Build and ship the image

```bash
cd phase3-microservice/user-manager
docker build -t user-manager:phase3 .
docker images | grep user-manager                     # ≈ 20-25 MB
docker save user-manager:phase3 -o user-manager.tar
for n in 192.168.10.6 192.168.10.7; do
  scp user-manager.tar root@$n:/tmp/
  ssh root@$n 'ctr -n k8s.io images import /tmp/user-manager.tar && rm /tmp/user-manager.tar'
done
```

If the build stops at `go mod download`, pass another mirror:
`docker build --build-arg GOPROXY='https://mirror-go.runflare.com|direct' -t user-manager:phase3 .`

> [!WARNING]
> Forgetting this step gives `ErrImagePull` / `ImagePullBackOff`: [Troubleshooting #11](TROUBLESHOOTING.md#11-user-manager-pod-stuck-in-errimagepull).

## Step 3.2 · Kubernetes objects

| File | Object | Notes |
|---|---|---|
| `00-namespace.yaml` | Namespace `user-manager` | its own namespace = "separate network" |
| `config.yaml` | ConfigMap | `DB_HOST=auth-db.auth.svc.cluster.local`, `DB_PORT`, `DB_NAME`, `HTTP_PORT=8090` |
| `secret.yaml` | Secret | `DB_USER`, `DB_PASSWORD`; **copy** of the auth DB credentials (a Secret cannot be shared across namespaces) |
| `deployment.yaml` | Deployment | pinned to `worker1`, 25m/32Mi → 200m/128Mi; readiness `GET /healthz`, liveness TCP 8090 |
| `service.yaml` | Service | NodePort `30090` |
| `np-default-deny.yaml`, `np-allow-dns.yaml` | NetworkPolicy | deny all, allow DNS |
| `np-user-manager.yaml` | NetworkPolicy | ingress 8090 from anywhere; **egress only to `auth-db` (namespace `auth`) port 3306** |
| `np-auth-db-from-user-manager.yaml` | NetworkPolicy (in namespace `auth`) | admits `user-manager` Pods to the database |

**Why two probes behave differently:** readiness uses `/healthz`, so the Pod receives no traffic while the database is
unreachable. Liveness only checks the TCP port: a database outage must not make Kubernetes restart a healthy process.

**Why the extra policy lives in the `auth` namespace:** policies are additive, so one more `NetworkPolicy` that selects
`auth-db` simply adds a permitted client. The Phase 2 files stay untouched.

```bash
cd phase3-microservice && ./apply.sh
kubectl get pods,svc -n user-manager -o wide          # 1/1 Running on worker1
kubectl get networkpolicy -n user-manager
kubectl get networkpolicy -n auth                     # now includes auth-db-allow-user-manager
```

## Step 3.3 · Verification

Create some users, then exercise pagination and deletion.

```bash
for i in $(seq 1 12); do
  curl -s -o /dev/null -X POST http://192.168.10.6:30082/user/register -H 'Content-Type: application/json' \
       -d "{\"email\":\"user$i@test.com\",\"password\":\"Pass1234\"}"
done
B=http://192.168.10.6:30090

curl -s $B/healthz; echo                                   # {"status":"ok","database":"up"}
curl -s "$B/auth/users?page=1&page_size=5"; echo           # 5 users, total, total_pages
curl -s "$B/auth/users?page=99"; echo                      # empty list, not an error
curl -s -i "$B/auth/users?page=abc" | head -1              # 400
curl -s -i -X POST "$B/auth/users"  | head -1              # 405

curl -s -X DELETE "$B/auth/user?id=2"; echo                         # delete by id
curl -s -X DELETE "$B/auth/user?email=user3@test.com"; echo         # delete by email
curl -s -i -X DELETE "$B/auth/user?id=2" | head -1                  # 404 (already gone)
curl -s -i -X DELETE "$B/auth/user?id=2&email=x@y.com" | head -1    # 400 (both)
curl -s -i -X DELETE "$B/auth/user" | head -1                       # 400 (neither)
curl -s -i -X DELETE "$B/auth/user?id=-5" | head -1                 # 400 (invalid id)
```

A deleted user must really be gone from the **auth** database: logging in as `user3@test.com` must now fail.

**Isolation ("ping all DBs and other microservices"):**

```bash
T='for t in "$@"; do h=${t%:*}; p=${t#*:}; nc -z -w 3 $h $p && echo "OK   $t" || echo "FAIL $t"; done'
kubectl exec -n user-manager deploy/user-manager -- sh -c "$T" _ \
  auth-db.auth:3306 auth.auth:8082 auth.auth:50051 core.core:8080 core-db.core:5432 activemq.core:61616
```

Expected: **only** `auth-db.auth:3306` is `OK`.

## Limitations

> [!WARNING]
> The API has **no authentication** (the assignment did not ask for it). Anyone who can reach port 30090 (or the
> gateway) can delete users. A shared admin token kept in a Secret would be the first improvement.

- It uses the same database account as the auth service. A dedicated MySQL user with only `SELECT, DELETE` on
  `users` would follow least privilege.
- Emails are not unique in the auth schema, so deleting by email can remove several rows; `deleted_count` reports it.

[← Phase 2](02-phase2-kubernetes.md) · [README](../README.md) · next: [Phase 4 · Autoscaling & gateway →](04-phase4-autoscaling-gateway.md)
