// internal/types/config.go
package types

type ConfigProvider interface {
    GetAWSRegion() string
    GetS3Bucket() string
}
