#!/usr/bin/env bash
# Regenerate pkg/ from the protobuf descriptors the dish serves over gRPC reflection.
# Needs: grpcurl, protoc, protoc-gen-go v1.27.1 (matches google.golang.org/protobuf in go.mod),
# and network access to the dish.  Usage: hack/regen-protos.sh [dish-address]
set -euo pipefail

DISH=${1:-192.168.100.1:9200}
MOD=github.com/danopstech/starlink_exporter/pkg
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

grpcurl -plaintext -protoset-out "$WORK/dish.protoset" "$DISH" describe SpaceX.API.Device.Device >/dev/null

# Files the dish ships, minus google/protobuf (provided by protoc).
FILES=$(protoc --decode=google.protobuf.FileDescriptorSet google/protobuf/descriptor.proto <"$WORK/dish.protoset" \
  | sed -n 's/^  name: "\(spacex_api\/[^"]*\)"/\1/p')

# go_package comes from the descriptors; three files ship without one.
OPTS=(
  "--go_opt=Mspacex_api/satellites/network/ut_disablement_codes.proto=$MOD/spacex.com/api/satellites/network"
  "--go_opt=Mspacex_api/telemetron/public/common/time.proto=$MOD/spacex.com/api/telemetron/public/common"
  "--go_opt=Mspacex_api/device/services/unlock/service.proto=$MOD/spacex.com/api/device/services/unlock"
)
for f in $FILES; do
  case $f in spacex_api/satellites/*|spacex_api/telemetron/public/common/*|*/unlock/*) continue;; esac
  g=$(protoc --decode=google.protobuf.FileDescriptorSet google/protobuf/descriptor.proto <"$WORK/dish.protoset" \
    | awk -v n="$f" '$0 ~ "name: \""n"\""{f=1} f&&/go_package/{gsub(/.*go_package: "|"/,"");print;exit}')
  OPTS+=("--go_opt=M$f=$MOD/$g")
done

mkdir "$WORK/gen"
# shellcheck disable=SC2086
protoc --descriptor_set_in="$WORK/dish.protoset" --go_out="$WORK/gen" "${OPTS[@]}" $FILES

# device_grpc.pb.go (service stubs) is kept as-is; only message files are replaced.
find "$ROOT/pkg" -name '*.pb.go' ! -name '*_grpc.pb.go' -delete
cp -r "$WORK/gen/$MOD/." "$ROOT/pkg/"
