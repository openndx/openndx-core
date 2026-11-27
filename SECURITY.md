# Security Policy

## Supported Versions

OpenNDX is before version 1.0. Security fixes are made on `main` and released in the next version. Earlier releases don't receive backported fixes.

| Version                                                                       | Supported |
| ----------------------------------------------------------------------------- | --------- |
| The [latest release](https://github.com/openndx/openndx-core/releases/latest) | Yes       |
| Earlier releases                                                              | No        |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues, discussions or pull requests.**

Report them privately, in either of these ways:

- **GitHub private vulnerability reporting:** use the [report form](https://github.com/openndx/openndx-core/security/advisories/new), also reachable through **Report a vulnerability** on the repository's Security tab.
- **Email:** write to [security@opensource.lk](mailto:security@opensource.lk).

### What to Include

- A description of the vulnerability and the component it affects
- Steps to reproduce it, and the release or commit you tested
- Its potential impact
- A suggested fix, if you have one

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Resolution**: Depends on severity and complexity

We appreciate your responsible disclosure and will acknowledge your contribution once the vulnerability is confirmed and addressed.

## Scope

This policy covers what OpenNDX releases from this repository: the Orchestration Engine, Policy Decision Point, Consent Engine and Portal Backend, the Consent Portal, the `ondx` CLI, the Helm chart and the published container images.

It doesn't cover:

- the Admin and Member portals, which are paused, unsupported prototypes and aren't released;
- the mock providers in [dev/](dev/README.md), and the local defaults in `compose.yml` and `.env.example`, such as the `1234` secrets, which are for local development only;
- the third-party projects OpenNDX uses, such as ThunderID, OpenTelemetry and PostgreSQL. Please report those to the project concerned.

## Disclosure Policy

- We will notify you when we receive your report
- We will keep you informed of our progress
- We will credit you for the discovery (unless you prefer to remain anonymous)
- We will coordinate public disclosure after a fix is available
