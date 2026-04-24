# lib-email-gateway

An outbound SMTP relay that scans every email body and attachment for
secrets, then either redacts them or quarantines the message.

## Pattern

```
app  ->  hush SMTP relay  ->  upstream SMTP
```

## Snippet

```go
s, _ := scanner.New(scanner.Options{MinConfidence: 0.95})

func onMessage(m *mail.Message) error {
    body, _ := io.ReadAll(m.Body)
    masked, findings, _ := s.Redact(string(body), "[REDACTED:%s]")
    if len(findings) > 0 {
        if policyQuarantine { return quarantine(m, findings) }
        m.Body = strings.NewReader(masked)
    }
    return upstream.Send(m)
}
```

Use an SMTP library like `mhale/smtpd` for the server side.
