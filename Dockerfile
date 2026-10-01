# GoLC container image. Built and pushed to ghcr.io by .github/workflows/release.yml.
#
#   docker run -p 8091:8091 -p 8090:8090 -v golc-data:/data ghcr.io/sonar-solutions/sonar-golc
#
# The binaries live in /app; /data is the working directory, where GoLC reads
# config.json and writes Results/ and Logs/.

# Cross-compile on the build host's architecture instead of emulating the target.
FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=development

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN LDFLAGS="-X github.com/SonarSource-Demos/sonar-golc/assets.Version=${VERSION}" \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags=launcher -ldflags "$LDFLAGS" -o /out/app/golc-launcher golc-launcher.go golc.go \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags=resultsall -ldflags "$LDFLAGS" -o /out/app/ResultsAll ResultsAll.go \
 && cp -r imgs /out/app/imgs \
 && mkdir -p /out/data \
 && cp config_sample.json /out/data/config.json

# Static binaries need nothing but CA certificates, which distroless provides.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
COPY --from=build --chown=nonroot:nonroot /out/data /data
WORKDIR /data
ENV GOLC_DATA_DIR=/data
USER nonroot
EXPOSE 8091 8090
ENTRYPOINT ["/app/golc-launcher"]
