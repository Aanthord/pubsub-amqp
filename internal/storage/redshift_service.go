package storage

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/config"
	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"go.uber.org/zap"
)

type S3Service interface {
	UploadFile(ctx context.Context, key string, content []byte) (string, error)
	GetFile(ctx context.Context, key string) ([]byte, error)
}

type s3Service struct {
	client *s3.S3
	bucket string
	logger *zap.SugaredLogger
}

func NewS3Service() (S3Service, error) {
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(config.GetEnv("AWS_REGION", "us-west-2")),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &s3Service{
		client: s3.New(sess),
		bucket: config.GetEnv("S3_BUCKET_NAME", ""),
		logger: sugar,
	}, nil
}

func (s *s3Service) UploadFile(ctx context.Context, key string, content []byte) (string, error) {
	start := time.Now()
	defer func() {
		metrics.S3UploadDuration.Observe(time.Since(start).Seconds())
	}()

	_, err := s.client.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(content),
	})
	if err != nil {
		s.logger.Errorw("Failed to upload to S3", "error", err, "key", key)
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	s3URI := fmt.Sprintf("s3://%s/%s", s.bucket, key)
	s.logger.Infow("File uploaded to S3", "uri", s3URI, "size", len(content))
	metrics.S3UploadsTotal.Inc()
	return s3URI, nil
}

func (s *s3Service) GetFile(ctx context.Context, key string) ([]byte, error) {
	start := time.Now()
	defer func() {
		metrics.S3DownloadDuration.Observe(time.Since(start).Seconds())
	}()

	result, err := s.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		s.logger.Errorw("Failed to get file from S3", "error", err, "key", key)
		return nil, fmt.Errorf("failed to get file from S3: %w", err)
	}
	defer result.Body.Close()

	content, err := ioutil.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object body: %w", err)
	}

	s.logger.Infow("File downloaded from S3", "key", key, "size", len(content))
	metrics.S3DownloadsTotal.Inc()
	return content, nil
}
