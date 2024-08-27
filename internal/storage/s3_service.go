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

func NewS3Service(region, bucket string,
