# Live log tail

Stream your app's logs through hush to get paged the moment a secret
shows up in production logs.

## Pattern

```
tail -F /var/log/app.log | hush scan --stream --format json | hush-alert
```

`--stream` reads line by line, emits one JSON object per finding, never
buffers.

## Systemd

```ini
[Service]
ExecStart=/bin/sh -c 'journalctl -fu myapp | hush scan --stream --min-confidence 0.95 | /usr/local/bin/hush-alert'
```

## Filebeat / Fluent Bit

Run hush as a sidecar that consumes the log socket, then ships findings
(not the raw lines) to your SIEM. Keeps secrets out of the SIEM.
