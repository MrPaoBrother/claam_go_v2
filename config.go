package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// defaultBlockQueueSize 区块队列默认容量，防止 backlog 无限增长
	defaultBlockQueueSize = 1000
	// defaultSQLitePath 默认的 SQLite 库文件名称
	defaultSQLitePath = "pools.db"
	// defaultArbReloadSeconds 套利发现者默认的刷新周期（秒）
	defaultArbReloadSeconds = 60
	// defaultArbMaxHops 默认的套利路径最大跳数
	defaultArbMaxHops = 5
	// defaultArbInitialCapital 默认的套利模拟起始资金（单位：USD）
	defaultArbInitialCapital = 1.0
	// defaultArbMinProfit 默认的套利最小收益门槛（单位：USD）
	defaultArbMinProfit = 0.0
	// defaultArbQueueSize 套利机会队列默认容量
	defaultArbQueueSize = 256
)

// AppConfig 应用配置
type AppConfig struct {
	// BlockQueueSize 区块内存队列容量
	BlockQueueSize int
	// SQLitePath 池子信息持久化所使用的 SQLite 文件路径
	SQLitePath string
	// ArbReloadInterval 套利发现者刷新池子图的时间间隔
	ArbReloadInterval time.Duration
	// ArbMaxHops 套利路径允许的最大跳数
	ArbMaxHops int
	// ArbInitialCapital 套利模拟的初始资金（单位：USD）
	ArbInitialCapital float64
	// ArbMinProfit 套利机会的最小收益阈值（单位：USD）
	ArbMinProfit float64
	// ArbQueueSize 套利机会队列容量
	ArbQueueSize int
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

	reloadInterval := time.Duration(defaultArbReloadSeconds) * time.Second
	if intervalStr := strings.TrimSpace(os.Getenv("ARB_RELOAD_INTERVAL")); intervalStr != "" {
		duration, err := time.ParseDuration(intervalStr)
		if err != nil || duration <= 0 {
			return nil, fmt.Errorf("ARB_RELOAD_INTERVAL 非法值: %s", intervalStr)
		}
		reloadInterval = duration
	}

	maxHops := defaultArbMaxHops
	if hopsStr := strings.TrimSpace(os.Getenv("ARB_MAX_HOPS")); hopsStr != "" {
		parsed, err := strconv.Atoi(hopsStr)
		if err != nil || parsed < 2 {
			return nil, fmt.Errorf("ARB_MAX_HOPS 非法值: %s", hopsStr)
		}
		maxHops = parsed
	}

	initialCapital := defaultArbInitialCapital
	if capitalStr := strings.TrimSpace(os.Getenv("ARB_INITIAL_CAPITAL")); capitalStr != "" {
		value, err := strconv.ParseFloat(capitalStr, 64)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("ARB_INITIAL_CAPITAL 非法值: %s", capitalStr)
		}
		initialCapital = value
	}

	minProfit := defaultArbMinProfit
	if profitStr := strings.TrimSpace(os.Getenv("ARB_MIN_PROFIT")); profitStr != "" {
		value, err := strconv.ParseFloat(profitStr, 64)
		if err != nil || value < 0 {
			return nil, fmt.Errorf("ARB_MIN_PROFIT 非法值: %s", profitStr)
		}
		minProfit = value
	}

	arbQueueSize := defaultArbQueueSize
	if queueStr := strings.TrimSpace(os.Getenv("ARB_QUEUE_SIZE")); queueStr != "" {
		parsed, err := strconv.Atoi(queueStr)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("ARB_QUEUE_SIZE 非法值: %s", queueStr)
		}
		arbQueueSize = parsed
	}

	return &AppConfig{
		BlockQueueSize:    queueSize,
		SQLitePath:        sqlitePath,
		ArbReloadInterval: reloadInterval,
		ArbMaxHops:        maxHops,
		ArbInitialCapital: initialCapital,
		ArbMinProfit:      minProfit,
		ArbQueueSize:      arbQueueSize,
	}, nil
}
