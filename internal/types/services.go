package types

type S3Service interface {
    UploadFile(key string, data []byte) error
    DownloadFile(key string) ([]byte, error)
}

type RedshiftService interface {
    ExecuteQuery(query string) error
}

type Neo4jService interface {
    ExecuteCypher(query string, params map[string]interface{}) error
}

type AMQPService interface {
    PublishMessage(exchange, routingKey string, message []byte) error
    Subscribe(queueName string, handler func([]byte)) error
}

type UUIDService interface {
    GenerateUUID() string
}

type SearchService interface {
    Search(query string) ([]string, error)
}
