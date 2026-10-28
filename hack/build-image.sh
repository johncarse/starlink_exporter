#!/usr/bin/env bash
# Builds the exporter on a BuildKit daemon running in a Kubernetes cluster and
# pushes it to your registry, tagged fb-<commit>. Nothing goes through a CI
# service: the source streams from this checkout over a kubectl port-forward and
# buildkitd pushes straight to the registry.
#
# Needs: a kubectl context for the cluster, docker with buildx, and crane
# (optional, prints the digest). Run from the repo root with:
#   REGISTRY            registry host to push to, e.g. registry.example.com
#   CREDENTIALS_SECRET  <namespace>/<name> of a Secret whose "password" key is
#                       the registry login
# and optionally:
#   REGISTRY_USER       registry user (default ci)
#   BUILDKIT_TARGET     <namespace>/<service> of buildkitd (default buildkit/buildkitd)
#   BUILDKIT_PORT       buildkitd service port (default 1234)
#   BUILDKIT_LOCAL_PORT local port for the port-forward (default 11234)
set -euo pipefail

REGISTRY=${REGISTRY:?set REGISTRY to the registry host to push to}
CREDENTIALS_SECRET=${CREDENTIALS_SECRET:?set CREDENTIALS_SECRET to <namespace>/<name> of the registry login Secret}
REGISTRY_USER=${REGISTRY_USER:-ci}
BUILDKIT_TARGET=${BUILDKIT_TARGET:-buildkit/buildkitd}
BUILDKIT_PORT=${BUILDKIT_PORT:-1234}
PORT=${BUILDKIT_LOCAL_PORT:-11234}
IMAGE=starlink_exporter
BUILDER=starlink-exporter-buildkitd

if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  echo "refusing to build: uncommitted changes (the tag is derived from the commit)" >&2
  exit 1
fi
REF="${REGISTRY}/${IMAGE}:fb-$(git rev-parse --short=8 HEAD)"

# Throwaway docker config holding the registry login; removed on exit.
DOCKER_CONFIG="$(mktemp -d)"
export DOCKER_CONFIG
kubectl -n "${BUILDKIT_TARGET%%/*}" port-forward "svc/${BUILDKIT_TARGET##*/}" "${PORT}:${BUILDKIT_PORT}" >/dev/null 2>&1 &
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

kubectl -n "${CREDENTIALS_SECRET%%/*}" get secret "${CREDENTIALS_SECRET##*/}" -o jsonpath='{.data.password}' | base64 -d |
  docker login "$REGISTRY" -u "$REGISTRY_USER" --password-stdin >/dev/null
docker buildx create --name "$BUILDER" --driver remote "tcp://127.0.0.1:${PORT}" >/dev/null

# Some registries (Zot, for one) only accept OCI manifests and answer 415 to
# Docker schema2, so force OCI media types.
docker buildx build --builder "$BUILDER" --platform linux/amd64 --provenance=false \
  -f Dockerfile.build \
  --output "type=image,name=${REF},push=true,oci-mediatypes=true" .

echo "pushed: ${REF}"
echo "digest: $(crane digest "${REF}" 2>/dev/null || echo 'run: crane digest <ref> (after docker login)')"
