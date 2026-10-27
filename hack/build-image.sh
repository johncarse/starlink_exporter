#!/usr/bin/env bash
# Builds the exporter on the in-cluster BuildKit daemon (buildkit/buildkitd) and
# pushes it to Zot, tagged fb-<commit>. Nothing goes through GitHub: the source
# streams from this checkout over a kubectl port-forward and buildkitd pushes
# straight to Zot.
#
# Needs: kubectl context for your-cluster, docker with buildx. Run from the repo root.
set -euo pipefail

REGISTRY=registry.example.com
IMAGE=starlink_exporter
BUILDER=starlink-exporter-buildkitd
PORT=${BUILDKIT_LOCAL_PORT:-11234}

if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  echo "refusing to build: uncommitted changes (the tag is derived from the commit)" >&2
  exit 1
fi
REF="${REGISTRY}/${IMAGE}:fb-$(git rev-parse --short=8 HEAD)"

# Throwaway docker config holding the Zot CI login; removed on exit.
DOCKER_CONFIG="$(mktemp -d)"
export DOCKER_CONFIG
kubectl -n buildkit port-forward svc/buildkitd "${PORT}:1234" >/dev/null 2>&1 &
PF_PID=$!
cleanup() {
  kill "$PF_PID" 2>/dev/null || true
  docker buildx rm "$BUILDER" >/dev/null 2>&1 || true
  rm -rf "$DOCKER_CONFIG"
}
trap cleanup EXIT

for _ in $(seq 1 30); do
  (exec 3<>"/dev/tcp/127.0.0.1/${PORT}") 2>/dev/null && break
  sleep 1
done

kubectl -n zot get secret registry-credentials -o jsonpath='{.data.password}' | base64 -d |
  docker login "$REGISTRY" -u ci --password-stdin >/dev/null
docker buildx create --name "$BUILDER" --driver remote "tcp://127.0.0.1:${PORT}" >/dev/null

# Zot only accepts OCI manifests (415 on Docker schema2), so force OCI media types.
docker buildx build --builder "$BUILDER" --platform linux/amd64 --provenance=false \
  -f Dockerfile.build \
  --output "type=image,name=${REF},push=true,oci-mediatypes=true" .

echo "pushed: ${REF}"
echo "digest: $(crane digest "${REF}" 2>/dev/null || echo 'run: crane digest <ref> (after docker login)')"
