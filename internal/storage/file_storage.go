package storage

import (
    "encoding/json"
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"
    "sync"

    "github.com/aanthord/pubsub-amqp/internal/models"
    "go.uber.org/zap"
)

const (
    NumShards = 256
    ShardMask = NumShards - 1
)

type FileStorage struct {
    basePath string
    cache    map[string]*models.MessagePayload
    mu       sync.RWMutex
    logger   *zap.SugaredLogger
}

func NewFileStorage(basePath string, logger *zap.SugaredLogger) (*FileStorage, error) {
    if err := os.MkdirAll(basePath, 0755); err != nil {
        return nil, fmt.Errorf("failed to create base directory: %w", err)
    }

    return &FileStorage{
        basePath: basePath,
        cache:    make(map[string]*models.MessagePayload),
        logger:   logger,
    }, nil
}

func (fs *FileStorage) Store(message *models.MessagePayload) error {
    fs.mu.Lock()
    defer fs.mu.Unlock()

    shard := fs.getShard(message.Timestamp)
    fileName := filepath.Join(fs.basePath, fmt.Sprintf("shard_%d.json", shard))

    fs.cache[message.ID] = message

    file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("failed to open file: %w", err)
    }
    defer file.Close()

    data, err := json.Marshal(message)
    if err != nil {
        return fmt.Errorf("failed to marshal message: %w", err)
    }

    if _, err := file.Write(append(data, '\n')); err != nil {
        return fmt.Errorf("failed to write to file: %w", err)
    }

    fs.logger.Infow("Message stored", "shard", shard, "id", message.ID)
    return nil
}

func (fs *FileStorage) Retrieve(id string) (*models.MessagePayload, error) {
    fs.mu.RLock()
    defer fs.mu.RUnlock()

    if message, ok := fs.cache[id]; ok {
        return message, nil
    }

    for shard := 0; shard < NumShards; shard++ {
        fileName := filepath.Join(fs.basePath, fmt.Sprintf("shard_%d.json", shard))
        data, err := ioutil.ReadFile(fileName)
        if err != nil {
            if os.IsNotExist(err) {
                continue
            }
            return nil, fmt.Errorf("failed to read file: %w", err)
        }

        var messages []*models.MessagePayload
        if err := json.Unmarshal(data, &messages); err != nil {
            return nil, fmt.Errorf("failed to unmarshal messages: %w", err)
        }

        for _, message := range messages {
            if message.ID == id {
                fs.cache[id] = message
                return message, nil
            }
        }
    }

    return nil, fmt.Errorf("message not found")
}

func (fs *FileStorage) getShard(timestamp string) int {
    return int(timestamp[0]) & ShardMask
}
