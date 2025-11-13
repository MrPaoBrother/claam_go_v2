package main

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// BlockEvent 表示一个待处理的区块事件
type BlockEvent struct {
	Number *big.Int
	Hash   common.Hash
}

// BlockQueue 内存队列，用于缓存待处理的区块
type BlockQueue struct {
	ch chan BlockEvent
}

// NewBlockQueue 创建新的区块队列
func NewBlockQueue(size int) (*BlockQueue, error) {
	if size <= 0 {
		return nil, fmt.Errorf("block queue size must be positive, current: %d", size)
	}
	return &BlockQueue{
		ch: make(chan BlockEvent, size),
	}, nil
}

// Publish 将新区块事件放入队列，如果队列已满会丢弃最旧的一个
func (q *BlockQueue) Publish(event BlockEvent) {
	select {
	case q.ch <- event:
	default:
		// 队列已满，丢弃最旧的一个
		select {
		case <-q.ch:
		default:
		}
		q.ch <- event
	}
}

// Subscribe 返回一个只读 channel，用于消费区块事件
func (q *BlockQueue) Subscribe() <-chan BlockEvent {
	return q.ch
}

// Len 返回当前队列积压的区块数量
func (q *BlockQueue) Len() int {
	return len(q.ch)
}
