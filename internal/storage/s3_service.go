package storage

import (
    "bytes"
    "context"
    "fmt"
    "io/ioutil"
    "net/http"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/metrics"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/aanthord/pubsub-amqp/internal/types"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "go.uber.org/zap"
)

type S3Service interface {
    UploadFile(ctx context.Context, key string, content []byte, traceID string) (string, error)
    GetFile(ctx context.Context, key string) ([]byte, error)
}

type s3Service struct {
    client *s3.Client
    bucket string
    logger *zap.SugaredLogger
    signer *v4.Signer
    creds  aws.Credentials
}

func NewS3Service(configProvider types.ConfigProvider) (S3Service, error) {
    logger, _ := zap.NewProduction()
    sugar := logger.Sugar()

    cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(configProvider.GetAWSRegion()))
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
    }

    bucket := configProvider.GetS3Bucket()
    signer := v4.NewSigner()
    creds, err := cfg.Credentials.Retrieve(context.TODO())
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve AWS credentials: %w", err)
    }

    return &s3Service{
        client: s3.NewFromConfig(cfg),
        bucket: bucket,
        logger: sugar,
        signer: signer,
        creds:  creds,
    }, nil
}

func (s *s3Service) UploadFile(ctx context.Context, key string, content []byte, traceID string) (string, error) {
    span, ctx := tracing.StartSpanFromContext(ctx, "S3Upload", traceID)
    defer span.Finish()

    start := time.Now()
    defer func() {
        metrics.S3UploadDuration.Observe(time.Since(start).Seconds())
    }()

    // Creating the PUT request
    httpReq, _ := http.NewRequest("PUT", fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.bucket, key), bytes.NewReader(content))

    // Sign the request using SigV4
    err := s.signer.SignHTTP(ctx, s.creds, httpReq, "s3", s.bucket, "us-east-1", time.Now())
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "s3_sign_error", "error.message", err.Error())
        s.logger.Errorw("Failed to sign S3 request", "error", err)
        return "", fmt.Errorf("failed to sign S3 request: %w", err)
    }

    resp, err := http.DefaultClient.Do(httpReq)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "s3_upload_request_error", "error.message", err.Error())
        s.logger.Errorw("Failed to execute signed S3 request", "error", err)
        return "", fmt.Errorf("failed to execute signed S3 request: %w", err)
    }
    defer resp.Body.Close()

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

    result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
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

    taggingResult, err := s.client.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{
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
