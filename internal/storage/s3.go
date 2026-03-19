// Package storage provides an S3-compatible object storage client.
// It works with both AWS S3 (production) and MinIO (local development).
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client wraps the AWS S3 SDK client and provides a simplified interface
// for storing and retrieving objects. It works with both real S3 and MinIO
// by configuring the endpoint and path style.
type S3Client struct {
	client *s3.Client
	bucket string
}

// S3Config holds the configuration for the S3 client.
type S3Config struct {
	Endpoint       string // e.g. "http://localhost:9000" for MinIO, "" for real S3
	Bucket         string // e.g. "odoodevtools-data"
	Region         string // e.g. "us-east-1"
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool // true for MinIO, false for real S3
}

// NewS3Client creates a new S3-compatible storage client.
// For MinIO (local dev): set Endpoint to MinIO URL and ForcePathStyle to true.
// For AWS S3 (production): leave Endpoint empty, ForcePathStyle false.
func NewS3Client(cfg S3Config) (*S3Client, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.Region = cfg.Region
			o.Credentials = credentials.NewStaticCredentialsProvider(
				cfg.AccessKey, cfg.SecretKey, "",
			)
			o.UsePathStyle = cfg.ForcePathStyle
		},
	}

	// For MinIO or custom endpoints, override the base endpoint.
	if cfg.Endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}

	client := s3.New(s3.Options{}, opts...)

	return &S3Client{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Put stores data at the given key with the specified content type.
func (c *S3Client) Put(ctx context.Context, key string, data []byte, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	_, err := c.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

// PutGzip stores gzip-compressed data at the given key.
func (c *S3Client) PutGzip(ctx context.Context, key string, data []byte) error {
	input := &s3.PutObjectInput{
		Bucket:          aws.String(c.bucket),
		Key:             aws.String(key),
		Body:            bytes.NewReader(data),
		ContentType:     aws.String("application/json"),
		ContentEncoding: aws.String("gzip"),
	}

	_, err := c.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("s3 put gzip %s: %w", key, err)
	}
	return nil
}

// Get retrieves the object at the given key.
func (c *S3Client) Get(ctx context.Context, key string) ([]byte, error) {
	output, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	defer output.Body.Close()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("s3 read body %s: %w", key, err)
	}
	return data, nil
}

// Delete removes the object at the given key.
func (c *S3Client) Delete(ctx context.Context, key string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}

// TraceKey builds the S3 key for a raw error traceback.
// Format: {tenant_id}/errors/{env_id}/{signature}/{timestamp}.json.gz
func TraceKey(tenantID, envID, signature, timestamp string) string {
	return fmt.Sprintf("%s/errors/%s/%s/%s.json.gz", tenantID, envID, signature, timestamp)
}
