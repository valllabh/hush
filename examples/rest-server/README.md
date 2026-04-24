# REST sidecar

Expose hush as a tiny HTTP API. Deploy as a sidecar so any service in
any language can scan text.

## API

```
POST /scan
Content-Type: text/plain

<body>
```

Response:

```json
{
  "findings": [
    {"line": 3, "column": 14, "rule": "slack_token", "score": 0.97}
  ]
}
```

## Snippet

```go
s, _ := scanner.New(scanner.Options{MinConfidence: 0.9})

http.HandleFunc("/scan", func(w http.ResponseWriter, r *http.Request) {
    body, _ := io.ReadAll(r.Body)
    findings, _ := s.ScanReader(bytes.NewReader(body))
    json.NewEncoder(w).Encode(map[string]any{"findings": findings})
})
http.ListenAndServe(":8080", nil)
```

Run with `--threads=2 --workers=4` inside a small container. Memory
footprint stays under 200 MB.
