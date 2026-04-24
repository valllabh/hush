# lib-data-masking

Replace detected secrets in a string with a redaction marker. The killer
primitive for building LLM prompt sanitizers, log scrubbers, and DLP tools.

## When to use

- cleaning user input before sending it to a third party API
- scrubbing values before writing to logs or telemetry
- preparing text for RAG ingestion

## Snippet

```go
import "github.com/valllabh/hush/pkg/scanner"

s, _ := scanner.New(scanner.Options{MinConfidence: 0.9})
defer s.Close()

masked, findings, _ := s.Redact(inputText, "[REDACTED:%s]")
// %s receives the rule id, e.g. "[REDACTED:aws_access_key]"
```

`Redact` returns the masked text and the list of findings so callers can
log or audit what was removed without leaking the original value.
