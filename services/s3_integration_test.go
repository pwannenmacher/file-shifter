package services

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"file-shifter/config"
)

type fakeS3Server struct {
	mu                   sync.Mutex
	buckets              map[string]map[string][]byte
	forceObjectHeadError bool
	forceDeleteError     bool
}

func newFakeS3Server() *fakeS3Server {
	return &fakeS3Server{buckets: make(map[string]map[string][]byte)}
}

func (f *fakeS3Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bucketFromHost := ""
	hostOnly := strings.Split(r.Host, ":")[0]
	if !strings.EqualFold(hostOnly, "localhost") && net.ParseIP(hostOnly) == nil {
		if idx := strings.Index(hostOnly, "."); idx > 0 {
			bucketFromHost = hostOnly[:idx]
		}
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	bucket := ""
	key := ""

	if path == "" {
		bucket = bucketFromHost
	} else {
		parts := strings.SplitN(path, "/", 2)
		if bucketFromHost != "" {
			bucket = bucketFromHost
			unescaped, err := url.PathUnescape(path)
			if err == nil {
				key = unescaped
			} else {
				key = path
			}
		} else {
			bucket = parts[0]
			if len(parts) == 2 {
				unescaped, err := url.PathUnescape(parts[1])
				if err == nil {
					key = unescaped
				} else {
					key = parts[1]
				}
			}
		}
	}

	if bucket == "" {
		if r.Method == http.MethodGet {
			f.writeListBuckets(w)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		if key == "" && r.URL.Query().Has("location") {
			if _, ok := f.buckets[bucket]; ok {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></LocationConstraint>`))
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<Error><Code>NoSuchBucket</Code><Message>Not Found</Message></Error>"))
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return

	case http.MethodHead:
		if key == "" {
			if _, ok := f.buckets[bucket]; ok {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if objs, ok := f.buckets[bucket]; ok {
			if content, ok := objs[key]; ok {
				if f.forceObjectHeadError {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("<Error><Code>InternalError</Code><Message>error</Message></Error>"))
					return
				}
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
				w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
				w.Header().Set("ETag", "\"test-etag\"")
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<Error><Code>NoSuchKey</Code><Message>Not Found</Message></Error>"))
		return

	case http.MethodPut:
		if key == "" {
			if _, ok := f.buckets[bucket]; !ok {
				f.buckets[bucket] = make(map[string][]byte)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if _, ok := f.buckets[bucket]; !ok {
			f.buckets[bucket] = make(map[string][]byte)
		}
		body, _ := io.ReadAll(r.Body)
		f.buckets[bucket][key] = body
		w.Header().Set("ETag", "\"test-etag\"")
		w.WriteHeader(http.StatusOK)
		return

	case http.MethodDelete:
		if f.forceDeleteError {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("<Error><Code>InternalError</Code><Message>error</Message></Error>"))
			return
		}
		if objs, ok := f.buckets[bucket]; ok && key != "" {
			delete(objs, key)
		}
		w.WriteHeader(http.StatusNoContent)
		return

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3Server) writeListBuckets(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Owner><ID>1</ID><DisplayName>test</DisplayName></Owner>
  <Buckets></Buckets>
</ListAllMyBucketsResult>`))
}

func boolPtr(v bool) *bool {
	return &v
}

func TestMinIO_WithFakeS3Server(t *testing.T) {
	fake := newFakeS3Server()
	ts := httptest.NewServer(fake)
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	minioConn, err := NewMinIOConnection(host, "key", "secret", false)
	if err != nil {
		t.Fatalf("failed to create minio connection: %v", err)
	}

	if err := minioConn.HealthCheck(); err != nil {
		t.Fatalf("expected health check success, got: %v", err)
	}

	if err := minioConn.EnsureBucket("test-bucket"); err != nil {
		t.Fatalf("expected EnsureBucket success, got: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "obj.txt")
	if err := os.WriteFile(tmp, []byte("hello s3"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	for _, name := range []string{"folder/file.txt", "folder/data.json", "folder/doc.pdf", "folder/blob.bin"} {
		if _, err := minioConn.UploadFile(tmp, "test-bucket", name); err != nil {
			t.Fatalf("expected UploadFile success for %s, got: %v", name, err)
		}
	}

	exists, err := minioConn.ObjectExists("test-bucket", "folder/file.txt")
	if err != nil || !exists {
		t.Fatalf("expected uploaded object to exist, exists=%v err=%v", exists, err)
	}

	if err := minioConn.DeleteFile("test-bucket", "folder/file.txt"); err != nil {
		t.Fatalf("expected DeleteFile success, got: %v", err)
	}

	exists, err = minioConn.ObjectExists("test-bucket", "folder/file.txt")
	if err != nil {
		t.Fatalf("expected no error for missing object, got: %v", err)
	}
	if exists {
		t.Fatal("expected object to be deleted")
	}
}

func TestMinIO_ErrorBranchesWithFakeS3Server(t *testing.T) {
	t.Run("object exists returns backend error", func(t *testing.T) {
		fake := newFakeS3Server()
		fake.forceObjectHeadError = true
		ts := httptest.NewServer(fake)
		defer ts.Close()

		host := strings.TrimPrefix(ts.URL, "http://")
		conn, err := NewMinIOConnection(host, "key", "secret", false)
		if err != nil {
			t.Fatalf("failed to create minio connection: %v", err)
		}
		if err := conn.EnsureBucket("err-bucket"); err != nil {
			t.Fatalf("expected EnsureBucket success, got: %v", err)
		}

		tmp := filepath.Join(t.TempDir(), "obj.txt")
		if err := os.WriteFile(tmp, []byte("x"), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
		if _, err := conn.UploadFile(tmp, "err-bucket", "obj.txt"); err != nil {
			t.Fatalf("expected upload success before stat error, got: %v", err)
		}

		exists, err := conn.ObjectExists("err-bucket", "obj.txt")
		if err == nil {
			t.Fatalf("expected backend error from ObjectExists, exists=%v", exists)
		}
	})

	t.Run("delete file returns backend error", func(t *testing.T) {
		fake := newFakeS3Server()
		fake.forceDeleteError = true
		ts := httptest.NewServer(fake)
		defer ts.Close()

		host := strings.TrimPrefix(ts.URL, "http://")
		conn, err := NewMinIOConnection(host, "key", "secret", false)
		if err != nil {
			t.Fatalf("failed to create minio connection: %v", err)
		}
		if err := conn.EnsureBucket("err-delete-bucket"); err != nil {
			t.Fatalf("expected EnsureBucket success, got: %v", err)
		}

		err = conn.DeleteFile("err-delete-bucket", "some/key.txt")
		if err == nil {
			t.Fatal("expected DeleteFile to return backend error")
		}
	})
}

func TestFileHandler_S3SuccessWithFakeServer(t *testing.T) {
	fake := newFakeS3Server()
	ts := httptest.NewServer(fake)
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	target := config.OutputTarget{
		Type:      "s3",
		Path:      "s3://bucket-a/prefix",
		Endpoint:  host,
		AccessKey: "key",
		SecretKey: "secret",
		SSL:       boolPtr(false),
		Region:    "us-east-1",
	}

	manager := NewS3ClientManager()
	defer manager.Close()
	fh := NewFileHandler([]config.OutputTarget{target}, manager)

	tmp := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(tmp, []byte("payload"), 0o644); err != nil {
		t.Fatalf("failed to write payload file: %v", err)
	}

	if err := fh.copyToS3(tmp, "sub/file.txt", target); err != nil {
		t.Fatalf("expected copyToS3 success, got: %v", err)
	}

	if err := fh.deleteFromS3("sub/file.txt", target); err != nil {
		t.Fatalf("expected deleteFromS3 success, got: %v", err)
	}
}

func TestWorker_ValidateS3TargetSuccessWithFakeServer(t *testing.T) {
	fake := newFakeS3Server()
	ts := httptest.NewServer(fake)
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	worker := &Worker{S3ClientManager: NewS3ClientManager()}
	defer worker.S3ClientManager.Close()

	target := config.OutputTarget{
		Type:      "s3",
		Path:      "s3://bucket-a",
		Endpoint:  host,
		AccessKey: "key",
		SecretKey: "secret",
		SSL:       boolPtr(false),
		Region:    "us-east-1",
	}

	if err := worker.validateS3Target(target); err != nil {
		t.Fatalf("expected validateS3Target success, got: %v", err)
	}
}
