# TODO - Remaining Hardening Tasks

## Open Security/Robustness Items

- [x] Enforce SFTP host key verification (no `ssh.InsecureIgnoreHostKey()` in production path).
  - Add known_hosts based validation and config flags for strict mode.
  - Fail closed by default if host key is unknown.

- [ ] Limit event fan-out to avoid unbounded goroutine growth under fsnotify storms.
  - Replace per-event goroutine spawning with a bounded event worker pipeline.
  - Add backpressure and metrics for dropped/throttled events.

- [x] Harden health server HTTP timeouts.
  - Set `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, and `IdleTimeout` on `http.Server`.

- [ ] Use operation-scoped context timeouts for S3/MinIO calls.
  - Replace `context.Background()` with `context.WithTimeout(...)` in bucket checks, uploads, stats, and deletes.
  - Make timeout configurable via config.

## Test Gaps

- [ ] Add tests for shutdown race resilience (`Stop()` while events are being produced).
- [ ] Add tests for bounded retry behavior on repeated checksum mismatch.
- [ ] Add tests for health endpoint timeout configuration.
- [ ] Add tests for S3 context timeout behavior.
