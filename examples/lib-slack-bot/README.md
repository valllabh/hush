# lib-slack-bot

A Slack app that watches public channels for pasted secrets. When it
sees one, it DMs the author, deletes the message, and posts a note to
a security channel.

## Flow

1. subscribe to `message.channels` events
2. scan `event.text` with hush
3. if findings, call `chat.delete`, `chat.postMessage` to user, log
   to `#sec-secrets`

## Snippet

```go
s, _ := scanner.New(scanner.Options{MinConfidence: 0.95})

func onMessage(ev *slackevents.MessageEvent) {
    findings, _ := s.ScanString(ev.Text)
    if len(findings) == 0 { return }
    slack.DeleteMessage(ev.Channel, ev.TimeStamp)
    slack.PostEphemeral(ev.Channel, ev.User, "A secret was detected. The message was removed. Please rotate the credential.")
    audit(ev, findings)
}
```

Use a high threshold (0.95+) since deletions are disruptive.
