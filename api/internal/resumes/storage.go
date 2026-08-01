package resumes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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

type SpacesStore struct {
	bucket string
	client *s3.Client
}

func NewSpacesStore(ctx context.Context, endpoint, region, bucket, accessKeyID, secretAccessKey string) (*SpacesStore, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	region = strings.TrimSpace(region)
	bucket = strings.TrimSpace(bucket)
	accessKeyID = strings.TrimSpace(accessKeyID)
	secretAccessKey = strings.TrimSpace(secretAccessKey)
	if endpoint == "" || region == "" || bucket == "" || accessKeyID == "" || secretAccessKey == "" {
		return nil, fmt.Errorf("SPACES_ENDPOINT, SPACES_REGION, SPACES_BUCKET, SPACES_ACCESS_KEY_ID, and SPACES_SECRET_ACCESS_KEY are required together")
	}
	if !strings.HasPrefix(endpoint, "https://") {
		return nil, fmt.Errorf("SPACES_ENDPOINT must use HTTPS")
	}
	configuration, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("configure Spaces client: %w", err)
	}
	client := s3.NewFromConfig(configuration, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = false
	})
	return &SpacesStore{bucket: bucket, client: client}, nil
}

func (s *SpacesStore) Put(ctx context.Context, key string, content []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(content),
		ContentLength: aws.Int64(int64(len(content))),
		ContentType:   aws.String("application/pdf"),
		ACL:           types.ObjectCannedACLPrivate,
	})
	if err != nil {
		return fmt.Errorf("upload resume to Spaces: %w", err)
	}
	return nil
}

func (s *SpacesStore) Get(ctx context.Context, key string) ([]byte, error) {
	response, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("download resume from Spaces: %w", err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, MaxPDFBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > MaxPDFBytes {
		return nil, fmt.Errorf("stored resume exceeds maximum size")
	}
	return content, nil
}
