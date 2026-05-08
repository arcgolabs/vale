# Security Policy

## Supported Versions

Until the first public tag, security fixes target the main branch.

After v0.1.0 is tagged, security fixes target the latest v0.1.x patch release
and the main branch.

| Version | Supported |
| --- | --- |
| main | Yes |
| v0.1.x | Yes, after v0.1.0 is tagged |

## Reporting a Vulnerability

Do not publish exploit details in a public issue.

Use GitHub private vulnerability reporting when it is enabled for this
repository. If that is unavailable, contact the maintainers through the
repository owner channels and include:

- affected version or commit
- affected configuration
- reproduction steps
- expected impact
- any known mitigations

Maintainers should acknowledge reports before discussing fixes publicly.

## Security Defaults

Vale's current secure defaults include:

- TLS listeners use Go's secure TLS defaults with minimum TLS 1.2.
- ACME requires explicit domains and email.
- ACME defaults its cache directory to `.vale/acme` when omitted.
- The built-in default runtime enables bounded header and body settings.
- Unknown middleware types fail compilation instead of being ignored.
