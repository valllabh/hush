# Backup sweep

Scan S3 database dumps or snapshot archives before archiving or
restoring. Catches secrets that rotated into a dump before you knew to
purge them.

## Lambda handler

```go
import "github.com/aws/aws-lambda-go/events"

s, _ := scanner.New(scanner.Options{MinConfidence: 0.9})

func handler(ctx context.Context, ev events.S3Event) error {
    for _, rec := range ev.Records {
        obj, _ := s3.GetObject(rec.S3.Bucket.Name, rec.S3.Object.Key)
        findings, _ := s.ScanReader(obj.Body)
        if len(findings) > 0 {
            sns.Publish("backup-alarm", rec.S3.Object.Key, findings)
        }
    }
    return nil
}
```

## Cron based

Schedule nightly via EventBridge. Pair with Athena for a queryable
finding table.
