# Security reporting

Use the repository's private vulnerability-reporting feature on GitHub when
enabled. If it is unavailable, request a private contact from the maintainer
without posting sensitive reproduction details in a public issue.

Never include Prism passwords, user-session cookies, console JWTs, SSH private
keys, or identifying production inventories in reports. A useful sanitized
report includes the application version, OS/architecture, PC version, and the
error category or reproduction sequence.

The viewer binds to IPv4 loopback and authorizes a single selected upstream
console. It is designed to limit access by unrelated websites, not isolate
against malicious processes running as the same OS user. Upstream certificate
verification is enabled by default; `--insecure` is an explicit override.
