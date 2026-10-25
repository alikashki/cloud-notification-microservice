#!/usr/bin/env bash
# Applies the Phase 3 manifests. Prerequisite: the image user-manager:phase3 must exist
# on the worker nodes (docker build + docker save + ctr -n k8s.io images import).
set -euo pipefail
cd "$(dirname "$0")/k8s"

kubectl apply -f 00-namespace.yaml
kubectl apply -f config.yaml -f secret.yaml
kubectl apply -f .

kubectl get pods,svc -n user-manager -o wide
