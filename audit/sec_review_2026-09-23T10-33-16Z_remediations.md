# Remediations of Security Review Findings

Review date and time: 2026-09-23T10:33:16Z (UTC, from system clock)

## Remediations

### Remediation of Finding 1: Any caller can delete any user's messages (no ownership check on DELETE)

- [ ] This remediation implements the security review's recommendation for this finding.
- [x] This remediation addresses the finding in a way that differs from the security review's recommendation.

Remediation commits:
- [856ccfd](https://github.com/AOSSIE-Org/ThruBox-Server/commit/856ccfdafa3aa6fdfa77d0979cf3da93c147effb) (PR #70)

Description: Instead of adding a deletion token, the `DELETE /api/messages/{id}` endpoint was removed, since no client used it. A `DELETE` now gets 405 Method Not Allowed, and messages leave the relay only by expiring (`ttl_days`). Authenticated deletion can be added later as a new endpoint. The client SDK's `delete()` was removed in AOSSIE-Org/ThruBox-Client#65.

### Remediation of Finding 2: Failed API-key attempts are never rate-limited (auth runs before the limiter)

- [ ] This remediation implements the security review's recommendation for this finding.
- [x] This remediation addresses the finding in a way that differs from the security review's recommendation.

Remediation commits:
- [8e5a57e](https://github.com/AOSSIE-Org/ThruBox-Server/commit/8e5a57eb62df151faaaa20a7f29aaa2304d06a89) (PR #67)

Description: Partly fixed. The rate limiter no longer trusts client-supplied `X-Forwarded-For` / `X-Real-IP`: by default it limits by the connecting address, and `security.trusted_proxies` sets how many proxy hops to trust. The middleware order (API-key check before the rate limiter) and the missing minimum key length are unchanged; see below.

## Non-remediated findings

### Finding 2 (remaining part): API-key check still runs before the rate limiter, and short keys are accepted

This part was not remediated because the API key is optional and is not treated as a secret: clients such as Chainvoice ship it in the browser bundle, so guessing it gives no extra access. Rate-limiting failed key checks and warning on short keys are left as a follow-up.
