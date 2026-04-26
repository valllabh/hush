#!/bin/sh
# Bench entrypoint. Clones a small set of public repos, then runs both
# the hush CLI and the in-process Go library against each. Prints one
# JSON line per (repo, mode) so an outer driver can ingest the numbers.
#
# Override the repo list via the REPOS env var (space-separated).
set -eu

REPOS="${REPOS:-https://github.com/sirupsen/logrus.git https://github.com/spf13/cobra.git https://github.com/Plazmaz/leaky-repo.git}"
WORK="${WORK:-/work}"
OUT="${OUT:-/tmp/bench.jsonl}"

mkdir -p "$WORK"
: > "$OUT"

run_one() {
  url="$1"
  name=$(basename "$url" .git)
  dir="$WORK/$name"

  if [ ! -d "$dir" ]; then
    echo "clone: $url" >&2
    git clone --depth=1 --quiet "$url" "$dir"
  fi
  size=$(du -sk "$dir" | cut -f1)
  echo "scan: $name (${size}K)" >&2

  # CLI mode: download-then-run release binary. Captures wallclock via
  # /usr/bin/time -f. Findings count = number of NDJSON lines.
  start=$(date +%s.%N)
  findings=$(/usr/local/bin/hush detect "$dir" 2>/dev/null | wc -l | tr -d ' ' || true)
  end=$(date +%s.%N)
  wall=$(awk -v s="$start" -v e="$end" 'BEGIN{printf "%.2f", e-s}')
  printf '{"mode":"cli","repo":"%s","findings":%s,"wall_seconds":%s,"size_kb":%s}\n' \
    "$name" "$findings" "$wall" "$size" >> "$OUT"

  # Library mode: in-process bench harness emits its own JSON.
  /usr/local/bin/perf-bench --repo "$dir" --json >> "$OUT" 2>/dev/null || true
}

for url in $REPOS; do
  run_one "$url"
done

echo >&2
echo "=== results ($OUT) ===" >&2
cat "$OUT"
