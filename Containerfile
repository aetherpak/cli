# Pin registry.fedoraproject.org/fedora-minimal:44
ARG FEDORA_DIGEST=sha256:673e2dd3288620989514c72e6b4b29fdd9b92adb59f12901505bd7348ff32b84
ARG CLI_VERSION=v0.6.1


FROM docker.io/library/golang:1.26-alpine AS cli-binary-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/aetherpak main.go

FROM registry.fedoraproject.org/fedora-minimal@${FEDORA_DIGEST} AS base

LABEL org.opencontainers.image.title="AetherPak CLI" \
      org.opencontainers.image.description="flatpak+ostree toolchain for aetherpak publish/site stages" \
      org.opencontainers.image.source="https://github.com/aetherpak/cli" \
      org.opencontainers.image.licenses="MIT"

# Install minimal tools needed for repository packaging, signature lookasides, and CLI setup.
RUN microdnf install -y --setopt=install_weak_deps=0 --nodocs \
        flatpak \
        ostree \
        ca-certificates \
        git \
        jq \
        curl \
        tar \
        gzip \
    && microdnf clean all \
    && rm -rf /var/cache/dnf /var/lib/dnf/history*

WORKDIR /workspace
CMD ["/bin/bash"]


FROM base AS builder

LABEL org.opencontainers.image.title="AetherPak Builder" \
      org.opencontainers.image.description="flatpak+ostree+flatpak-builder toolchain for local building of Flatpak applications"

# Install flatpak-builder and elfutils for local compile runs.
# flatpak-builder automatically sets up bubblewrap (bwrap) for sandboxing.
RUN microdnf install -y --setopt=install_weak_deps=0 --nodocs \
        flatpak-builder \
        elfutils \
    && microdnf clean all \
    && rm -rf /var/cache/dnf /var/lib/dnf/history*


FROM base AS cli

LABEL org.opencontainers.image.title="AetherPak CLI" \
      org.opencontainers.image.description="AetherPak image with the pre-baked aetherpak CLI"

COPY --from=cli-binary-builder /bin/aetherpak /usr/local/bin/aetherpak


FROM builder AS cli-builder

LABEL org.opencontainers.image.title="AetherPak Builder CLI" \
      org.opencontainers.image.description="AetherPak Builder image with the pre-baked aetherpak CLI"

COPY --from=cli-binary-builder /bin/aetherpak /usr/local/bin/aetherpak
