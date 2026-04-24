# lib-clipboard-watch

A desktop daemon that watches your clipboard and pops a warning when
you copy something that looks like a secret. Stops the "I just pasted
my API key into a public chat" footgun.

## Snippet

```go
import "golang.design/x/clipboard"

s, _ := hush.New(hush.Options{MinConfidence: 0.95})
clipboard.Init()
ch := clipboard.Watch(ctx, clipboard.FmtText)
for data := range ch {
    findings, _ := s.ScanString(string(data))
    if len(findings) > 0 {
        notify("Secret detected in clipboard: " + findings[0].Rule)
    }
}
```

macOS notifications via `osascript`, Linux via `notify-send`, Windows
via `toast`. Ship as a menu bar app using `systray`.
