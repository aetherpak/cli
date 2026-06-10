ARG BASE_IMAGE=ghcr.io/aetherpak/flatpak:latest
ARG BUILDER_IMAGE=ghcr.io/aetherpak/flatpak-builder:latest

FROM docker.io/library/golang:1.26-alpine AS cli-binary-builder

ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/aetherpak/aetherpak/cmd.Version=${VERSION}" -o /bin/aetherpak main.go


FROM ${BASE_IMAGE} AS cli

LABEL org.opencontainers.image.title="AetherPak CLI" \
      org.opencontainers.image.description="AetherPak image with the pre-baked aetherpak CLI"

COPY --from=cli-binary-builder /bin/aetherpak /usr/local/bin/aetherpak

WORKDIR /workspace
CMD ["/bin/bash"]


FROM ${BUILDER_IMAGE} AS cli-builder

LABEL org.opencontainers.image.title="AetherPak Builder CLI" \
      org.opencontainers.image.description="AetherPak Builder image with the pre-baked aetherpak CLI"

COPY --from=cli-binary-builder /bin/aetherpak /usr/local/bin/aetherpak

WORKDIR /workspace
CMD ["/bin/bash"]
