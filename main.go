package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wsURL := DefaultBSCWssURL

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	blockQueue, err := NewBlockQueue(cfg.BlockQueueSize)
	if err != nil {
		log.Fatalf("创建区块队列失败: %v", err)
	}

	v1ABI, err := abi.JSON(strings.NewReader(UniswapV1ExchangeABIJSON))
	if err != nil {
		log.Fatalf("解析 V1 ABI 失败: %v", err)
	}
	v2ABI, err := abi.JSON(strings.NewReader(PairABIJSON))
	if err != nil {
		log.Fatalf("解析 V2 ABI 失败: %v", err)
	}
	v3ABI, err := abi.JSON(strings.NewReader(UniswapV3ABIJSON))
	if err != nil {
		log.Fatalf("解析 V3 ABI 失败: %v", err)
	}

	protocols := GetProtocolsConfig(&v1ABI, &v2ABI, &v3ABI)

	conn, err := ethclient.DialContext(ctx, wsURL)
	if err != nil {
		log.Fatalf("连接 BSC 节点失败: %v", err)
	}
	defer conn.Close()

	store, err := NewPoolStore(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("初始化 SQLite 失败: %v", err)
	}
	defer store.Close()

	arbQueue := NewArbitrageQueue(cfg.ArbQueueSize)

	// 1. 订阅区块
	subscriber := NewBlockSubscriber(wsURL, conn, blockQueue)
	go func() {
		if err := subscriber.Start(ctx); err != nil {
			log.Printf("订阅器结束: %v", err)
		}
	}()

	// // 定时打印队列积压量
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Printf("当前区块队列积压: %d", blockQueue.Len())
			}
		}
	}()

	// // 2. 发现池子
	discoverer := NewPoolDiscoverer(blockQueue, conn, store, protocols)
	go discoverer.Start(ctx)

	// 3. 发现套利机会
	finder := NewArbitrageFinder(store, arbQueue, cfg)
	go finder.Start(ctx)

	// 4. 计算套利机会
	calculator := NewArbitrageCalculator(arbQueue, cfg)
	go calculator.Start(ctx)

	router := gin.Default()
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})
	if err := router.Run(); err != nil {
		log.Fatalf("启动 HTTP 服务器失败: %v", err)
	}
}
