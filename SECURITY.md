# Security Policy

## Supported Versions

ForgeTSS is actively developed for Stellar testnet validation. Until a formal audit is completed, production/mainnet use is at your own risk.

| Version | Supported |
|---------|-----------|
| 0.1.x   | Yes       |
| < 0.1   | No        |

## Reporting a Vulnerability

We take security vulnerabilities seriously. Thank you for responsibly disclosing any issues.

**Do NOT file a public GitHub issue for security matters.**

Instead, use the GitHub Security Advisory process:

1. Go to the **Security tab** on the repository
2. Click **"Report a vulnerability"**
3. Describe the issue with as much detail as possible

### Response Timeline

| Timeline | Action |
|----------|--------|
| Within 48 hours | Initial acknowledgment of your report |
| Within 7 days | Initial assessment and severity classification |
| Within 30 days | Fix or mitigation plan communicated |
| Ongoing | Coordination of public disclosure |

### In-Scope

- Channel account leasing race conditions (concurrent `FOR UPDATE SKIP LOCKED`)
- Sequence number collisions and replay attacks
- Fee-bump logic errors (incorrect bump factors, nonce reuse)
- Authentication bypass on the REST API
- Secret/seed handling and exposure in logs or responses
- Privilege escalation through malformed API requests

### Out-of-Scope

- Vulnerabilities in upstream Horizon or Soroban RPC implementations
- Denial-of-service attacks against external Stellar network endpoints
- Issues in Go standard library or third-party dependencies (report upstream instead)

### Note

ForgeTSS has not undergone a formal third-party security audit. While we follow defensive programming practices, the codebase should be independently reviewed before mainnet deployment.
