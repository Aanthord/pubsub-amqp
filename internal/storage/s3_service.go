package storage

import (
    "bytes"
    "context"
    "fmt"
    "io/ioutil"

    "github.com/aanthord/pubsub-amqp/internal/tracing"
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

func NewS3Service(region, bucket string, logger *zap.SugaredLogger) (S3Service, error) {
    sess, err := session.NewSession(&aws.Config{
        Region: aws.String(region),
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create AWS session: %w", err)
    }

    return &s3Service{
        client: s3.New(sess),
        bucket: bucket,
        logger: logger,
    }, nil
}

func (s *s3Service) UploadFile(ctx context.Context, key string, content []byte) (string, error) {
    span, ctx := tracing.StartSpanFromContext(ctx, "S3Upload", "")
    defer span.Finish()

    _, err := s.client.PutObjectWithContext(ctx, &s3.PutObjectInput{
        Bucket: aws.String(s.bucket),
        Key:    aws.String(key),
        Body:   bytes.NewReader(content),
    })
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "s3_upload_error", "error.message", err.Error())
        return "", fmt.Errorf("failed to upload to S3: %w", err)
    }

    s3URI := fmt.Sprintf("s3://%s/%s", s.bucket, key)
    span.LogKV("event", "s3_upload_success", "s3_uri", s3URI)
    return s3URI, nil
}

func (s *s3Service) GetFile(ctx context.Context, key string) ([]byte, error) {
    span, ctx := tracing.StartSpanFromContext(ctx, "S3Get", "")
    defer span.Finish()

    result, err := s.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
        Bucket: aws.String(s.bucket),
        Key:    aws.String(key),
    })
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "s3_get_error", "error.message", err.Error())
        return nil, fmt.Errorf("failed to get file from S3: %w", err)
    }
    defer result.Body.Close()

    content, err := ioutil.ReadAll(result.Body)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "s3_read_error", "error.message", err.Error())
        return nil, fmt.Errorf("failed to read S3 object body: %w", err)
    }

    span.LogKV("event", "s3_get_success", "key", key)
    return content, nil
}
