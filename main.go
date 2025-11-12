package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BSC 公共 WebSocket 节点地址（默认值）
const defaultBSCWssURL = "wss://bsc.drpc.org"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 获取 WebSocket URL，默认使用 BSC 公共节点
	wsURL := defaultBSCWssURL

	// 创建池子监控器
	monitor, err := NewPoolMonitor(wsURL)
	if err != nil {
		log.Fatalf("创建池子监控器失败: %v", err)
	}
	defer monitor.Close()

	// 在协程中启动区块订阅和池子发现
	go func() {
		if err := monitor.Process(ctx); err != nil {
			log.Printf("池子监控器退出: %v", err)
		}
	}()

	// TODO 后续每个步骤的任务都是一个独立协程
	// 计算套利路径

	// 启动 Gin HTTP 服务器
	router := gin.Default()
	// TODO 后续提供api辅助
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})
	if err := router.Run(); err != nil {
		log.Fatalf("启动 HTTP 服务器失败: %v", err)
	}
}
