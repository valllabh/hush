# GitHub Actions

Run hush on every PR. Upload findings to GitHub Code Scanning so they
show up in the Security tab.

## Workflow

`.github/workflows/hush.yml`:

```yaml
name: hush
on: [pull_request]
permissions:
  contents: read
  security-events: write

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - name: Install hush
        run: |
          curl -sSL https://github.com/valllabh/hush/releases/latest/download/hush_linux_amd64.tar.gz | tar xz
          sudo mv hush /usr/local/bin/
      - name: Scan PR diff
        run: |
          git diff --name-only origin/${{ github.base_ref }}... | \
            xargs hush scan --format sarif > hush.sarif || true
      - uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: hush.sarif
```

## Fail the build on findings

Drop `|| true` and add `--fail-on-finding`.

## PR comments

Combine with [reviewdog](https://github.com/reviewdog/reviewdog) to
surface findings as inline PR comments:

```
hush scan --format sarif . | reviewdog -f=sarif -reporter=github-pr-review
```
