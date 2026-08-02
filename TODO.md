# TODO

## Test Gaps

- [x] Add tests for shutdown race resilience (`Stop()` while events are being produced).
- [x] Add tests for bounded retry behavior on repeated checksum mismatch.
- [x] Add tests for health endpoint timeout configuration.
- [x] Add tests for S3 context timeout behavior (`operationContext`/`uploadContext` incl. default fallback).

## Nice to Have

- [ ] End-to-end test for FTPS (`tls: true` on FTP targets) against a real FTPS server — currently only unit-tested.
- [ ] Translate remaining German comments/messages in test files (production code is English-only).
- [ ] Stabilize `TestFileWatcher_DirectoryDeletion` (fixed sleeps make it flaky under load).
- [ ] Report a missing input directory as unhealthy via the health endpoint (currently the service idles silently
      after the input directory is deleted).
- [ ] Recover automatically when the input directory is recreated (re-register the watcher instead of requiring
      a restart).
