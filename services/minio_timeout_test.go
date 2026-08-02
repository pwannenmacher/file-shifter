package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// assertContextDeadline creates a context via the given factory and asserts
// that its deadline is approximately now + expected.
func assertContextDeadline(t *testing.T, expected time.Duration, factory func() (deadline time.Time, ok bool)) {
	t.Helper()

	before := time.Now()
	deadline, ok := factory()
	if !ok {
		t.Fatal("expected context with a deadline")
	}

	remaining := deadline.Sub(before)
	// Allow generous slack for slow test machines; the distinct timeout values
	// used by the tests differ by minutes, so this cannot mask a wrong branch.
	if remaining <= expected-5*time.Second || remaining > expected+5*time.Second {
		t.Fatalf("expected deadline about %v from now, got %v", expected, remaining)
	}
}

// TestMinIO_OperationContext_TimeoutConfiguration verifies that
// operationContext uses the configured timeout and falls back to the default
// for zero or negative values.
func TestMinIO_OperationContext_TimeoutConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		configured time.Duration
		expected   time.Duration
	}{
		{name: "configured timeout is used", configured: 90 * time.Second, expected: 90 * time.Second},
		{name: "zero falls back to default", configured: 0, expected: defaultOperationTimeout},
		{name: "negative falls back to default", configured: -3 * time.Second, expected: defaultOperationTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MinIO{OperationTimeout: tt.configured}
			assertContextDeadline(t, tt.expected, func() (time.Time, bool) {
				ctx, cancel := m.operationContext()
				defer cancel()
				deadline, ok := ctx.Deadline()
				return deadline, ok
			})
		})
	}
}

// TestMinIO_UploadContext_TimeoutConfiguration verifies that uploadContext
// uses the configured timeout and falls back to the default for zero or
// negative values.
func TestMinIO_UploadContext_TimeoutConfiguration(t *testing.T) {
	tests := []struct {
		name       string
		configured time.Duration
		expected   time.Duration
	}{
		{name: "configured timeout is used", configured: 2 * time.Minute, expected: 2 * time.Minute},
		{name: "zero falls back to default", configured: 0, expected: defaultUploadTimeout},
		{name: "negative falls back to default", configured: -1 * time.Second, expected: defaultUploadTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MinIO{UploadTimeout: tt.configured}
			assertContextDeadline(t, tt.expected, func() (time.Time, bool) {
				ctx, cancel := m.uploadContext()
				defer cancel()
				deadline, ok := ctx.Deadline()
				return deadline, ok
			})
		})
	}
}

// TestMinIO_NewConnection_DefaultTimeouts verifies that a freshly created
// connection carries the default operation and upload timeouts.
func TestMinIO_NewConnection_DefaultTimeouts(t *testing.T) {
	m, err := NewMinIOConnection("localhost:9000", "access", "secret", false)
	if err != nil {
		t.Fatalf("failed to create MinIO connection: %v", err)
	}
	if m.OperationTimeout != defaultOperationTimeout {
		t.Fatalf("expected default operation timeout %v, got %v", defaultOperationTimeout, m.OperationTimeout)
	}
	if m.UploadTimeout != defaultUploadTimeout {
		t.Fatalf("expected default upload timeout %v, got %v", defaultUploadTimeout, m.UploadTimeout)
	}
}

// TestMinIO_UploadFile_ContentTypeFallbackAndTimeout drives UploadFile with a
// real (offline) client against an unreachable endpoint: the content-type
// fallback for unknown extensions is exercised and the configured upload
// timeout bounds the failing call.
func TestMinIO_UploadFile_ContentTypeFallbackAndTimeout(t *testing.T) {
	m, err := NewMinIOConnection("127.0.0.1:1", "access", "secret", false)
	if err != nil {
		t.Fatalf("failed to create MinIO connection: %v", err)
	}
	m.UploadTimeout = 2 * time.Second

	srcFile := filepath.Join(t.TempDir(), "data.unknownext123")
	if err := os.WriteFile(srcFile, []byte("payload"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	start := time.Now()
	_, err = m.UploadFile(srcFile, "test-bucket", "data.unknownext123")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected upload against unreachable endpoint to fail")
	}
	if strings.Contains(err.Error(), ErrMinIOClientNotInitialized) {
		t.Fatalf("expected a transport/context error, not nil-client error: %v", err)
	}
	// The configured 2s timeout must bound the operation; allow ample slack
	// for retries and scheduling.
	if elapsed > 30*time.Second {
		t.Fatalf("upload was not bounded by the configured timeout, took %v", elapsed)
	}
}
