# perf-bench

A self contained Docker image that measures hush performance on real
public repositories. Bundles two consumers of hush so you can compare
them in the same environment:

1. The hush CLI binary (downloaded from the latest release; tests the
   artifact end users actually run).
2. A small Go program (`perf-bench`) that imports hush as a library and
   walks the same repos in-process.

Designed to run on a small EC2 instance with CPU only.

## Run locally

```
# From the hush repo root.
docker build -t hush-bench:latest \
  --build-arg HUSH_VERSION=v0.1.11 \
  -f examples/perf-bench/Dockerfile .

docker run --rm hush-bench:latest
```

The container clones a few small public repos, scans each with both
modes, and prints one JSON line per (repo, mode) result.

Override the repo list:

```
docker run --rm \
  -e REPOS="https://github.com/foo/bar.git https://github.com/baz/qux.git" \
  hush-bench:latest
```

## Run on EC2

Recommended: **t3.small** (2 vCPU, 2 GB RAM) or **t4g.small** (ARM,
same shape, cheaper). Both are about $0.02/hour.

`t3.nano` and `t4g.nano` (0.5 GB RAM) are **too small** for the v2
detector — the int8 model dequantises to ~316 MB resident at load.
Use `nano` only with `--v2=false` (legacy regex+classifier) which fits.

Sample EC2 deploy with the Docker daemon already running on the
instance:

```
# Pull or build the image.
docker pull ghcr.io/valllabh/hush-bench:latest

# Run and stream JSON to stdout.
docker run --rm ghcr.io/valllabh/hush-bench:latest

# Or persist the cloned repos between runs:
docker run --rm \
  -v /var/lib/hush-bench/work:/work \
  ghcr.io/valllabh/hush-bench:latest
```

## Output

One NDJSON line per measurement. Two modes per repo:

```json
{"mode":"cli","repo":"logrus","findings":3,"wall_seconds":2.41,"size_kb":1024}
{"mode":"lib-v2","repo":"logrus","files_scanned":34,"bytes_scanned":318211,"findings":3,"wall_seconds":2.18,"heap_alloc_peak_mb":92.4,"heap_inuse_peak_mb":118.6}
```

Fields:

- `mode` — `cli` (subprocess release binary) or `lib-v2` / `lib-v1` /
  `lib-v2-noprefilter` (in-process Go library).
- `findings` — total findings reported.
- `wall_seconds` — wallclock end-to-end including walk + scan.
- `heap_*_mb` — peak Go heap during the scan, library mode only.
- `size_kb` — disk size of the cloned repo (CLI mode only).

## What it scans

Default repo set, picked to be small + diverse:

- `sirupsen/logrus` — popular Go logging lib, mostly clean source.
- `spf13/cobra` — CLI framework, mostly clean source.
- `Plazmaz/leaky-repo` — deliberately seeded with fake credentials for
  detection tooling smoke tests. Expect findings here.

These together are under 30 MB on disk and finish in tens of seconds
on a t3.small.

## How the bench harness works

`main.go` is a minimal hush library consumer:

```go
s, _ := hush.New(hush.Options{
    UseDetector:       true,  // v2 NER detector
    DetectorPrefilter: true,  // skip model on clean files
    MinConfidence:     0.5,
})
defer s.Close()

for each file under --repo {
    findings, _ := s.ScanReader(f)
    ...sample heap, count findings...
}
```

It walks the cloned repo, skipping `.git`, `node_modules`, `vendor`,
and files larger than 1 MB (which usually are generated artifacts, not
source). For every file it samples `runtime.MemStats` so the JSON
captures peak heap during the scan.

`run.sh` runs the CLI variant first (subprocess + NDJSON line count)
then the library variant (in-process + JSON emit). Same repo, same
machine, same dependency-free environment.

## What this is NOT

- Not a unit benchmark. For micro-benchmarks see `pkg/native/*_test.go`
  (`go test -bench`).
- Not a quality benchmark. For precision/recall see the corpus gate in
  `pkg/scanner/corpus_test.go`.
- Not stable across hardware. Compare numbers from the same instance
  type when tracking regressions.
