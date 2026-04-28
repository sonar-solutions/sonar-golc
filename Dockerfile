# Build stage: always run on the host arch (amd64 on GitHub) so we cross-compile instead of using QEMU.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -tags=webui -ldflags "-X main.version=v2.0" -o webui webui.go golc.go && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -tags=resultsall -o ResultsAll ResultsAll.go

# Run stage: minimal Alpine for fewest vulnerabilities and small size
FROM alpine:3.19

RUN apk add --no-cache ca-certificates && \
    adduser -D -g "" appuser

WORKDIR /app

COPY --from=builder /build/webui /build/ResultsAll ./
COPY --from=builder /build/dist ./dist
COPY --from=builder /build/imgs ./imgs
COPY docker-entrypoint.sh ./

RUN chmod +x docker-entrypoint.sh && chown -R appuser:appuser /app

USER appuser

# GOLC_DEVOPS: which platform to analyze (e.g. Github, Gitlab, BitBucket, File); must match a key in config.json
ENV GOLC_CONFIG_FILE=/config/config.json

VOLUME ["/config", "/data"]

WORKDIR /data

# GOLC_WEBUI_PORT (default 8091): web UI port (not used in Docker mode)
# GOLC_RESULTS_PORT / PORT (default 8090): ResultsAll dashboard port

EXPOSE 8090

ENTRYPOINT ["/app/docker-entrypoint.sh"]
