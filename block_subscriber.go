package main

import (
	"context"
	"errors"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// BlockSubscriber 订阅新区块并推送到内存队列
type BlockSubscriber struct {
	wsURL  string
	client *ethclient.Client
	queue  *BlockQueue
}

// NewBlockSubscriber 创建区块订阅器
func NewBlockSubscriber(wsURL string, client *ethclient.Client, queue *BlockQueue) *BlockSubscriber {
	return &BlockSubscriber{
		wsURL:  wsURL,
		client: client,
		queue:  queue,
	}
}

// Start 启动订阅流程
func (bs *BlockSubscriber) Start(ctx context.Context) error {
	headers := make(chan *types.Header, 16)

	for {
		sub, err := bs.client.SubscribeNewHead(ctx, headers)
		if err != nil {
			log.Printf("订阅区块失败: %v，5秒后重试", err)
			select {
			case <-time.After(5 * time.Second):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if err := bs.loop(ctx, headers, sub); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("监听循环错误: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Println("尝试重新订阅区块头")
	}
}

type subscription interface {
	Err() <-chan error
	Unsubscribe()
}

func (bs *BlockSubscriber) loop(ctx context.Context, headers chan *types.Header, sub subscription) error {
	defer sub.Unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-sub.Err():
			return err
		case header := <-headers:
			if header == nil {
				continue
			}
			bs.handleHeader(header)
		}
	}
}

func (bs *BlockSubscriber) handleHeader(header *types.Header) {
	if header == nil {
		return
	}

	number := header.Number
	if number == nil {
		return
	}

	event := BlockEvent{
		Number: new(big.Int).Set(number),
		Hash:   header.Hash(),
	}
	bs.queue.Publish(event)

	log.Printf("收到新区块: 高度 %s 哈希 %s", number.String(), event.Hash.Hex())
}
