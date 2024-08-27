package storage

import (
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"
    "sync"
    "bytes"

    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "go.uber.org/zap"
)

const (
    NumShards = 256
    ShardMask = NumShards - 1
)

type FileStorage struct {
    basePath string
    mu       sync.RWMutex
    logger   *zap.SugaredLogger
}

func NewFileStorage(basePath string, logger *zap.SugaredLogger) (*FileStorage, error) {
    if err := os.MkdirAll(basePath, 0755); err != nil {
        return nil, fmt.Errorf("failed to create base directory: %w", err)
    }

    return &FileStorage{
        basePath: basePath,
        logger:   logger,
    }, nil
}

func (fs *FileStorage) Store(ctx context.Context, message *models.MessagePayload) error {
    span, _ := tracing.StartSpanFromContext(ctx, "FileStore", message.TraceID)
    defer span.Finish()

    fs.mu.Lock()
    defer fs.mu.Unlock()

    shard := fs.getShard(message.Timestamp)
    fileName := filepath.Join(fs.basePath, fmt.Sprintf("shard_%d.json", shard))

    file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "file_open_error", "error.message", err.Error())
        return fmt.Errorf("failed to open file: %w", err)
    }
    defer file.Close()

    data, err := json.Marshal(message)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "marshal_error", "error.message", err.Error())
        return fmt.Errorf("failed to marshal message: %w", err)
    }

    if _, err := file.Write(append(data, '\n')); err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "write_error", "error.message", err.Error())
        return fmt.Errorf("failed to write to file: %w", err)
    }

    span.LogKV("event", "message_stored", "shard", shard, "message_id", message.ID)
    return nil
}

func (fs *FileStorage) Retrieve(ctx context.Context, traceID string) ([]*models.MessagePayload, error) {
    span, _ := tracing.StartSpanFromContext(ctx, "FileRetrieve", traceID)
    defer span.Finish()

    fs.mu.RLock()
    defer fs.mu.RUnlock()

    var messages []*models.MessagePayload

    for shard := 0; shard < NumShards; shard++ {
        fileName := filepath.Join(fs.basePath, fmt.Sprintf("shard_%d.json", shard))
        data, err := ioutil.ReadFile(fileName)
        if err != nil {
            if os.IsNotExist(err) {
                continue
            }
            span.SetTag("error", true)
            span.LogKV("event", "file_read_error", "error.message", err.Error())
            return nil, fmt.Errorf("failed to read file: %w", err)
        }

        lines := bytes.Split(data, []byte("\n"))
        for _, line := range lines {
            if len(line) == 0 {
                continue
            }
            var message models.MessagePayload
            if err := json.Unmarshal(line, &message); err != nil {
                span.SetTag("error", true)
                span.LogKV("event", "unmarshal_error", "error.message", err.Error())
                continue
            }
            if message.TraceID == traceID {
                messages = append(messages, &message)
            }
        }
    }

    span.LogKV("event", "messages_retrieved", "count", len(messages))
    return messages, nil
}

func (fs *FileStorage) getShard(timestamp string) int {
    return int(timestamp[0]) & ShardMask
}
