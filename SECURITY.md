# Security Policy

## Supported Versions

Only the latest minor release line receives security fixes.

| Version | Supported |
| ------- | --------- |
| latest  | yes       |
| older   | no        |

## Reporting a Vulnerability

Please do not open public GitHub issues for security problems.

Report privately via GitHub Security Advisories:
https://github.com/valllabh/hush/security/advisories/new

Or email: vallabh.joshi@gmail.com with subject prefixed `[hush-security]`.

Include:
- a description of the issue and its impact
- steps to reproduce or a proof of concept
- affected hush version (`hush --version`)
- your contact info for follow up

You should receive an acknowledgement within 72 hours. A fix plan with
a disclosure timeline will follow within 7 days.

## Threat Model

Hush is a local scanner. It does not make network calls, does not send
samples anywhere, and does not require credentials. The primary risks
we take seriously:

- arbitrary file read through path traversal in scan targets
- denial of service through pathological inputs (extreme file sizes,
  deeply nested archives, regex catastrophic backtracking)
- supply chain compromise of the embedded model or dependencies

Hush is not a sandbox. Do not scan hostile binaries or rely on hush to
contain malicious inputs. Run hush as a non-privileged user on untrusted
paths.
