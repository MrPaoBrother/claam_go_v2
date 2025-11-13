package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	_ "modernc.org/sqlite"
)

// PoolStore 负责池子信息的持久化
type PoolStore struct {
	db *sql.DB
	mu sync.Mutex
}

// NewPoolStore 创建池子存储，path 为空时默认使用 pools.db
func NewPoolStore(path string) (*PoolStore, error) {
	if path == "" {
		path = defaultSQLitePath
	}

	dsn := path
	if !strings.HasPrefix(path, "file:") {
		// 设置 busy_timeout 和 WAL，提高并发写入能力
		dsn = fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 使用单连接模式，避免驱动在内部创建多个连接导致锁冲突
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &PoolStore{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (ps *PoolStore) init() error {
	const createTable = `
CREATE TABLE IF NOT EXISTS pools (
	id TEXT PRIMARY KEY,
	protocol TEXT NOT NULL,
	token0 TEXT NOT NULL,
	token1 TEXT NOT NULL,
	fee REAL NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

	ps.mu.Lock()
	defer ps.mu.Unlock()

	_, err := ps.db.Exec(createTable)
	return err
}

// InsertPoolIfNotExists 如果池子不存在则插入
func (ps *PoolStore) InsertPoolIfNotExists(pool poolDetail) error {
	const insertStmt = `
INSERT INTO pools (id, protocol, token0, token1, fee, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO NOTHING;
`

	ps.mu.Lock()
	defer ps.mu.Unlock()

	_, err := ps.db.Exec(insertStmt, pool.Address.Hex(), pool.Protocol, pool.Token0.Hex(), pool.Token1.Hex(), pool.Fee)
	return err
}

// ListPools 返回数据库中所有池子信息
func (ps *PoolStore) ListPools(ctx context.Context) ([]poolDetail, error) {
	const selectStmt = `
SELECT id, protocol, token0, token1, fee
FROM pools;
`

	ps.mu.Lock()
	defer ps.mu.Unlock()

	rows, err := ps.db.QueryContext(ctx, selectStmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pools []poolDetail
	for rows.Next() {
		var (
			id       string
			protocol string
			token0   string
			token1   string
			fee      float64
		)
		if err := rows.Scan(&id, &protocol, &token0, &token1, &fee); err != nil {
			return nil, err
		}
		pools = append(pools, poolDetail{
			Address:  common.HexToAddress(id),
			Token0:   common.HexToAddress(token0),
			Token1:   common.HexToAddress(token1),
			Fee:      fee,
			Protocol: protocol,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pools, nil
}

// Close 关闭数据库
func (ps *PoolStore) Close() error {
	if ps.db != nil {
		return ps.db.Close()
	}
	return nil
}
