package resumes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileStore struct{ root string }

func NewFileStore(root string) (*FileStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("RESUME_STORAGE_DIRECTORY is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create resume storage directory: %w", err)
	}
	return &FileStore{root: root}, nil
}

func (s *FileStore) Put(_ context.Context, key string, content []byte) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "resume-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func (s *FileStore) Get(_ context.Context, key string) ([]byte, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *FileStore) path(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid resume object key")
	}
	return filepath.Join(s.root, clean), nil
}

type SupabaseStore struct {
	baseURL, serviceKey, bucket string
	client                      *http.Client
}

func NewSupabaseStore(baseURL, serviceKey, bucket string) (*SupabaseStore, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(serviceKey) == "" || strings.TrimSpace(bucket) == "" {
		return nil, fmt.Errorf("SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY, and RESUME_STORAGE_BUCKET are required together")
	}
	return &SupabaseStore{baseURL: strings.TrimRight(baseURL, "/"), serviceKey: serviceKey, bucket: bucket, client: &http.Client{Timeout: 20 * time.Second}}, nil
}

func (s *SupabaseStore) Put(ctx context.Context, key string, content []byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.objectURL(key), bytes.NewReader(content))
	if err != nil {
		return err
	}
	s.authorize(request)
	request.Header.Set("Content-Type", "application/pdf")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Supabase Storage upload returned %d", response.StatusCode)
	}
	return nil
}

func (s *SupabaseStore) Get(ctx context.Context, key string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.objectURL(key), nil)
	if err != nil {
		return nil, err
	}
	s.authorize(request)
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Supabase Storage download returned %d", response.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, MaxPDFBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > MaxPDFBytes {
		return nil, fmt.Errorf("stored resume exceeds maximum size")
	}
	return content, nil
}

func (s *SupabaseStore) authorize(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+s.serviceKey)
	request.Header.Set("apikey", s.serviceKey)
}

func (s *SupabaseStore) objectURL(key string) string {
	segments := strings.Split(key, "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return s.baseURL + "/storage/v1/object/" + url.PathEscape(s.bucket) + "/" + strings.Join(segments, "/")
}
