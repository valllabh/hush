# Browser extension (WASM)

Compile hush's classifier to WASM and run it inside a Chrome or Firefox
extension. Scan text pasted into web forms (GitHub issue bodies, Jira
comments, webmail) before the user hits submit.

## Build

```
GOOS=js GOARCH=wasm go build -o hush.wasm ./cmd/hush-wasm
```

The classifier runs in the browser. The embedded ONNX model is fetched
once and cached via the Cache API.

## Extension shape

- `manifest.json` with `host_permissions: ["<all_urls>"]`
- content script: hooks `paste` and `input` events
- scans via `postMessage` to a background worker running the WASM
- badge on the extension icon when a finding appears

Keep the WASM bundle under 30 MB by pruning rules and using model
quantization. Load it lazily on first activation.
