# Shell history sweep

Developers regularly paste tokens into `export` or curl commands. Scan
your history weekly to catch leaks you already made.

## Ad hoc

```
hush scan ~/.bash_history ~/.zsh_history
```

## Weekly cron

```
0 9 * * 1  /usr/local/bin/hush scan ~/.bash_history ~/.zsh_history --format json > ~/.cache/hush-history.json
```

## What to do with findings

1. rotate the leaked credential
2. `sed -i '/SECRET_VALUE/d' ~/.bash_history` to scrub
3. remove from any terminal multiplexer scrollback
