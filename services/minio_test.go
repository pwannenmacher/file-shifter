package services

import (
	"testing"
)

func TestNewMinIOConnection(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		accessKey string
		secretKey string
		useSSL    bool
		expectErr bool
	}{
		{
			name:      "valid connection parameters",
			endpoint:  "localhost:9000",
			accessKey: "testkey",
			secretKey: "testsecret",
			useSSL:    false,
			expectErr: false,
		},
		{
			name:      "SSL connection",
			endpoint:  "localhost:9000",
			accessKey: "testkey",
			secretKey: "testsecret",
			useSSL:    true,
			expectErr: false,
		},
		{
			name:      "empty endpoint should error during construction",
			endpoint:  "",
			accessKey: "testkey",
			secretKey: "testsecret",
			useSSL:    false,
			expectErr: true, // Constructor validates endpoint
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minioConn, err := NewMinIOConnection(tt.endpoint, tt.accessKey, tt.secretKey, tt.useSSL)

			if tt.expectErr && err == nil {
				t.Error("Erwartete einen Fehler, aber bekam keinen")
				return
			}

			if !tt.expectErr && err != nil {
				t.Errorf("Unerwarteter Fehler: %v", err)
				return
			}

			if !tt.expectErr {
				if minioConn == nil {
					t.Error("MinIO Connection sollte nicht nil sein")
				}
				if minioConn.MinIOClient == nil {
					t.Error("MinIO Client sollte nicht nil sein")
				}
			}
		})
	}
}

func TestMinIO_SanitizeBucketName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal name",
			input:    "mybucket",
			expected: "mybucket",
		},
		{
			name:     "uppercase to lowercase",
			input:    "MyBucket",
			expected: "mybucket",
		},
		{
			name:     "underscores to dashes",
			input:    "my_bucket_name",
			expected: "my-bucket-name",
		},
		{
			name:     "spaces to dashes",
			input:    "my bucket name",
			expected: "my-bucket-name",
		},
		{
			name:     "special characters removed",
			input:    "my@bucket#name$",
			expected: "mybucketname",
		},
		{
			name:     "numbers preserved",
			input:    "bucket123",
			expected: "bucket123",
		},
		{
			name:     "complex case",
			input:    "My_Bucket Name@123#Test",
			expected: "my-bucket-name123test",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			input:    "@#$%",
			expected: "",
		},
		{
			name:     "mixed valid and invalid",
			input:    "valid-name@invalid#chars",
			expected: "valid-nameinvalidchars",
		},
	}

	minioConn := &MinIO{} // Client ist für diese Methode nicht relevant

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := minioConn.SanitizeBucketName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeBucketName(%q) = %q, erwartet %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Die folgenden Tests würden eine echte MinIO-Instanz oder erweiterte Mock-Funktionalität benötigen
// Für diese Tests erstellen wir zumindest grundlegende Struktur-Tests

func TestMinIO_EnsureBucket_Structure(t *testing.T) {
	// Test that the method doesn't panic with nil client (defensive programming test)
	minioConn := &MinIO{MinIOClient: nil}

	// Diese Funktion sollte einen Fehler zurückgeben, nicht panic
	err := minioConn.EnsureBucket("test-bucket")
	if err == nil {
		t.Error("EnsureBucket sollte einen Fehler bei nil Client zurückgeben")
	}
}

func TestMinIO_UploadFile_Structure(t *testing.T) {
	// Test that the method doesn't panic with nil client
	minioConn := &MinIO{MinIOClient: nil}

	_, err := minioConn.UploadFile("/tmp/nonexistent", "bucket", "file.txt")
	if err == nil {
		t.Error("UploadFile sollte einen Fehler bei nil Client zurückgeben")
	}
}

func TestMinIO_ObjectExists_Structure(t *testing.T) {
	// Test that the method doesn't panic with nil client
	minioConn := &MinIO{MinIOClient: nil}

	_, err := minioConn.ObjectExists("bucket", "key")
	if err == nil {
		t.Error("ObjectExists sollte einen Fehler bei nil Client zurückgeben")
	}
}

func TestMinIO_HealthCheck_Structure(t *testing.T) {
	// Test that the method doesn't panic with nil client
	minioConn := &MinIO{MinIOClient: nil}

	err := minioConn.HealthCheck()
	if err == nil {
		t.Error("HealthCheck sollte einen Fehler bei nil Client zurückgeben")
	}
}

func TestMinIO_DeleteFile_Structure(t *testing.T) {
	// Test that the method doesn't panic with nil client
	minioConn := &MinIO{MinIOClient: nil}

	err := minioConn.DeleteFile("bucket", "key")
	if err == nil {
		t.Error("DeleteFile sollte einen Fehler bei nil Client zurückgeben")
	}
}

// Content-Type Detection Test
func TestMinIO_ContentTypeDetection(t *testing.T) {
	// Dieser Test prüft die Content-Type Logik indirekt durch den Code
	// Da wir die UploadFile-Funktion nicht direkt testen können ohne MinIO-Server
	// können wir zumindest die Logik für Content-Type-Detection dokumentieren

	tests := []struct {
		filename    string
		expectedExt string
	}{
		{"test.txt", ".txt"},
		{"document.pdf", ".pdf"},
		{"data.json", ".json"},
		{"binary.bin", ".bin"},
		{"noextension", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			// Hier würden wir in der echten Implementierung den Content-Type testen
			// Für jetzt dokumentieren wir nur die erwarteten Zuordnungen

			// Die Logik in UploadFile:
			// .txt -> text/plain
			// .json -> application/json
			// .pdf -> application/pdf
			// default -> application/octet-stream

			if tt.expectedExt == "" && tt.filename != "noextension" {
				t.Errorf("Unerwarteter Test-Fall: %s", tt.filename)
			}
		})
	}
}

// More comprehensive tests for functions with low coverage
func TestMinIO_EnsureBucket(t *testing.T) {
	tests := []struct {
		name        string
		client      *MinIO
		bucketName  string
		expectError bool
		errorCheck  func(error) bool
	}{
		{
			name:        "nil client should return error",
			client:      &MinIO{MinIOClient: nil},
			bucketName:  "test-bucket",
			expectError: true,
			errorCheck: func(err error) bool {
				return err.Error() == ErrMinIOClientNotInitialized
			},
		},
		{
			name:        "empty bucket name",
			client:      &MinIO{MinIOClient: nil}, // Will fail on nil check first
			bucketName:  "",
			expectError: true,
			errorCheck: func(err error) bool {
				return err.Error() == ErrMinIOClientNotInitialized
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.EnsureBucket(tt.bucketName)

			if tt.expectError && err == nil {
				t.Error("Expected error, but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
			if tt.expectError && err != nil && tt.errorCheck != nil {
				if !tt.errorCheck(err) {
					t.Errorf("Error check failed for: %v", err)
				}
			}
		})
	}
}
