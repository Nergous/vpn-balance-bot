# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.7

FROM golang:${GO_VERSION}-alpine AS build

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /src

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/vpn-balance-bot \
    ./cmd/vpn-balance-bot

RUN mkdir -p /runtime-data \
    && touch /runtime-data/.keep \
    && chown -R 65532:65532 /runtime-data

FROM build AS test

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go test ./...

FROM scratch AS runtime

ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="VPN Balance Bot" \
      org.opencontainers.image.description="Self-hosted Telegram bot for VPN subscription balances" \
      org.opencontainers.image.source="https://github.com/Nergous/vpn-balance-bot" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build --chown=65532:65532 /out/vpn-balance-bot /usr/local/bin/vpn-balance-bot
COPY --from=build --chown=65532:65532 /runtime-data /data

USER 65532:65532
WORKDIR /data
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/usr/local/bin/vpn-balance-bot", "doctor"]

ENTRYPOINT ["/usr/local/bin/vpn-balance-bot"]
