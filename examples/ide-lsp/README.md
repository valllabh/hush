# Editor integration via LSP

Surface findings as squiggly underlines in VS Code, Neovim, Emacs, or
any LSP compatible editor. Developers see secrets as they type, before
they even save.

## Skeleton

Use [go.lsp.dev/protocol](https://pkg.go.dev/go.lsp.dev/protocol). Handle
`textDocument/didOpen` and `didChange`, scan the text, publish
diagnostics.

```go
func (h *Handler) publish(uri string, text string) {
    findings, _ := h.scanner.ScanString(text)
    var diags []protocol.Diagnostic
    for _, f := range findings {
        diags = append(diags, protocol.Diagnostic{
            Severity: protocol.SeverityError,
            Message:  "secret detected: " + f.Rule,
            Range:    rangeFor(f),
        })
    }
    h.client.PublishDiagnostics(protocol.PublishDiagnosticsParams{
        URI: uri, Diagnostics: diags,
    })
}
```

## Editor config

- VS Code: generic LSP client via `vscode-languageclient`
- Neovim: `nvim-lspconfig` custom server
- Emacs: `eglot` with `server-program` pointing at `hush lsp`
