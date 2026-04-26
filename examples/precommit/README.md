# pre commit hook

Block commits that contain secrets or personal data, locally, before
they reach a remote.

## pre-commit framework

`.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/valllabh/hush
    rev: v0.1.8
    hooks:
      - id: hush
        args: ["--fail-on-finding", "--min-confidence=0.9"]
```

Install: `pre-commit install`.

## Plain git hook

`.git/hooks/pre-commit`:

```bash
#!/usr/bin/env bash
set -e
files=$(git diff --cached --name-only --diff-filter=ACM -z)
if [ -z "$files" ]; then
  exit 0
fi
echo "$files" | xargs -0 hush detect
```

If `hush detect` prints any findings, its non zero exit aborts the
commit.

## Husky (JS projects)

`.husky/pre-commit`:

```
git diff --cached --name-only --diff-filter=ACM | xargs hush detect
```

## Lefthook

```yaml
pre-commit:
  commands:
    hush:
      run: hush detect {staged_files}
```

## Why `hush detect`

`hush detect` uses the v2 detector and catches both secrets and personal
data (emails, phone numbers, names, addresses, SSNs, credit cards). The
older `hush scan` is still available if you want secret only coverage.
