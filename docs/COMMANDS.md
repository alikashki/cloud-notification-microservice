# Command reference

> Every command used while building the project, grouped by phase and in the order they were run. Placeholders:
> `<node>` = a node IP, `$TOKEN` = the JWT from the login call.

[← Troubleshooting](TROUBLESHOOTING.md) · [README](../README.md)

**Contents:** [Step 0](#step-0--preparation) · [Phase 1](#phase-1--docker) · [Phase 2](#phase-2--kubernetes) · [Phase 3](#phase-3--microservice) · [Phase 4](#phase-4--autoscaling-gateway-ha) · [Handy extras](#handy-extras)

---

## Step 0 · Preparation

```bash
# tools installed?
docker --version && docker compose version
kubectl version --client
git --version

# unpack the course repositories (leave out the 24 MB Windows binary)
tar -tf auth.tar | head -50
tar -xf auth.tar --exclude='./auth/main.exe'
tar -xf core.tar

# project folder
mkdir -p ~/cloud_project/{phase1,phase2/k8s,phase3,phase4}
```

`--exclude='./auth/main.exe'`: the archive stores paths with a leading `./`, so the pattern must match exactly; the file is
a Windows executable that is useless in a Linux image.

## Phase 1 · Docker

### Build the images

```bash
cd ~/cloud_project/phase1
docker build -t auth:phase1 ./auth
docker build -t core:phase1 ./core
docker images | grep phase1

# use another Go module mirror if the default list fails
docker build --build-arg GOPROXY='https://mirror-go.runflare.com|direct' -t auth:phase1 ./auth
```

Prove that environment variables override the placeholder `dev.env` (the app prints the DSN, then fails to connect):

```bash
docker run --rm -e DB_URL='user:pass@tcp(nohost:3306)/testdb' -e JWT_SECRET_KEY=x auth:phase1
```

Inspect the layers when an image is bigger than expected:

```bash
docker history core:phase1
```

### Compose: infrastructure first (step 1.3)

```bash
docker compose config > /dev/null && echo OK        # validate + interpolate .env
docker compose up -d
docker compose ps
```

Persistence test with a scratch table:

```bash
docker compose exec auth-db sh -c 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE" -e "CREATE TABLE t(id INT); INSERT INTO t VALUES (1); SELECT * FROM t;"'
docker compose exec core-db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "CREATE TABLE t(id int); INSERT INTO t VALUES (1); SELECT * FROM t;"'

docker compose down                                  # NOT "down -v" (that deletes the volumes)
docker compose up -d

docker compose exec auth-db sh -c 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE" -e "SELECT * FROM t;"'
docker compose exec core-db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT * FROM t;"'

docker compose exec auth-db sh -c 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE" -e "DROP TABLE t;"'
docker compose exec core-db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "DROP TABLE t;"'
```

### Compose: the full stack (steps 1.4-1.5)

```bash
docker compose down
docker compose up -d --build
docker compose ps
docker compose logs core | tail -20
docker compose logs auth core
docker compose ps --format 'table {{.Name}}\t{{.Ports}}'      # which ports are published?
```

### API checks

```bash
curl -i -X POST localhost:8082/user/register -H 'Content-Type: application/json' -d '{"email":"a@b.com","password":"Pass1234"}'
curl -s -X POST localhost:8082/user/login    -H 'Content-Type: application/json' -d '{"email":"a@b.com","password":"Pass1234"}'

TOKEN=$(curl -s -X POST localhost:8082/user/login -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","password":"Pass1234"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')

curl -s -X POST localhost:8082/user/validate -H "Authorization: Bearer $TOKEN"     # auth wants "Bearer"
curl -i localhost:8080/mail/configs -H "Authorization: $TOKEN"                     # core wants the RAW token → 200
curl -i localhost:8080/mail/configs -H "Authorization: Bearer $TOKEN"              # 401 (wrong format)
curl -sI localhost:8080/docs | head -1                                             # 302 → Swagger UI
curl -i localhost:8080/actuator/health                                             # 500: needs a token
```

### Network isolation matrix

```bash
T='for t in "$@"; do h=${t%:*}; p=${t#*:}; nc -z -w 3 $h $p && echo "OK   $t" || echo "FAIL $t"; done'
docker compose exec auth sh -c "$T" _ auth-db:3306 core:8080 core-db:5432 activemq:61616
docker compose exec core sh -c "$T" _ core-db:5432 activemq:61616 auth:50051 auth:8082 auth-db:3306

# real ICMP ping needs root inside the container
docker compose exec -u root auth ping -c 1 -W 2 auth-db
docker compose exec -u root core ping -c 1 -W 2 core-db
```

## Phase 2 · Kubernetes

### Inspect the cluster

```bash
kubectl get nodes -o wide --show-labels
kubectl version
kubectl get storageclass
kubectl get pods -A
kubectl get ns
```

### Free the ports used by the Compose stack

```bash
cd ~/cloud_project/phase1 && docker compose down        # without -v
```

### Namespaces and storage

```bash
cd ~/cloud_project/phase2
kubectl apply -f k8s/00-namespaces/ -f k8s/01-storage/
kubectl get ns auth core
kubectl get storageclass
kubectl get pv
```

### Can the nodes pull images?

```bash
ssh root@192.168.10.6 'ctr -n k8s.io images pull docker.arvancloud.ir/library/alpine:3.20 && echo PULL-OK'
ssh root@192.168.10.7 'ctr -n k8s.io images pull docker.arvancloud.ir/library/alpine:3.20 && echo PULL-OK'
ssh root@192.168.10.6 'ctr -n k8s.io images rm docker.arvancloud.ir/library/alpine:3.20'      # clean up
```

### Moving local images to the nodes

```bash
docker images | grep phase1
docker save auth:phase1 core:phase1 -o fum-images.tar
ls -lh fum-images.tar

for n in 192.168.10.6 192.168.10.7; do
  echo "== $n"
  scp fum-images.tar root@$n:/tmp/
  ssh root@$n 'ctr -n k8s.io images import /tmp/fum-images.tar && rm /tmp/fum-images.tar'
done

for n in 192.168.10.6 192.168.10.7; do
  ssh root@$n 'ctr -n k8s.io images ls -q | grep -E "auth:phase1|core:phase1"'
done

# if import complains about a missing content digest
docker save --platform linux/amd64 auth:phase1 core:phase1 -o fum-images.tar

# passwordless SSH
ssh-copy-id root@192.168.10.6 ; ssh-copy-id root@192.168.10.7
```

### ConfigMaps and Secrets

```bash
kubectl apply -f k8s/10-auth/ -f k8s/20-core/
kubectl get configmap,secret -n auth
kubectl get configmap,secret -n core
kubectl get secret auth-secret -n auth -o jsonpath='{.data.JWT_SECRET_KEY}'; echo
kubectl get secret auth-secret -n auth -o jsonpath='{.data.JWT_SECRET_KEY}' | base64 -d; echo
```

### Workloads

```bash
kubectl apply --dry-run=client -f k8s/20-core/deployment-activemq.yaml      # validate before applying
grep -n -A8 "resources:" k8s/20-core/deployment-activemq.yaml | cat -A      # show hidden characters

./apply.sh                                                                  # ordered apply of everything

kubectl get pod,svc,pvc -n auth -o wide
kubectl get pod,svc,pvc -n core -o wide
kubectl get pv
kubectl get pods -n core -w                                                 # watch
kubectl logs -n auth deploy/auth
kubectl logs -n core deploy/core | tail -30
kubectl describe pod -n <ns> -l app=<name> | tail -25
```

### Functional tests through the NodePorts

```bash
curl -i -X POST http://192.168.10.6:30082/user/register -H 'Content-Type: application/json' -d '{"email":"a@b.com","password":"Pass1234"}'

TOKEN=$(curl -s -X POST http://192.168.10.6:30082/user/login -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","password":"Pass1234"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')
curl -i http://192.168.10.6:30080/mail/configs -H "Authorization: $TOKEN"

# both nodes: a NodePort exists on every node
for ip in 192.168.10.6 192.168.10.7; do
  TOKEN=$(curl -s -X POST http://$ip:30082/user/login -H 'Content-Type: application/json' \
    -d '{"email":"a@b.com","password":"Pass1234"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')
  echo "$ip -> core status:" $(curl -s -o /dev/null -w '%{http_code}' http://$ip:30080/mail/configs -H "Authorization: $TOKEN")
done
```

### NetworkPolicies and the isolation matrix

```bash
kubectl apply -f k8s/10-auth/ -f k8s/20-core/
kubectl get networkpolicy -A
kubectl get pods -A | grep -E 'auth|core'

T='for t in "$@"; do h=${t%:*}; p=${t#*:}; nc -z -w 3 $h $p && echo "OK   $t" || echo "FAIL $t"; done'
kubectl exec -n auth deploy/auth -- sh -c "$T" _ auth-db:3306 core-db.core:5432 activemq.core:61616 core.core:8080
kubectl exec -n core deploy/core -- sh -c "$T" _ core-db:5432 activemq:61616 auth.auth:50051 auth.auth:8082 auth-db.auth:3306

# an "outsider" Pod in the unrestricted default namespace
kubectl run outsider --image=auth:phase1 --restart=Never --command -- sleep 600
kubectl wait --for=condition=Ready pod/outsider --timeout=60s
kubectl exec outsider -- sh -c "$T" _ auth.auth:8082 auth.auth:50051 auth-db.auth:3306 core.core:8080 core-db.core:5432 activemq.core:61616
kubectl delete pod outsider

# if nothing is blocked, is the CNI enforcing policy at all?
kubectl -n kube-system get cm cilium-config -o yaml | grep -iE "enable-policy|policy-enforcement"
```

### Persistence

```bash
kubectl delete pod -n auth -l app=auth-db
kubectl delete pod -n core -l app=core-db
kubectl delete pod -n core -l app=activemq
kubectl get pods -A -o wide | grep -E 'auth|core' | grep -v kube-system
curl -s -X POST http://192.168.10.6:30082/user/login -H 'Content-Type: application/json' -d '{"email":"a@b.com","password":"Pass1234"}'
ssh root@192.168.10.6 'ls /data/fum/auth-db | head'
```

### Everything for the demo video

```bash
kubectl get nodes -o wide
kubectl get all,configmap,secret,pvc,networkpolicy -n auth -o wide
kubectl get all,configmap,secret,pvc,networkpolicy -n core -o wide
kubectl get storageclass,pv
kubectl get pods -A -o custom-columns='NS:.metadata.namespace,NAME:.metadata.name,NODE:.spec.nodeName,CPU-LIM:.spec.containers[0].resources.limits.cpu,MEM-LIM:.spec.containers[0].resources.limits.memory' | grep -E 'NS|auth|core' | grep -v kube-system
kubectl describe deploy auth -n auth | head -60
```

## Phase 3 · Microservice

```bash
# build and ship the image
cd phase3-microservice/user-manager
ls -la                                                 # .dockerignore Dockerfile go.mod go.sum main.go
docker build -t user-manager:phase3 .
docker images | grep user-manager
docker save user-manager:phase3 -o user-manager.tar
for n in 192.168.10.6 192.168.10.7; do
  scp user-manager.tar root@$n:/tmp/
  ssh root@$n 'ctr -n k8s.io images import /tmp/user-manager.tar && rm /tmp/user-manager.tar'
done
ssh root@192.168.10.6 'ctr -n k8s.io images ls -q | grep user-manager'

# diagnosing ErrImagePull
kubectl describe pod -n user-manager -l app=user-manager | tail -20
docker images | grep user-manager
ssh root@192.168.10.6 'ctr -n k8s.io images ls | grep -i user-manager'

# deploy
cd ..; ./apply.sh
kubectl delete pod -n user-manager -l app=user-manager
kubectl get pods,svc -n user-manager -o wide -w
kubectl get networkpolicy -n user-manager
kubectl get networkpolicy -n auth
kubectl logs -n user-manager deploy/user-manager
```

API tests (full list with expected results in [Phase 3](03-phase3-microservice.md#step-33--verification)):

```bash
B=http://192.168.10.6:30090
curl -s $B/healthz
curl -s "$B/auth/users?page=1&page_size=5"
curl -s -X DELETE "$B/auth/user?id=2"
curl -s -X DELETE "$B/auth/user?email=user3@test.com"
```

Isolation:

```bash
kubectl exec -n user-manager deploy/user-manager -- sh -c "$T" _ \
  auth-db.auth:3306 auth.auth:8082 auth.auth:50051 core.core:8080 core-db.core:5432 activemq.core:61616
```

## Phase 4 · Autoscaling, gateway, HA

### Capacity

```bash
kubectl top nodes
kubectl describe node worker1 worker2 | grep -A8 "Allocated resources"
```

### metrics-server

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

kubectl get pods -n kube-system -l k8s-app=metrics-server
kubectl describe pod -n kube-system -l k8s-app=metrics-server | grep -iE "Image:|Failed|Error" | head
kubectl get deploy metrics-server -n kube-system -o jsonpath='{.spec.template.spec.containers[0].image}'; echo
kubectl get deploy metrics-server -n kube-system -o jsonpath='{.spec.template.spec.containers[0].args}'; echo

# image pulled on each node
ctr -n k8s.io image pull registry.k8s.io/metrics-server/metrics-server:v0.9.0
./tools/load-image.sh registry.k8s.io/metrics-server/metrics-server:v0.9.0       # alternative

# accept the kubelets' self-signed certificates
kubectl patch deployment metrics-server -n kube-system --type=json -p='[
  {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}
]'
kubectl rollout restart deploy/metrics-server -n kube-system
kubectl rollout status deploy/metrics-server -n kube-system
kubectl logs -n kube-system deploy/metrics-server --tail=5
kubectl top nodes
```

### HPAs

```bash
sed -i '/^  replicas: 1$/d' phase2-kubernetes/k8s/10-auth/deployment-auth.yaml \
  phase2-kubernetes/k8s/20-core/deployment-core.yaml phase3-microservice/k8s/deployment.yaml
./phase2-kubernetes/apply.sh && ./phase3-microservice/apply.sh
kubectl apply -f phase4-autoscaling-gateway/k8s/hpa/
kubectl get hpa -A
kubectl describe hpa auth -n auth
```

### NGINX gateway

```bash
./tools/load-image.sh docker.arvancloud.ir/library/nginx:1.27-alpine nginx:1.27-alpine
./phase4-autoscaling-gateway/apply.sh
kubectl get pods,svc -n gateway -o wide
kubectl get endpoints -n auth auth-headless
kubectl logs -n gateway deploy/nginx --tail=5

B=http://192.168.10.6:30100
curl -s $B/nginx-health
for i in $(seq 1 12); do
  curl -s -o /dev/null -D - -X POST $B/user/login -H 'Content-Type: application/json' \
    -d '{"email":"a@b.com","password":"Pass1234"}' | grep -i x-served-by
done | sort | uniq -c
```

### Load test

```bash
watch -n 3 'kubectl get hpa -A; echo; kubectl get pods -n auth -o wide; echo; kubectl top pods -n auth'

kubectl apply -f phase4-autoscaling-gateway/loadtest/loadgen-auth.yaml
kubectl delete -f phase4-autoscaling-gateway/loadtest/loadgen-auth.yaml

# optional: load on core (needs a token)
kubectl create deployment loadgen-core --image=auth:phase1 --replicas=8 -- sh -c \
  "while true; do wget -q -O /dev/null --header 'Authorization: $TOKEN' http://nginx.gateway.svc.cluster.local/mail/configs || sleep 1; done"
kubectl delete deployment loadgen-core
```

### HAProxy / control plane (run on the load-balancer machines)

```bash
cat /etc/haproxy/haproxy.cfg
systemctl status haproxy --no-pager | head -8
cat /etc/keepalived/keepalived.conf 2>/dev/null
systemctl status keepalived --no-pager 2>/dev/null | head -8
ip -4 addr show | grep inet
```

## Handy extras

```bash
# which container runtime / node image store is used?
ctr -n k8s.io images ls -q

# list everything in a namespace, including config and policies
kubectl get all,cm,secret,pvc,networkpolicy -n <ns>

# one Deployment's effective environment
kubectl exec -n core deploy/core -- env | sort

# clean up everything of one phase (careful: PVs are Retain, data stays on the node)
kubectl delete -f phase3-microservice/k8s/
```

[← Troubleshooting](TROUBLESHOOTING.md) · [README](../README.md)
