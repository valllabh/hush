# lib-logging-hook

A `slog.Handler` (or `zap.Core`) that redacts secrets from every log
line before write. Catches the "we accidentally logged the JWT" class
of incidents without requiring code changes across your service.

## Snippet

```go
type redactHandler struct {
    inner slog.Handler
    s     *scanner.Scanner
}

func (h *redactHandler) Handle(ctx context.Context, r slog.Record) error {
    r.Message, _, _ = h.s.Redact(r.Message, "[REDACTED:%s]")
    r.Attrs(func(a slog.Attr) bool {
        if s, ok := a.Value.Any().(string); ok {
            clean, _, _ := h.s.Redact(s, "[REDACTED:%s]")
            a.Value = slog.StringValue(clean)
        }
        return true
    })
    return h.inner.Handle(ctx, r)
}

slog.SetDefault(slog.New(&redactHandler{inner: slog.NewJSONHandler(os.Stdout, nil), s: s}))
```

Cost is ~1 ms per log line. If that matters, gate on a config flag or
sample.
