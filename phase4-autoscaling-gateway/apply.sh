#!/usr/bin/env bash
# Applies the Phase 4 manifests.
# Prerequisites: metrics-server works ("kubectl top nodes") and the image nginx:1.27-alpine
# exists on the worker nodes (see tools/load-image.sh).
set -euo pipefail
cd "$(dirname "$0")/k8s"

kubectl apply -f headless/
kubectl apply -f gateway/00-namespace.yaml
kubectl apply -f gateway/
kubectl apply -f hpa/

kubectl get hpa -A
kubectl get pods,svc -n gateway -o wide
