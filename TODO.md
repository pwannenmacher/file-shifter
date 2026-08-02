# TODO

## Test Gaps

- [ ] Add tests for shutdown race resilience (`Stop()` while events are being produced).
- [ ] Add tests for bounded retry behavior on repeated checksum mismatch.
- [ ] Add tests for health endpoint timeout configuration.
- [ ] Add tests for S3 context timeout behavior (`operationContext`/`uploadContext` incl. default fallback).

## Nice to Have

- [ ] End-to-end test for FTPS (`tls: true` on FTP targets) against a real FTPS server — currently only unit-tested.
- [ ] Translate remaining German comments/messages in test files (production code is English-only).
- [ ] Stabilize `TestFileWatcher_DirectoryDeletion` (fixed sleeps make it flaky under load).
