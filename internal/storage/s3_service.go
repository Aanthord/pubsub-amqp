package storage

import (
    "bytes"
    "context"
    "fmt"
    "io/ioutil"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/metrics"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/aanthord/pubsub-amqp/internal/types"
    "github.com/aws/aws-sdk-go/aws"
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/aws/aws-sdk-go/service/s3"
    "go.uber.org/zap"
)

type S3Service interface {
    UploadFile(ctx context.Context, key string, content []byte, traceID string) (string, error)
    GetFile(ctx context.Context, key string) ([]byte, error)
}

type s3Service struct {
    client *s3.S3
    bucket string
    logger *zap.SugaredLogger
}

func NewS3Service(config types.ConfigProvider) (S3Service, error) {
    logger, _ := zap.NewProduction()
    sugar := logger.Sugar()

    sess, err := session.NewSession(&aws.Config{
        Region: aws.String(config.GetAWSRegion()),
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create AWS session: %w", err)
    }

    bucket := config.GetS3Bucket()

    return &s3Service{
        client: s3.New(sess),
        bucket: bucket,
        logger: sugar,
    }, nil
}

func (s *s3Service) UploadFile(ctx context.Context, key string, content []byte, traceID string) (string, error) {
    span, ctx := tracing.StartSpanFromContext(ctx, "S3Upload", traceID)
    defer span.Finish()

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
        span.SetTag("error", true)
        span.LogKV("event", "s3_upload_error", "error.message", err.Error())
        s.logger.Errorw("Failed to upload to S3", "error", err)
        return "", fmt.Errorf("failed to upload to S3: %w", err)
    }

    _, err = s.client.PutObjectTaggingWithContext(ctx, &s3.PutObjectTaggingInput{
        Bucket: aws.String(s.bucket),
        Key:    aws.String(key),
        Tagging: &s3.Tagging{
            TagSet: []*s3.Tag{
                {
                    Key:   aws.String("TraceID"),
                    Value: aws.String(traceID),
                },
                {
                    Key:   aws.String("Timestamp"),
                    Value: aws.String(time.Now().UTC().Format(time.RFC3339)),
                },
            },
        },
    })
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "s3_metadata_error", "error.message", err.Error())
        s.logger.Errorw("Failed to add metadata to S3 object", "error", err)
        return "", fmt.Errorf("failed to add metadata to S3 object: %w", err)
    }

    s3URI := fmt.Sprintf("s3://%s/%s", s.bucket, key)
    span.LogKV("event", "s3_upload_success", "s3_uri", s3URI)
    s.logger.Infow("File uploaded successfully to S3", "s3_uri", s3URI)
    metrics.S3UploadsTotal.Inc()
    return s3URI, nil
}

func (s *s3Service) GetFile(ctx context.Context, key string) ([]byte, error) {
    span, ctx := tracing.StartSpanFromContext(ctx, "S3Get", "")
    defer span.Finish()

    start := time.Now()
    defer func() {
        metrics.S3DownloadDuration.Observe(time.Since(start).Seconds())
    }()

    result, err := s.client.GetObjectWithContext(ctx, &s3.GetObjectInput{
        Bucket: aws.String(s.bucket),
        Key:    aws.String(key),
    })
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "s3_get_error", "error.message", err.Error())
        s.logger.Errorw("Failed to get file from S3", "error", err)
        return nil, fmt.Errorf("failed to get file from S3: %w", err)
    }
    defer result.Body.Close()

    content, err := ioutil.ReadAll(result.Body)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "s3_read_error", "error.message", err.Error())
        s.logger.Errorw("Failed to read S3 object body", "error", err)
        return nil, fmt.Errorf("failed to read S3 object body: %w", err)
    }

    taggingResult, err := s.client.GetObjectTaggingWithContext(ctx, &s3.GetObjectTaggingInput{
        Bucket: aws.String(s.bucket),
        Key:    aws.String(key),
    })
    if err != nil {
        s.logger.Warnw("Failed to get S3 object tags", "error", err)
    } else {
        for _, tag := range taggingResult.TagSet {
            span.LogKV(fmt.Sprintf("s3_metadata_%s", *tag.Key), *tag.Value)
        }
    }

    span.LogKV("event", "s3_get_success", "key", key)
    s.logger.Infow("File retrieved successfully from S3", "key", key)
    metrics.S3DownloadsTotal.Inc()
    return content, nil
}
