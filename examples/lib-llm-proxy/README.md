# lib-llm-proxy

An HTTP proxy in front of OpenAI, Anthropic, or any LLM that strips
secrets from outgoing prompts. Prevents an employee from accidentally
sending an API key to a third party.

## Pattern

```
client  ->  hush proxy  ->  LLM provider
              |
              scan + redact messages[].content
```

## Snippet

```go
s, _ := scanner.New(scanner.Options{MinConfidence: 0.9})

http.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
    var body map[string]any
    json.NewDecoder(r.Body).Decode(&body)
    for _, m := range body["messages"].([]any) {
        msg := m.(map[string]any)
        masked, _, _ := s.Redact(msg["content"].(string), "[REDACTED:%s]")
        msg["content"] = masked
    }
    // forward to upstream ...
})
```

## Deployment

Run as a sidecar to your app. Point the app's `ANTHROPIC_BASE_URL` or
`OPENAI_BASE_URL` at the proxy.
