package main

import "sync"

// ArbitrageOpportunity 表示潜在的套利路径
type ArbitrageOpportunity struct {
	Path            []ArbitrageStep
	StartToken      string
	InitialAmount   float64
	EstimatedReturn float64
}

// ArbitrageStep 表示套利路径中的一步
type ArbitrageStep struct {
	Pool      poolDetail
	FromToken string
	ToToken   string
	Protocol  string
	Fee       float64
}

// ArbitrageQueue 用于缓存套利机会
type ArbitrageQueue struct {
	ch chan ArbitrageOpportunity
	mu sync.RWMutex
}

// NewArbitrageQueue 创建新的套利队列
func NewArbitrageQueue(size int) *ArbitrageQueue {
	return &ArbitrageQueue{
		ch: make(chan ArbitrageOpportunity, size),
	}
}

// Publish 推送新的套利机会，如果队列已满则丢弃最旧的数据
func (q *ArbitrageQueue) Publish(op ArbitrageOpportunity) {
	q.mu.Lock()
	defer q.mu.Unlock()

	select {
	case q.ch <- op:
	default:
		select {
		case <-q.ch:
		default:
		}
		q.ch <- op
	}
}

// Subscribe 返回队列的只读 channel
func (q *ArbitrageQueue) Subscribe() <-chan ArbitrageOpportunity {
	return q.ch
}
