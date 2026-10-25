#!/usr/bin/env bash
# Applies all Phase 2 manifests in a safe order.
# Prerequisite: the images auth:phase1 and core:phase1 must exist on the worker nodes
# (docker save + ctr -n k8s.io images import).
set -euo pipefail
cd "$(dirname "$0")/k8s"

kubectl apply -f 00-namespaces/
kubectl apply -f 01-storage/
# ConfigMaps and Secrets first, so Pods find them when they start
kubectl apply -f 10-auth/config.yaml -f 10-auth/secret.yaml \
              -f 20-core/config.yaml -f 20-core/secret.yaml
# everything else (PVCs, Deployments, Services, NetworkPolicies)
kubectl apply -f 10-auth/ -f 20-core/

kubectl get pods -A -o wide | grep -E 'NAMESPACE|auth|core' | grep -v kube-system
