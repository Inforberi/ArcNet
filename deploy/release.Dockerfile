# syntax=docker/dockerfile:1
# Build host only; Docker, Node and Go are not required on the target VPS.
FROM golang:1.26.6-bookworm AS go-toolchain
FROM node:22.21.1-bookworm-slim AS node-toolchain
FROM ubuntu:22.04 AS builder
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    build-essential libsqlite3-dev libqrencode4 ca-certificates git openssh-client \
    && rm -rf /var/lib/apt/lists/*
COPY --from=node-toolchain /usr/local /usr/local
COPY --from=go-toolchain /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:/usr/local/bin:${PATH}"
WORKDIR /src
COPY . .
ARG ARCVPN_BUILD_VERSION=0.7.0-stage5-a3
ARG ARCVPN_SOURCE_REVISION=unknown
ARG ARCVPN_SOURCE_DIRTY=true
ENV ARCVPN_BUILD_VERSION=$ARCVPN_BUILD_VERSION
ENV ARCVPN_SOURCE_REVISION=$ARCVPN_SOURCE_REVISION
ENV ARCVPN_SOURCE_DIRTY=$ARCVPN_SOURCE_DIRTY
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/.npm \
    ./scripts/build-release.sh
FROM scratch AS artifacts
COPY --from=builder /src/dist/arcvpn-*-linux-amd64.tar.gz /
COPY --from=builder /src/dist/arcvpn-*-linux-amd64.tar.gz.sha256 /
COPY --from=builder /src/dist/arcvpn-*-source.tar.gz /
COPY --from=builder /src/dist/VERSION /VERSION
COPY --from=builder /src/dist/BUILDINFO.json /BUILDINFO.json
COPY --from=builder /src/SOURCE_MANIFEST.sha256 /SOURCE_MANIFEST.sha256
