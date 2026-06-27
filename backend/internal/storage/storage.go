package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/example/dispscenario-analyst-v2/internal/observability"
)

type Storage struct {
	client       *minio.Client
	publicClient *minio.Client
	bucket       string
}

func New(endpoint, publicEndpoint, accessKey, secretKey, bucket, region string, secure bool) (*Storage, error) {
	internalHost := strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	publicHost := strings.TrimPrefix(strings.TrimPrefix(publicEndpoint, "http://"), "https://")

	client, err := minio.New(internalHost, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}

	publicClient, err := minio.New(publicHost, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: strings.HasPrefix(publicEndpoint, "https://"),
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("create public s3 client: %w", err)
	}

	return &Storage{client: client, publicClient: publicClient, bucket: bucket}, nil
}

func (s *Storage) EnsureBucket(ctx context.Context) (err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("s3", "ensure_bucket", started, err) }()
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}

func (s *Storage) PresignPut(ctx context.Context, key string, ttl time.Duration) (result *url.URL, err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("s3", "presign_put", started, err) }()
	return s.publicClient.PresignedPutObject(ctx, s.bucket, key, ttl)
}

func (s *Storage) PresignGet(ctx context.Context, key string, ttl time.Duration) (result *url.URL, err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("s3", "presign_get", started, err) }()
	return s.publicClient.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
}

func (s *Storage) Stat(ctx context.Context, key string) (result minio.ObjectInfo, err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("s3", "stat", started, err) }()
	return s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
}

func (s *Storage) Delete(ctx context.Context, key string) (err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("s3", "delete", started, err) }()
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *Storage) DeletePrefix(ctx context.Context, prefix string) (err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("s3", "delete_prefix", started, err) }()
	objects := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix: prefix, Recursive: true,
	})
	for result := range s.client.RemoveObjects(ctx, s.bucket, objects, minio.RemoveObjectsOptions{}) {
		if result.Err != nil {
			return result.Err
		}
	}
	return nil
}

func (s *Storage) Download(ctx context.Context, key, destination string) (err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("s3", "download", started, err) }()
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = object.Close() }()

	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, object); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (s *Storage) Upload(ctx context.Context, key, source, contentType string) (err error) {
	started := time.Now()
	defer func() { observability.ObserveDependency("s3", "upload", started, err) }()
	_, err = s.client.FPutObject(
		ctx, s.bucket, key, source,
		minio.PutObjectOptions{ContentType: contentType},
	)
	return err
}
