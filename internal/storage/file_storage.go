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
    DefaultNumShards = 256
    ShardMask = DefaultNumShards - 1
    MaxBatchSize = 100 // Max number of messages per batch during migration
)

type FileStorage struct {
    basePath   string
    mu         sync.RWMutex
    logger     *zap.SugaredLogger
    shardDepth int
}

func NewFileStorage(basePath string, logger *zap.SugaredLogger, shardDepth int) (*FileStorage, error) {
    if err := os.MkdirAll(basePath, 0755); err != nil {
        return nil, fmt.Errorf("failed to create base directory: %w", err)
    }

    return &FileStorage{
        basePath:   basePath,
        logger:     logger,
        shardDepth: shardDepth,
    }, nil
}

func (fs *FileStorage) Store(ctx context.Context, message *models.MessagePayload) error {
    span, _ := tracing.StartSpanFromContext(ctx, "FileStore", message.Header.TraceID)
    defer span.Finish()

    fs.mu.Lock()
    defer fs.mu.Unlock()

    shard := fs.getShard(message.Header.ID)
    fileName := filepath.Join(fs.basePath, fmt.Sprintf("shard_%s.json", shard))

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

    span.LogKV("event", "message_stored", "shard", shard, "message_id", message.Header.ID)
    return nil
}

func (fs *FileStorage) Retrieve(ctx context.Context, traceID string) ([]*models.MessagePayload, error) {
    span, _ := tracing.StartSpanFromContext(ctx, "FileRetrieve", traceID)
    defer span.Finish()

    fs.mu.RLock()
    defer fs.mu.RUnlock()

    var messages []*models.MessagePayload

    for shard := 0; shard < DefaultNumShards; shard++ {
        fileName := filepath.Join(fs.basePath, fmt.Sprintf("shard_%02x.json", shard))
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
            if message.Header.TraceID == traceID {
                messages = append(messages, &message)
            }
        }
    }

    span.LogKV("event", "messages_retrieved", "count", len(messages))
    return messages, nil
}

func (fs *FileStorage) getShard(uuid string) string {
    if len(uuid) < fs.shardDepth {
        return uuid // Fallback if UUID is shorter than shard depth
    }
    return uuid[:fs.shardDepth]
}

func (fs *FileStorage) MigrateShards(ctx context.Context, oldShardDepth, newShardDepth int) error {
    if oldShardDepth == newShardDepth {
        fs.logger.Info("No migration needed, shard depth is unchanged.")
        return nil
    }

    fs.logger.Infow("Starting shard migration", "oldShardDepth", oldShardDepth, "newShardDepth", newShardDepth)

    fs.mu.Lock()
    defer fs.mu.Unlock()

    for shard := 0; shard < (1 << oldShardDepth); shard++ {
        oldFileName := filepath.Join(fs.basePath, fmt.Sprintf("shard_%d.json", shard))
        data, err := ioutil.ReadFile(oldFileName)
        if err != nil {
            if os.IsNotExist(err) {
                continue
            }
            return fmt.Errorf("failed to read file during migration: %w", err)
        }

        // Now process the data and move it to the new shard structure
        lines := bytes.Split(data, []byte("\n"))
        for _, line := range lines {
            if len(line) == 0 {
                continue
            }
            var message models.MessagePayload
            if err := json.Unmarshal(line, &message); err != nil {
                return fmt.Errorf("failed to unmarshal during migration: %w", err)
            }

            newShard := fs.getShard(message.Header.ID[:newShardDepth]) // Use Header instead of Headers
            newFileName := filepath.Join(fs.basePath, fmt.Sprintf("shard_%d.json", newShard))
            file, err := os.OpenFile(newFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
            if err != nil {
                return fmt.Errorf("failed to open new shard file during migration: %w", err)
            }

            if _, err := file.Write(append(line, '\n')); err != nil {
                return fmt.Errorf("failed to write to new shard file during migration: %w", err)
            }

            file.Close()
        }

        // Optionally, remove old shard file after successful migration
        if err := os.Remove(oldFileName); err != nil {
            fs.logger.Warnw("Failed to remove old shard file", "file", oldFileName, "error", err)
        }
    }

    fs.logger.Info("Shard migration completed successfully.")
    return nil
}


func (fs *FileStorage) writeBatch(fileName string, batch [][]byte) error {
    file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("failed to open new shard file: %w", err)
    }
    defer file.Close()

    for _, line := range batch {
        if _, err := file.Write(append(line, '\n')); err != nil {
            return fmt.Errorf("failed to write to new shard file: %w", err)
        }
    }

    return nil
}
