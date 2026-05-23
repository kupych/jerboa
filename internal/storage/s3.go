package storage

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Client struct {
	client *minio.Client
	core   *minio.Core
	bucket string
}

func NewS3Client(endpoint, bucket, region, accessKey, secretKey string) (*S3Client, error) {
	// Strip scheme if provided
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")

	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: true,
		Region: region,
	}
	client, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, err
	}
	core, err := minio.NewCore(endpoint, opts)
	if err != nil {
		return nil, err
	}
	return &S3Client{client: client, core: core, bucket: bucket}, nil
}

func (s *S3Client) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *S3Client) PresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *S3Client) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// InitiateMultipart starts an S3 multipart upload and returns the uploadID.
func (s *S3Client) InitiateMultipart(ctx context.Context, key, contentType string) (string, error) {
	return s.core.NewMultipartUpload(ctx, s.bucket, key, minio.PutObjectOptions{
		ContentType: contentType,
	})
}

// PresignPart returns a short-lived presigned PUT URL for a single multipart part.
// The browser PUTs the chunk directly to this URL; the ETag in the response must
// be collected and sent back to CompleteMultipart.
func (s *S3Client) PresignPart(ctx context.Context, key, uploadID string, partNumber int, expiry time.Duration) (string, error) {
	u, err := s.client.Presign(ctx, "PUT", s.bucket, key, expiry, url.Values{
		"uploadId":   {uploadID},
		"partNumber": {strconv.Itoa(partNumber)},
	})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// CompleteMultipart assembles the uploaded parts into the final object.
func (s *S3Client) CompleteMultipart(ctx context.Context, key, uploadID string, parts []minio.CompletePart) error {
	_, err := s.core.CompleteMultipartUpload(ctx, s.bucket, key, uploadID, parts, minio.PutObjectOptions{})
	return err
}

// AbortMultipart cancels an in-progress multipart upload and frees S3 storage.
func (s *S3Client) AbortMultipart(ctx context.Context, key, uploadID string) error {
	return s.core.AbortMultipartUpload(ctx, s.bucket, key, uploadID)
}
