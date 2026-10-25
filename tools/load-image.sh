#!/usr/bin/env bash
# Make an image available on THIS machine (pull it if it is not local yet) and import it
# into containerd on every worker node, so Kubernetes never has to pull it itself.
#
# Usage:   ./load-image.sh <image-to-pull> [<name-the-manifest-expects>]
# Example: ./load-image.sh docker.arvancloud.ir/library/nginx:1.27-alpine nginx:1.27-alpine
#
# Use the 2nd argument when you pull from a mirror under a different name than the one in
# the manifest: the image is re-tagged, so the manifest finds it with imagePullPolicy IfNotPresent.
# Override the node list with:  NODES="192.168.10.6 192.168.10.7" ./load-image.sh ...
set -euo pipefail

SRC="${1:?usage: $0 <image-to-pull> [<name-the-manifest-expects>]}"
DST="${2:-$SRC}"
read -r -a NODES <<< "${NODES:-192.168.10.6 192.168.10.7}"

# images built locally (auth:phase1, ...) exist only on this machine: do not try to pull them
if ! docker image inspect "$SRC" >/dev/null 2>&1; then
  docker pull "$SRC"
fi
if [ "$SRC" != "$DST" ]; then
  docker tag "$SRC" "$DST"
fi

TAR="$(mktemp /tmp/image.XXXXXX.tar)"
trap 'rm -f "$TAR"' EXIT
docker save "$DST" -o "$TAR"
echo "saved $DST ($(du -h "$TAR" | cut -f1))"

for n in "${NODES[@]}"; do
  echo "== importing on $n"
  scp -q "$TAR" "root@$n:/tmp/image.tar"
  ssh "root@$n" 'ctr -n k8s.io images import /tmp/image.tar && rm -f /tmp/image.tar'
done

echo "== check on ${NODES[0]}"
ssh "root@${NODES[0]}" "ctr -n k8s.io images ls -q | grep -F '${DST##*/}'" || echo "WARNING: image not listed"
