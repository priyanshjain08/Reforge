# Security notes

**What is protected**
- All dashboard APIs need a bearer token (admin or read-only viewer); comparisons are constant-time.
- Agents authenticate with an ID and secret (stored only as a SHA-256 hash; shown once at creation) and can be revoked instantly.
- Strict JSON decoding with size limits, input validation on services/policies/agents, per-IP rate limiting, security headers.
- Recovery actions are an allowlist; targets must be explicitly authorised; the agent enforces its own allowlist too.
- Audit entries are append-only through the API; secrets are never logged or returned.

**Known limits (read before using beyond the demo)**
- The agent container mounts the Docker socket, which effectively grants root on the host. Run the demo only on a machine you control.
- Tokens are static and the API is plain HTTP. Put it behind TLS (reverse proxy) and rotate tokens for any real deployment.
- Service health URLs are fetched by the core: only admins can register them, but treat that as an SSRF-capable permission.
- State is stored unencrypted in a local file; the SHA-256 secret hash is not a password-grade KDF.
- No independent security review has been done.
