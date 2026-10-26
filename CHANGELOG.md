# Changelog

All notable changes to ThruBox Server are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Each release also has detailed notes in [`docs/releases/`](docs/releases/), which
become the body of its GitHub release.

## [Unreleased]

## [1.0.0] - 2026-10-26

First stable release. See the [release notes](docs/releases/v1.0.0.md) for an overview and diagrams.

### Added

- REST API: `POST /api/messages`, `GET /api/messages/{address}`, `DELETE /api/messages/{id}` and `GET /health`.
- Per-mailbox sequence numbers (`seq`) on every message, and `?after=<seq>&limit=<n>` on `GET /api/messages/{address}` for cursor-based reads. Numbers are never reused, even after deletes or expiry. ([#69])
- Message IDs derived from content: the SHA-256 of `to`, `from` and `payload`. Resending an identical message returns the stored one with `200` instead of creating a duplicate. ([#69])
- `security.trusted_proxies` / `RELAY_SECURITY_TRUSTED_PROXIES` to read the client IP correctly behind reverse proxies. ([#68])
- Configurable CORS allowlist for direct browser clients (`security.allowed_origins`). ([#32])
- Optional API key authentication (`security.api_key`).
- Message TTL with an hourly background purge (`messages.ttl_days`; `0` keeps messages until they are deleted).
- Configuration from `config.yaml` and `RELAY_*` environment variables, `PORT` as a fallback for hosting platforms, and `RELAY_CONFIG_PATH` for the config file location. ([#26], [#27])
- Multi-arch Docker images on GitHub Container Registry with provenance attestations and SBOM. ([#12])
- Self-hosting guides for Nginx and Cloudflare Tunnel, and a post-deployment security checklist. ([#11])

### Security

- The rate limiter no longer trusts client-supplied `X-Forwarded-For` or `X-Real-IP`. Rotating either header used to bypass the limit. ([#68])

[Unreleased]: https://github.com/AOSSIE-Org/ThruBox-Server/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/AOSSIE-Org/ThruBox-Server/releases/tag/v1.0.0
[#11]: https://github.com/AOSSIE-Org/ThruBox-Server/issues/11
[#12]: https://github.com/AOSSIE-Org/ThruBox-Server/issues/12
[#26]: https://github.com/AOSSIE-Org/ThruBox-Server/issues/26
[#27]: https://github.com/AOSSIE-Org/ThruBox-Server/issues/27
[#32]: https://github.com/AOSSIE-Org/ThruBox-Server/issues/32
[#68]: https://github.com/AOSSIE-Org/ThruBox-Server/issues/68
[#69]: https://github.com/AOSSIE-Org/ThruBox-Server/issues/69
