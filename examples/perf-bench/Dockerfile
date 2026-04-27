# Hush perf-bench image.
#
# Tiny, self contained, deterministic: bundle one real public codebase
# at build time, download the released hush CLI at run time, time one
# `hush detect` over the bundle, print the numbers, exit.
#
# No Go toolchain in the runtime image. No clone at runtime. No
# library wrapper. Same image works on amd64 and arm64; the runtime
# script picks the matching release artifact.
#
#   docker buildx build --platform linux/arm64 \
#     -t hush-bench:latest \
#     -f examples/perf-bench/Dockerfile .

FROM debian:bookworm-slim AS sample
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates git \
      && rm -rf /var/lib/apt/lists/* \
      && git clone --depth=1 https://github.com/sirupsen/logrus /sample/logrus \
      && rm -rf /sample/logrus/.git

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl time \
      && rm -rf /var/lib/apt/lists/*
COPY --from=sample /sample /sample
COPY examples/perf-bench/run.sh /usr/local/bin/run.sh
RUN chmod +x /usr/local/bin/run.sh

ENV HUSH_VERSION=0.1.11
WORKDIR /sample
ENTRYPOINT ["/usr/local/bin/run.sh"]
