#!/bin/sh
# Download the released hush CLI for the current arch, time one
# `hush detect` over the bundled sample codebase, print results.
set -eu

ARCH=$(uname -m)
case "$ARCH" in
  aarch64|arm64) GOARCH=arm64 ;;
  x86_64|amd64)  GOARCH=amd64 ;;
  *) echo "unsupported arch $ARCH" >&2; exit 1 ;;
esac

VERSION="${HUSH_VERSION:-0.1.11}"
URL="https://github.com/valllabh/hush/releases/download/v${VERSION}/hush_${VERSION}_linux_${GOARCH}.tar.gz"

echo "===== hush perf-bench ====="
echo "arch=${GOARCH}  hush=${VERSION}  url=${URL}"
echo

echo "----- download hush -----"
curl -fsSL "$URL" -o /tmp/hush.tgz
tar -xzf /tmp/hush.tgz -C /tmp
# Release tarball has a versioned top-level dir, e.g.
#   hush_0.1.11_linux_arm64/hush
HUSH_BIN=$(find /tmp -maxdepth 3 -type f -name hush -perm -u+x | head -1)
install -m 0755 "$HUSH_BIN" /usr/local/bin/hush
hush version
echo

SAMPLE="${SAMPLE:-/sample/logrus}"
echo "----- sample codebase -----"
files=$(find "$SAMPLE" -type f 2>/dev/null | wc -l | tr -d ' ')
bytes=$(du -sk "$SAMPLE" 2>/dev/null | cut -f1)
echo "path=${SAMPLE}  files=${files}  size=${bytes}K"
echo

echo "----- bench: hush detect -----"
# /usr/bin/time -v gives wall, user, sys, peak RSS. Capture findings
# to a side file so the line count doesn't pollute the timing block.
set +e
/usr/bin/time -v hush detect "$SAMPLE" > /tmp/findings.jsonl 2> /tmp/time.txt
exitcode=$?
set -e
findings=$(wc -l < /tmp/findings.jsonl | tr -d ' ')
echo "exit=${exitcode}  findings=${findings}"
echo
echo "----- /usr/bin/time output -----"
cat /tmp/time.txt
echo
echo "----- first 3 findings -----"
head -3 /tmp/findings.jsonl || true
