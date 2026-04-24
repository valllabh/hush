# Self contained hush image. Build from the hush repo root:
#
#   make vendor   (once, to populate vendor/)
#   docker build -t hush:latest .
#   docker run --rm -v "$PWD:/src:ro" hush:latest scan /src

ARG ORT_VERSION=1.20.0
ARG ORT_ARCH=x64

FROM golang:1.26-bookworm AS build
ARG ORT_VERSION
ARG ORT_ARCH
ENV CGO_ENABLED=1

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl && rm -rf /var/lib/apt/lists/*

RUN curl -sSL -o /tmp/ort.tgz \
      https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-${ORT_ARCH}-${ORT_VERSION}.tgz && \
    tar -xzf /tmp/ort.tgz -C /opt && \
    cp /opt/onnxruntime-linux-${ORT_ARCH}-${ORT_VERSION}/lib/* /usr/local/lib/ && \
    ldconfig && rm /tmp/ort.tgz

WORKDIR /src
COPY . .
RUN go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/hush ./cmd/hush

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /usr/local/lib/libonnxruntime.so* /usr/local/lib/
COPY --from=build /out/hush /usr/local/bin/hush
RUN ldconfig
USER nobody
ENTRYPOINT ["/usr/local/bin/hush"]
CMD ["--help"]
