# Security

## Reporting

Email security concerns to the repository owner. Do not open public issues for active exploits or leaked credentials.

## Practices

- Secrets live only in gitignored env files (`prod.env`, `.env`) and CI/host secret stores — never in the repo.
- Session cookies are `HttpOnly`, `Secure`, `SameSite=None`, scoped to `COOKIE_DOMAIN`.
- Signup/login are rate-limited (Redis, fail-closed when Redis is unavailable).
- Personal access tokens are hashed at rest; plaintext is shown once at creation.
- Introspection requires a shared server secret (`INTROSPECT_SECRET`).

## CI scanners

Pull requests and `main` runs include:

- `gosec` / `govulncheck` (Go)
- `gitleaks` (secret scan)
- `trivy fs` (filesystem advisories; non-blocking while noisy)

## Scope notes

This service is an auth BFF in front of Keycloak. Treat Keycloak admin credentials and Mailgun/Resend API keys as highly sensitive.
