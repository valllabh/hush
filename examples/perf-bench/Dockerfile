# Hush perf-bench image.
#
# Bundles two consumers of hush in one container so we can measure both
# in the same environment:
#
#   1. The hush CLI (downloaded as a release binary; tests the artifact
#      end users actually get).
#   2. A Go program (perf-bench) that imports hush as a library and
#      walks the same repos in-process.
#
# Designed to run on a small EC2 instance (t3.small / t4g.small or
# larger; t4g.nano lacks the RAM headroom for the v2 detector). On
# startup it shallow-clones a few public repos and runs both modes
# against each, emitting NDJSON to stdout.
#
#   docker build -t hush-bench:latest \
#     --build-arg HUSH_VERSION=v0.1.11 \
#     -f examples/perf-bench/Dockerfile .
#
#   docker run --rm hush-bench:latest
#
# Override targets via env:
#   REPOS="https://github.com/foo/bar.git https://github.com/baz/qux.git"

ARG HUSH_VERSION=v0.1.11

# ----- build stage: compile the bench harness from source -----
FROM golang:1.26-bookworm AS build
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
WORKDIR /src
COPY . .
RUN cd examples/perf-bench && \
    go build -ldflags="-s -w" -o /out/perf-bench .

# ----- download stage: pull the released hush CLI tarball -----
FROM debian:bookworm-slim AS fetch
ARG HUSH_VERSION
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl && rm -rf /var/lib/apt/lists/*

# Pick the right artifact for the current build platform. BuildKit
# exposes TARGETOS/TARGETARCH; default to linux_amd64 if absent.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN ARCH="$TARGETARCH"; \
    case "$ARCH" in \
      amd64) GOARCH=amd64 ;; \
      arm64) GOARCH=arm64 ;; \
      *)     echo "unsupported arch $ARCH" >&2; exit 1 ;; \
    esac; \
    URL="https://github.com/valllabh/hush/releases/download/${HUSH_VERSION}/hush_${TARGETOS}_${GOARCH}.tar.gz"; \
    echo "fetch $URL" >&2; \
    curl -fSL "$URL" -o /tmp/hush.tgz && \
    tar -xzf /tmp/hush.tgz -C /tmp && \
    install -m 0755 /tmp/hush /out/hush

# ----- runtime stage: minimal image with git + both binaries -----
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates git \
      && rm -rf /var/lib/apt/lists/*

COPY --from=build  /out/perf-bench /usr/local/bin/perf-bench
COPY --from=fetch  /out/hush       /usr/local/bin/hush
COPY examples/perf-bench/run.sh    /usr/local/bin/run.sh
RUN chmod +x /usr/local/bin/run.sh /usr/local/bin/perf-bench /usr/local/bin/hush

# Cache cloned repos under /work so repeated container runs reuse them.
WORKDIR /work
ENTRYPOINT ["/usr/local/bin/run.sh"]
