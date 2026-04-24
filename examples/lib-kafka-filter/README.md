# lib-kafka-filter

Consume a Kafka topic, redact secrets from each message, produce to a
clean topic. Use as a pre ingest filter for analytics pipelines that
should never see credentials.

## Pattern

```
topic.raw  ->  hush filter  ->  topic.clean
```

## Snippet

```go
s, _ := scanner.New(scanner.Options{MinConfidence: 0.9})
for msg := range consumer.Messages() {
    clean, findings, _ := s.Redact(string(msg.Value), "[REDACTED:%s]")
    if len(findings) > 0 {
        metrics.Inc("secrets_redacted", len(findings))
    }
    producer.Produce(&kafka.Message{Value: []byte(clean)})
}
```

Pair with a Kafka consumer library (segmentio/kafka-go or Shopify/sarama).
