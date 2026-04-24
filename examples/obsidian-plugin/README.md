# Obsidian plugin

Scans your vault for secrets. A lot of engineers dump API keys into
personal notes during debugging and forget about them.

## Shape

- Obsidian plugin written in TypeScript
- spawns `hush scan <vault-path>` on a configurable interval
- shows findings in a dedicated pane with click to jump to line

## Snippet

```ts
async function scanVault() {
    const vault = this.app.vault.getAbstractFileByPath('/').path;
    const { stdout } = await execFile('hush', ['scan', vault, '--format', 'json']);
    const findings: Finding[] = JSON.parse(stdout);
    renderFindingsView(findings);
}
```

Register a command `Hush: scan vault` and a settings tab to configure
the hush binary path and schedule.
