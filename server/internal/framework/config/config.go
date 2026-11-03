package config

import "context"

// Config 是进程启动所需的配置快照。
type Config struct {
	HTTP        HTTPConfig
	Postgres    PostgresConfig
	Redis       RedisConfig
	ObjectStore ObjectStoreConfig
}

type HTTPConfig struct {
	Address string
}

type PostgresConfig struct {
	DSN string
}

type RedisConfig struct {
	Address string
}

type ObjectStoreConfig struct {
	Endpoint string
	Bucket   string
}

// Loader 读取并校验配置来源。
type Loader interface {
	Load(ctx context.Context) (Config, error)
}
