# lib-rag-ingest

Scan documents before they enter a RAG (retrieval augmented generation)
pipeline. Without this step, secrets embedded in internal docs end up in
the vector DB and leak through search results.

## Pattern

```
source docs  ->  chunker  ->  hush redact  ->  embedder  ->  vector DB
```

## Snippet

```go
s, _ := scanner.New(scanner.Options{MinConfidence: 0.85})
for _, chunk := range chunks {
    clean, findings, _ := s.Redact(chunk.Text, "[REDACTED:%s]")
    if len(findings) > 0 {
        audit.Log(chunk.SourcePath, findings)
    }
    chunk.Text = clean
    embedder.Embed(chunk)
}
```

Lower the threshold here (0.85) versus CI (0.95). RAG leaks are more
costly than a few extra redactions.
