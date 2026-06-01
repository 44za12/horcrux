package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Provider struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	client    *minio.Client
}

func NewS3Provider(endpoint, region, bucket, accessKey, secretKey string) *S3Provider {
	return &S3Provider{
		Endpoint:  endpoint,
		Region:    region,
		Bucket:    bucket,
		AccessKey: accessKey,
		SecretKey: secretKey,
	}
}

func (s *S3Provider) Name() string { return "s3" }

func (s *S3Provider) Authenticate(ctx context.Context) error {
	useSSL := true
	if s.Endpoint == "localhost:9000" || s.Endpoint == "127.0.0.1:9000" {
		useSSL = false
	}

	client, err := minio.New(s.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(s.AccessKey, s.SecretKey, ""),
		Region: s.Region,
		Secure: useSSL,
	})
	if err != nil {
		return fmt.Errorf("creating S3 client: %w", err)
	}

	exists, err := client.BucketExists(ctx, s.Bucket)
	if err != nil {
		return fmt.Errorf("checking bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, s.Bucket, minio.MakeBucketOptions{Region: s.Region}); err != nil {
			return fmt.Errorf("creating bucket: %w", err)
		}
	}

	s.client = client
	return nil
}

func (s *S3Provider) Upload(ctx context.Context, key string, data []byte) error {
	if s.client == nil {
		return fmt.Errorf("not authenticated")
	}

	reader := bytes.NewReader(data)
	_, err := s.client.PutObject(ctx, s.Bucket, "horcrux/"+key, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	return err
}

func (s *S3Provider) Download(ctx context.Context, key string) ([]byte, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	obj, err := s.client.GetObject(ctx, s.Bucket, "horcrux/"+key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("reading object: %w", err)
	}
	return data, nil
}

func (s *S3Provider) Delete(ctx context.Context, key string) error {
	if s.client == nil {
		return fmt.Errorf("not authenticated")
	}
	return s.client.RemoveObject(ctx, s.Bucket, "horcrux/"+key, minio.RemoveObjectOptions{})
}

func (s *S3Provider) Exists(ctx context.Context, key string) (bool, error) {
	if s.client == nil {
		return false, fmt.Errorf("not authenticated")
	}

	_, err := s.client.StatObject(ctx, s.Bucket, "horcrux/"+key, minio.StatObjectOptions{})
	if err != nil {
		if isS3NotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *S3Provider) List(ctx context.Context, prefix string) ([]string, error) {
	if s.client == nil {
		return nil, fmt.Errorf("not authenticated")
	}
	fullPrefix := "horcrux/" + prefix
	var keys []string
	for obj := range s.client.ListObjects(ctx, s.Bucket, minio.ListObjectsOptions{
		Prefix:    fullPrefix,
		Recursive: false,
	}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		keys = append(keys, strings.TrimPrefix(obj.Key, "horcrux/"))
	}
	return keys, nil
}

func isS3NotFound(err error) bool {
	if resp, ok := err.(minio.ErrorResponse); ok {
		return resp.Code == "NoSuchKey" || resp.Code == "NotFound"
	}
	return false
}
