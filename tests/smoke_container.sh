#!/usr/bin/env bash
# End-to-End Container Smoke Test
# Validates that the built multi-arch builder image can build, push, and release flatpaks.
set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 [BUILDER_IMAGE]"
  exit 1
fi

TEST_IMAGE="$1"
REGISTRY_NAME="smoke-registry"
REGISTRY_PORT="5000"

CONTAINER_TOOL=${CONTAINER_TOOL:-$(command -v podman 2>/dev/null || command -v docker 2>/dev/null)}
if [ -z "$CONTAINER_TOOL" ]; then
  echo "Error: Neither podman nor docker found in PATH"
  exit 1
fi

echo "=== Using container tool: $CONTAINER_TOOL ==="

echo "=== Starting OCI registry container on port ${REGISTRY_PORT} ==="
# Cleanup any previous stale registry
"$CONTAINER_TOOL" stop "$REGISTRY_NAME" || true
"$CONTAINER_TOOL" rm "$REGISTRY_NAME" || true

"$CONTAINER_TOOL" run -d --name "$REGISTRY_NAME" --net=host docker.io/library/registry:2

cleanup() {
  echo "=== Tearing down smoke test registry ==="
  "$CONTAINER_TOOL" stop "$REGISTRY_NAME" || true
  "$CONTAINER_TOOL" rm "$REGISTRY_NAME" || true
}
trap cleanup EXIT

check_platform_supported() {
  local img="$1"
  local plat="$2"
  local target_arch="${plat#linux/}"

  # Check if manifest list is supported and exists
  if "$CONTAINER_TOOL" manifest inspect "$img" >/dev/null 2>&1; then
    if "$CONTAINER_TOOL" manifest inspect "$img" | jq -e ".manifests[].platform.architecture | select(. == \"$target_arch\")" >/dev/null 2>&1; then
      return 0
    fi
  else
    # Fallback to single image inspect
    local img_arch
    img_arch=$("$CONTAINER_TOOL" image inspect "$img" | jq -r '.[0].Architecture')
    if [ "$img_arch" = "$target_arch" ]; then
      return 0
    fi
  fi
  return 1
}

echo "=== Waiting for registry to accept connections ==="
for i in {1..15}; do
  if curl -s http://localhost:${REGISTRY_PORT}/v2/ >/dev/null; then
    echo "Registry is ready!"
    break
  fi
  sleep 1
done

# Create temporary workspace on the host
SMOKE_DIR=$(mktemp -d -t aetherpak-smoke-XXXXXX)
chmod 777 "$SMOKE_DIR"

for platform in linux/amd64 linux/arm64; do
  if ! check_platform_supported "$TEST_IMAGE" "$platform"; then
    echo "=== Skipping platform $platform: not supported by the local image/manifest ==="
    continue
  fi

  echo "=========================================================="
  echo "=== Running Smoke Test for Platform: $platform ==="
  echo "=========================================================="

  if [ "$platform" = "linux/amd64" ]; then
    flatpak_arch="x86_64"
  else
    flatpak_arch="aarch64"
  fi

  arch_dir="${SMOKE_DIR}/${flatpak_arch}"
  mkdir -p "$arch_dir"
  chmod 777 "$arch_dir"

  # Run the build/push/site release flow inside the container
  "$CONTAINER_TOOL" run --rm --privileged --net=host --platform "$platform" \
    -v "${arch_dir}:/workspace" -w /workspace "$TEST_IMAGE" \
    /bin/bash -c "
      set -euo pipefail

      echo '=== Generating Mock Flatpak Manifest ==='
      cat > app.json <<EOF
{
  \"id\": \"org.aetherpak.Smoke\",
  \"runtime\": \"org.freedesktop.Platform\",
  \"runtime-version\": \"24.08\",
  \"sdk\": \"org.freedesktop.Sdk\",
  \"command\": \"smoke\",
  \"modules\": [
    {
      \"name\": \"smoke\",
      \"buildsystem\": \"simple\",
      \"build-commands\": [\"mkdir -p /app/bin\", \"touch /app/bin/smoke\"]
    }
  ]
}
EOF

      echo '=== Generating Ephemeral GPG Key Pair ==='
      cat > gpg-params <<EOF
%no-protection
Key-Type: RSA
Key-Length: 2048
Name-Real: AetherPak CI
Name-Email: ci@aetherpak.local
Expire-Date: 0
%commit
EOF
      gpg --batch --gen-key gpg-params
      gpg --armor --export-secret-keys > key.priv.asc

      echo '=== Building Application ==='
      aetherpak build --manifest app.json --arch ${flatpak_arch} --branch stable --repo-path repo \
        --builder-arg --install-deps-from=flathub --builder-arg --disable-rofiles-fuse --plain

      echo '=== Pushing to Local Registry ==='
      aetherpak push-oci --app org.aetherpak.Smoke --branch stable --registry localhost:${REGISTRY_PORT} \
        --oci-repository aetherpak/smoke --repo-path repo --records-dir records --gpg-key key.priv.asc --insecure

      echo '=== Building Site ==='
      aetherpak build-site --pages-url http://localhost:9999 --records-dir records --site-dir site \
        --gpg-key key.priv.asc --allow-unsigned
    "

  echo "=== Verifying outputs for $platform ==="
  if [ ! -f "${arch_dir}/site/index/static" ]; then
    echo "Error: index/static not found for $platform"
    exit 1
  fi
  if [ ! -f "${arch_dir}/site/refs/org.aetherpak.Smoke-stable.flatpakref" ]; then
    echo "Error: .flatpakref not found for $platform"
    exit 1
  fi

  # Verify app ref in index matches flatpak architecture
  app_ref=$(jq -r '.Results[0].Images[0].Labels["org.flatpak.ref"]' "${arch_dir}/site/index/static")
  expected_ref="app/org.aetherpak.Smoke/${flatpak_arch}/stable"
  if [ "$app_ref" != "$expected_ref" ]; then
    echo "Error: invalid app ref in index: got '$app_ref', expected '$expected_ref'"
    exit 1
  fi

  echo "=== Smoke test for $platform passed successfully! ==="
done

echo "=========================================================="
echo "=== ALL SMOKE TESTS COMPLETED SUCCESSFULLY ==="
echo "=========================================================="
rm -rf "$SMOKE_DIR"
