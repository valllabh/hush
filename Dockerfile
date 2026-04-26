# Self contained hush image. Pure Go, no CGO, no ONNX Runtime needed.
#
#   docker build -t hush:latest .
#   docker run --rm -v "$PWD:/src:ro" hush:latest detect /src
#
# Vendored deps optional (CI runs `go mod vendor` first); if vendor/
# is absent, the build pulls modules from the network.

FROM golang:1.26-bookworm AS build
ENV CGO_ENABLED=0 GOFLAGS=-trimpath

WORKDIR /src
COPY . .

RUN if [ -d vendor ]; then \
      go build -mod=vendor -ldflags="-s -w" -o /out/hush ./cmd/hush; \
    else \
      go build -ldflags="-s -w" -o /out/hush ./cmd/hush; \
    fi

# Distroless static is the smallest base that still ships ca-certificates
# and tzdata. The hush binary is statically linked so this is enough.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hush /usr/local/bin/hush
USER nonroot
ENTRYPOINT ["/usr/local/bin/hush"]
CMD ["--help"]
