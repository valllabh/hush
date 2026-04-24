# pre-commit hook

Block commits that contain secrets, locally, before they ever reach a
remote.

## pre-commit framework

`.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/valllabh/hush
    rev: v0.1.0
    hooks:
      - id: hush
        args: ["--fail-on-finding", "--min-confidence=0.95"]
```

Install: `pre-commit install`.

## Plain git hook

`.git/hooks/pre-commit`:

```bash
#!/usr/bin/env bash
git diff --cached --name-only -z | xargs -0 hush scan --fail-on-finding
```

## Husky (JS projects)

`.husky/pre-commit`:

```
hush scan --staged --fail-on-finding
```

## Lefthook

```yaml
pre-commit:
  commands:
    hush:
      run: hush scan {staged_files} --fail-on-finding
```
