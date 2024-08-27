// internal/types/config.go
package types

type ConfigProvider interface {
    GetAWSRegion() string
    GetS3Bucket() string
    GetNeo4jURI() string
    GetNeo4jUsername() string
    GetNeo4jPassword() string
    GetRedshiftConnString() string
}

