package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultBlockQueueSize = 1000
	defaultSQLitePath     = "pools.db"
)

// AppConfig 应用配置
type AppConfig struct {
	BlockQueueSize int
	SQLitePath     string
}

// LoadConfig 从环境变量加载配置
func LoadConfig() (*AppConfig, error) {
	queueSize := defaultBlockQueueSize
	if queueSizeEnv := strings.TrimSpace(os.Getenv("BLOCK_QUEUE_SIZE")); queueSizeEnv != "" {
		parsed, err := strconv.Atoi(queueSizeEnv)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("BLOCK_QUEUE_SIZE 非法值: %s", queueSizeEnv)
		}
		queueSize = parsed
	}

	sqlitePath := strings.TrimSpace(os.Getenv("SQLITE_PATH"))
	if sqlitePath == "" {
		sqlitePath = defaultSQLitePath
	}

	return &AppConfig{
		BlockQueueSize: queueSize,
		SQLitePath:     sqlitePath,
	}, nil
}
