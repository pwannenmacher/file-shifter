package services

import (
	"crypto/md5"
	"fmt"
	"testing"

	"file-shifter/config"
)

func TestNewS3ClientManager(t *testing.T) {
	manager := NewS3ClientManager()

	if manager == nil {
		t.Fatal("NewS3ClientManager() should not return nil")
	}

	if manager.clients == nil {
		t.Error("clients map should be initialized")
	}

	if len(manager.clients) != 0 {
		t.Error("clients map should be empty initially")
	}

	if manager.GetActiveClientCount() != 0 {
		t.Error("active client count should be 0 initially")
	}
}

func TestS3ClientManager_getClientKey(t *testing.T) {
	manager := NewS3ClientManager()

	tests := []struct {
		name         string
		config1      config.S3Config
		config2      config.S3Config
		shouldBeSame bool
	}{
		{
			name: "identical configs should have same key",
			config1: config.S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "key1",
				SecretKey: "secret1",
				SSL:       true,
				Region:    "us-east-1",
			},
			config2: config.S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "key1",
				SecretKey: "secret1",
				SSL:       true,
				Region:    "us-east-1",
			},
			shouldBeSame: true,
		},
		{
			name: "different endpoints should have different keys",
			config1: config.S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "key1",
				SecretKey: "secret1",
				SSL:       true,
				Region:    "us-east-1",
			},
			config2: config.S3Config{
				Endpoint:  "localhost:9000",
				AccessKey: "key1",
				SecretKey: "secret1",
				SSL:       true,
				Region:    "us-east-1",
			},
			shouldBeSame: false,
		},
		{
			name: "different access keys should have different keys",
			config1: config.S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "key1",
				SecretKey: "secret1",
				SSL:       true,
				Region:    "us-east-1",
			},
			config2: config.S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "key2",
				SecretKey: "secret1",
				SSL:       true,
				Region:    "us-east-1",
			},
			shouldBeSame: false,
		},
		{
			name: "different SSL settings should have different keys",
			config1: config.S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "key1",
				SecretKey: "secret1",
				SSL:       true,
				Region:    "us-east-1",
			},
			config2: config.S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "key1",
				SecretKey: "secret1",
				SSL:       false,
				Region:    "us-east-1",
			},
			shouldBeSame: false,
		},
		{
			name: "different regions should have different keys",
			config1: config.S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "key1",
				SecretKey: "secret1",
				SSL:       true,
				Region:    "us-east-1",
			},
			config2: config.S3Config{
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "key1",
				SecretKey: "secret1",
				SSL:       true,
				Region:    "eu-central-1",
			},
			shouldBeSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := manager.getClientKey(tt.config1)
			key2 := manager.getClientKey(tt.config2)

			if tt.shouldBeSame {
				if key1 != key2 {
					t.Errorf("Keys should be the same: %s != %s", key1, key2)
				}
			} else {
				if key1 == key2 {
					t.Errorf("Keys should be different but both are: %s", key1)
				}
			}
		})
	}
}

func TestS3ClientManager_getClientKey_Consistency(t *testing.T) {
	manager := NewS3ClientManager()

	config := config.S3Config{
		Endpoint:  "test.endpoint.com",
		AccessKey: "testkey",
		SecretKey: "testsecret",
		SSL:       true,
		Region:    "test-region",
	}

	// Generate key multiple times and verify consistency
	key1 := manager.getClientKey(config)
	key2 := manager.getClientKey(config)
	key3 := manager.getClientKey(config)

	if key1 != key2 || key2 != key3 {
		t.Error("getClientKey should return consistent results for the same config")
	}

	// Verify key format (should be a hex string)
	if len(key1) != 32 { // MD5 hash is 32 characters in hex
		t.Errorf("Expected key length 32, got %d", len(key1))
	}

	// Verify expected key value
	expectedData := fmt.Sprintf("%s:%s:%s:%t:%s",
		config.Endpoint,
		config.AccessKey,
		config.SecretKey,
		config.SSL,
		config.Region)
	expectedKey := fmt.Sprintf("%x", md5.Sum([]byte(expectedData)))

	if key1 != expectedKey {
		t.Errorf("Key mismatch. Got %s, expected %s", key1, expectedKey)
	}
}

func TestS3ClientManager_Close(t *testing.T) {
	manager := NewS3ClientManager()

	// Simulate having some clients in the manager
	// Note: We can't actually create real MinIO clients in tests without a real S3 server
	// So we'll test the structure and logic

	// Initially should have 0 clients
	if manager.GetActiveClientCount() != 0 {
		t.Error("Should start with 0 active clients")
	}

	// Close should not panic even with no clients
	manager.Close()

	// Should still have 0 clients after close
	if manager.GetActiveClientCount() != 0 {
		t.Error("Should have 0 active clients after Close()")
	}
}

func TestS3ClientManager_GetActiveClientCount(t *testing.T) {
	manager := NewS3ClientManager()

	// Initially should be 0
	if count := manager.GetActiveClientCount(); count != 0 {
		t.Errorf("Expected 0 active clients, got %d", count)
	}

	// Test thread safety by calling from multiple goroutines
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = manager.GetActiveClientCount()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should still be 0
	if count := manager.GetActiveClientCount(); count != 0 {
		t.Errorf("Expected 0 active clients after concurrent access, got %d", count)
	}
}

func TestS3ClientManager_GetOrCreateClient_InvalidConfig(t *testing.T) {
	manager := NewS3ClientManager()

	tests := []struct {
		name   string
		config config.S3Config
	}{
		{
			name: "empty endpoint",
			config: config.S3Config{
				Endpoint:  "",
				AccessKey: "key",
				SecretKey: "secret",
				SSL:       true,
				Region:    "region",
			},
		},
		{
			name: "invalid endpoint format",
			config: config.S3Config{
				Endpoint:  "not-a-valid-endpoint",
				AccessKey: "key",
				SecretKey: "secret",
				SSL:       true,
				Region:    "region",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := manager.GetOrCreateClient(tt.config)

			// Should return an error for invalid configs
			if err == nil {
				t.Error("Expected error for invalid config")
			}
			if client != nil {
				t.Error("Client should be nil when error occurs")
			}

			// Should not add failed clients to the cache
			if manager.GetActiveClientCount() != 0 {
				t.Error("Failed client creation should not increase active client count")
			}
		})
	}
}

// Test concurrent access to the manager
func TestS3ClientManager_ConcurrentAccess(t *testing.T) {
	manager := NewS3ClientManager()

	config1 := config.S3Config{
		Endpoint:  "test1.endpoint.com",
		AccessKey: "key1",
		SecretKey: "secret1",
		SSL:       true,
		Region:    "region1",
	}

	config2 := config.S3Config{
		Endpoint:  "test2.endpoint.com",
		AccessKey: "key2",
		SecretKey: "secret2",
		SSL:       false,
		Region:    "region2",
	}

	done := make(chan bool)
	errors := make(chan error, 20)

	// Start 10 goroutines trying to get clients
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			var testConfig config.S3Config
			if id%2 == 0 {
				testConfig = config1
			} else {
				testConfig = config2
			}

			// Try to get client multiple times
			for j := 0; j < 10; j++ {
				key := manager.getClientKey(testConfig)
				if key == "" {
					errors <- fmt.Errorf("goroutine %d: empty key generated", id)
					return
				}

				// Test GetActiveClientCount for thread safety
				_ = manager.GetActiveClientCount()
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check for errors
	close(errors)
	for err := range errors {
		t.Error(err)
	}

	// Verify the manager is still in a consistent state
	count := manager.GetActiveClientCount()
	if count < 0 {
		t.Errorf("Active client count should never be negative, got %d", count)
	}
}

// Benchmark tests
func BenchmarkS3ClientManager_getClientKey(b *testing.B) {
	manager := NewS3ClientManager()
	config := config.S3Config{
		Endpoint:  "s3.amazonaws.com",
		AccessKey: "AKIAIOSFODNN7EXAMPLE",
		SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SSL:       true,
		Region:    "us-east-1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.getClientKey(config)
	}
}

func BenchmarkS3ClientManager_GetActiveClientCount(b *testing.B) {
	manager := NewS3ClientManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetActiveClientCount()
	}
}

func BenchmarkS3ClientManager_ConcurrentGetActiveClientCount(b *testing.B) {
	manager := NewS3ClientManager()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			manager.GetActiveClientCount()
		}
	})
}
