package config

import (
	"testing"
)

func TestOutputTarget_GetS3Config(t *testing.T) {
	tests := []struct {
		name     string
		target   OutputTarget
		expected S3Config
	}{
		{
			name: "complete S3 config",
			target: OutputTarget{
				Path:      "s3://bucket/path",
				Type:      "s3",
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
				SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SSL:       boolPtr(true),
				Region:    "eu-central-1",
			},
			expected: S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "AKIAIOSFODNN7EXAMPLE",
				SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SSL:       true,
				Region:    "eu-central-1",
			},
		},
		{
			name: "S3 config with SSL false",
			target: OutputTarget{
				Path:      "s3://bucket/path",
				Type:      "s3",
				Endpoint:  "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				SSL:       boolPtr(false),
				Region:    "us-east-1",
			},
			expected: S3Config{
				Endpoint:  "localhost:9000",
				AccessKey: "minioadmin",
				SecretKey: "minioadmin",
				SSL:       false,
				Region:    "us-east-1",
			},
		},
		{
			name: "S3 config with default SSL (nil pointer)",
			target: OutputTarget{
				Path:      "s3://bucket/path",
				Type:      "s3",
				Endpoint:  "s3.example.com",
				AccessKey: "testkey",
				SecretKey: "testsecret",
				SSL:       nil, // Should default to true
				Region:    "us-west-2",
			},
			expected: S3Config{
				Endpoint:  "s3.example.com",
				AccessKey: "testkey",
				SecretKey: "testsecret",
				SSL:       true, // Default value
				Region:    "us-west-2",
			},
		},
		{
			name: "minimal S3 config",
			target: OutputTarget{
				Path:      "s3://bucket",
				Type:      "s3",
				Endpoint:  "endpoint",
				AccessKey: "key",
				SecretKey: "secret",
				Region:    "region",
			},
			expected: S3Config{
				Endpoint:  "endpoint",
				AccessKey: "key",
				SecretKey: "secret",
				SSL:       true, // Default
				Region:    "region",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.target.GetS3Config()

			if result.Endpoint != tt.expected.Endpoint {
				t.Errorf("Endpoint = %q, want %q", result.Endpoint, tt.expected.Endpoint)
			}
			if result.AccessKey != tt.expected.AccessKey {
				t.Errorf("AccessKey = %q, want %q", result.AccessKey, tt.expected.AccessKey)
			}
			if result.SecretKey != tt.expected.SecretKey {
				t.Errorf("SecretKey = %q, want %q", result.SecretKey, tt.expected.SecretKey)
			}
			if result.SSL != tt.expected.SSL {
				t.Errorf("SSL = %v, want %v", result.SSL, tt.expected.SSL)
			}
			if result.Region != tt.expected.Region {
				t.Errorf("Region = %q, want %q", result.Region, tt.expected.Region)
			}
		})
	}
}

func TestOutputTarget_GetFTPConfig(t *testing.T) {
	tests := []struct {
		name     string
		target   OutputTarget
		expected FTPConfig
	}{
		{
			name: "complete FTP config with custom port",
			target: OutputTarget{
				Path:     "ftp://server/path",
				Type:     "ftp",
				Host:     "ftp.example.com",
				Username: "ftpuser",
				Password: "ftppass",
				Port:     2121,
			},
			expected: FTPConfig{
				Host:     "ftp.example.com",
				Username: "ftpuser",
				Password: "ftppass",
				Port:     2121,
			},
		},
		{
			name: "FTP config with default port (port = 0)",
			target: OutputTarget{
				Path:     "ftp://server/path",
				Type:     "ftp",
				Host:     "ftp.server.com",
				Username: "user",
				Password: "pass",
				Port:     0, // Should default to 21
			},
			expected: FTPConfig{
				Host:     "ftp.server.com",
				Username: "user",
				Password: "pass",
				Port:     21, // Default FTP port
			},
		},
		{
			name: "SFTP config with default port (port = 0)",
			target: OutputTarget{
				Path:     "sftp://server/path",
				Type:     "sftp",
				Host:     "sftp.server.com",
				Username: "sftpuser",
				Password: "sftppass",
				Port:     0, // Should default to 22
			},
			expected: FTPConfig{
				Host:     "sftp.server.com",
				Username: "sftpuser",
				Password: "sftppass",
				Port:     22, // Default SFTP port
			},
		},
		{
			name: "SFTP config with custom port",
			target: OutputTarget{
				Path:     "sftp://server/path",
				Type:     "sftp",
				Host:     "custom.sftp.com",
				Username: "sftpuser",
				Password: "sftppass",
				Port:     2222,
			},
			expected: FTPConfig{
				Host:     "custom.sftp.com",
				Username: "sftpuser",
				Password: "sftppass",
				Port:     2222,
			},
		},
		{
			name: "minimal FTP config",
			target: OutputTarget{
				Path:     "ftp://minimal",
				Type:     "ftp",
				Host:     "host",
				Username: "user",
				Password: "pass",
			},
			expected: FTPConfig{
				Host:     "host",
				Username: "user",
				Password: "pass",
				Port:     21, // Default FTP port
			},
		},
		{
			name: "minimal SFTP config",
			target: OutputTarget{
				Path:     "sftp://minimal",
				Type:     "sftp",
				Host:     "host",
				Username: "user",
				Password: "pass",
			},
			expected: FTPConfig{
				Host:     "host",
				Username: "user",
				Password: "pass",
				Port:     22, // Default SFTP port
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.target.GetFTPConfig()

			if result.Host != tt.expected.Host {
				t.Errorf("Host = %q, want %q", result.Host, tt.expected.Host)
			}
			if result.Username != tt.expected.Username {
				t.Errorf("Username = %q, want %q", result.Username, tt.expected.Username)
			}
			if result.Password != tt.expected.Password {
				t.Errorf("Password = %q, want %q", result.Password, tt.expected.Password)
			}
			if result.Port != tt.expected.Port {
				t.Errorf("Port = %d, want %d", result.Port, tt.expected.Port)
			}
		})
	}
}

func TestOutputTarget_GetS3Config_SSLDefault(t *testing.T) {
	// Test that SSL defaults to true when not specified
	target := OutputTarget{
		Path:      "s3://test",
		Type:      "s3",
		Endpoint:  "endpoint",
		AccessKey: "key",
		SecretKey: "secret",
		Region:    "region",
		// SSL is not set (nil)
	}

	config := target.GetS3Config()
	if !config.SSL {
		t.Error("SSL should default to true when not specified")
	}
}

func TestOutputTarget_GetFTPConfig_PortDefaults(t *testing.T) {
	tests := []struct {
		name         string
		targetType   string
		expectedPort int
	}{
		{
			name:         "FTP defaults to port 21",
			targetType:   "ftp",
			expectedPort: 21,
		},
		{
			name:         "SFTP defaults to port 22",
			targetType:   "sftp",
			expectedPort: 22,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := OutputTarget{
				Path:     "protocol://server/path",
				Type:     tt.targetType,
				Host:     "server.com",
				Username: "user",
				Password: "pass",
				Port:     0, // Should trigger default
			}

			config := target.GetFTPConfig()
			if config.Port != tt.expectedPort {
				t.Errorf("Port = %d, want %d for type %s", config.Port, tt.expectedPort, tt.targetType)
			}
		})
	}
}

func TestOutputTarget_GetFTPConfig_UnknownType(t *testing.T) {
	// Test behavior with unknown type (should default to FTP port 21)
	target := OutputTarget{
		Path:     "unknown://server/path",
		Type:     "unknown",
		Host:     "server.com",
		Username: "user",
		Password: "pass",
		Port:     0,
	}

	config := target.GetFTPConfig()
	if config.Port != 21 {
		t.Errorf("Port = %d, want 21 for unknown type (should default to FTP)", config.Port)
	}
}

// Benchmark tests
func BenchmarkOutputTarget_GetS3Config(b *testing.B) {
	target := OutputTarget{
		Path:      "s3://benchmark-bucket/path",
		Type:      "s3",
		Endpoint:  "s3.amazonaws.com",
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
		SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SSL:       boolPtr(true),
		Region:    "eu-central-1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		target.GetS3Config()
	}
}

func BenchmarkOutputTarget_GetFTPConfig(b *testing.B) {
	target := OutputTarget{
		Path:     "ftp://benchmark.server.com/path",
		Type:     "ftp",
		Host:     "benchmark.server.com",
		Username: "benchuser",
		Password: "benchpass",
		Port:     2121,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		target.GetFTPConfig()
	}
}
