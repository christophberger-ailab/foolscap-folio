#!/bin/sh
# Writes the CycloneDX SBOM of one folio binary. GoReleaser runs it once per
# build (.goreleaser.yaml, sboms), from dist/:
#
#   ../packaging/sbom/sbom.sh <binary> <document> <version>
#
# folio ships three ecosystems in one file, and syft sees each where it is
# exact:
#
# - Go: the module list the linker wrote into the binary, so only the
#   modules that are compiled in (not everything in go.sum), at the versions
#   actually used, per platform.
# - Rust: internal/svgimage/resvg/Cargo.lock, for resvg, compiled to
#   WebAssembly and embedded as resvg.wasm.gz.
# - JavaScript: internal/site/package-lock.json, for the embedded copy of
#   Fuse.js (see internal/site/package.json).
#
# syft reads one source per run, so the three are put side by side in a
# scratch directory and scanned as one.
set -eu

binary=$1
document=$2
version=$3
root=$(cd "$(dirname "$0")/../.." && pwd)

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

mkdir -p "$stage/rust" "$stage/js"
cp "$binary" "$stage/"
cp "$root/internal/svgimage/resvg/Cargo.lock" "$stage/rust/"
cp "$root/internal/site/package-lock.json" "$stage/js/"

syft scan "dir:$stage" \
  --quiet \
  --source-name folio \
  --source-version "$version" \
  --output "cyclonedx-json=$document"
