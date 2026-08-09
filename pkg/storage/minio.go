package storage

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"time"

	"github.com/jblabs/tripmate-be/pkg/apperror"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/lifecycle"
)

type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

// S3 talks to any S3-compatible bucket (MinIO in dev, S3 in production). The bucket it creates is
// private by default — nothing here ever sets a public-read policy (ST-2).
type S3 struct {
	client *minio.Client
	bucket string
}

func NewS3(ctx context.Context, cfg S3Config) (*S3, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	store := &S3{client: client, bucket: cfg.Bucket}
	if err = store.ensureBucket(ctx, cfg.Region); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *S3) ensureBucket(ctx context.Context, region string) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	if !exists {
		if err = s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region}); err != nil {
			return apperror.Wrap(err, "INTERNAL_ERROR")
		}
	}
	// ST-6: images expire after 90 days. The extracted line items are the durable record, so losing
	// the photograph later costs nothing. A backend that cannot express lifecycle rules is not fatal.
	config := lifecycle.NewConfiguration()
	config.Rules = []lifecycle.Rule{{
		ID:         "expire-receipt-images",
		Status:     "Enabled",
		RuleFilter: lifecycle.Filter{Prefix: "trips/"},
		Expiration: lifecycle.Expiration{Days: lifecycle.ExpirationDays(RetentionDays)},
	}}
	if err = s.client.SetBucketLifecycle(ctx, s.bucket, config); err != nil {
		slog.Warn("could not set the receipt image lifecycle rule", "bucket", s.bucket, "error", err)
	}
	return nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}

func (s *S3) Get(ctx context.Context, key string) ([]byte, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, apperror.Wrap(err, "INTERNAL_ERROR")
	}
	defer object.Close()
	data, err := io.ReadAll(object)
	if err != nil {
		return nil, apperror.New("RECEIPT_NOT_FOUND")
	}
	return data, nil
}

func (s *S3) PresignedGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	link, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return link.String(), nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return apperror.Wrap(err, "INTERNAL_ERROR")
	}
	return nil
}
