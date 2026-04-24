# CI blame bot

When hush finds a secret on `main`, automatically open a GitHub issue
tagging the author who introduced the line using `git blame`.

## Flow

1. post commit workflow runs `hush scan`
2. for each finding, `git blame -L <line> -- <file>` to get author + SHA
3. create issue with `gh issue create`, assign the author, label `security`

## Snippet (bash)

```bash
hush scan --format json . | jq -c '.[]' | while read -r f; do
  path=$(echo "$f" | jq -r .path)
  line=$(echo "$f" | jq -r .line)
  who=$(git blame -L "$line,$line" -- "$path" --porcelain | awk '/^author-mail/ {print $2; exit}' | tr -d '<>')
  gh issue create \
    --title "secret detected in $path:$line" \
    --body "Author: $who\nRule: $(echo "$f" | jq -r .rule)" \
    --assignee "$who" \
    --label security
done
```

Be ready for noise. Gate with a high confidence threshold and an
allowlist of paths.
