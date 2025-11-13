package main

import (
	"context"
	"log"
	"strings"
)

// ArbitrageCalculator 负责对套利机会进行精细化计算
type ArbitrageCalculator struct {
	queue *ArbitrageQueue
	cfg   *AppConfig
}

// NewArbitrageCalculator 创建套利路径计算者
func NewArbitrageCalculator(queue *ArbitrageQueue, cfg *AppConfig) *ArbitrageCalculator {
	return &ArbitrageCalculator{
		queue: queue,
		cfg:   cfg,
	}
}

// Start 开始处理套利机会
func (ac *ArbitrageCalculator) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case opportunity := <-ac.queue.Subscribe():
			ac.handleOpportunity(ctx, opportunity)
		}
	}
}

func (ac *ArbitrageCalculator) handleOpportunity(ctx context.Context, opportunity ArbitrageOpportunity) {
	log.Printf("套利机会入队 (跳数 %d): 初始 %.6f USDT, 估算 %.6f USDT, 路径: %s",
		len(opportunity.Path), opportunity.InitialAmount, opportunity.EstimatedReturn, formatOpportunityPath(opportunity))
	detailReturn, profitable := ac.calculateDetailedProfit(ctx, opportunity)
	if !profitable {
		log.Printf("套利机会经精算后无效 (跳数 %d): 初始 %.6f USDT, 估算 %.6f USDT, 路径: %s",
			len(opportunity.Path), opportunity.InitialAmount, detailReturn, formatOpportunityPath(opportunity))
		return
	}

	log.Printf("确认套利机会: 起始代币 %s, 跳数 %d, 初始 %.6f USDT -> 预期 %.6f USDT, 利润 %.6f, 路径: %s",
		opportunity.StartToken, len(opportunity.Path), opportunity.InitialAmount, detailReturn,
		detailReturn-opportunity.InitialAmount, formatOpportunityPath(opportunity))
	ac.submitExecution(ctx, opportunity, detailReturn)
}

func (ac *ArbitrageCalculator) calculateDetailedProfit(ctx context.Context, opportunity ArbitrageOpportunity) (float64, bool) {
	// TODO: 在此处实现链下详细计算逻辑，例如结合实时储备、滑点模型等
	return opportunity.EstimatedReturn, opportunity.EstimatedReturn-opportunity.InitialAmount >= ac.cfg.ArbMinProfit
}

func (ac *ArbitrageCalculator) submitExecution(ctx context.Context, opportunity ArbitrageOpportunity, expectedReturn float64) {
	// TODO: 实现交易下单逻辑，例如构建多跳交易并提交到区块链
	log.Printf("提交套利执行（占位）: 起始 %s, 预期收益 %.6f, 路径长度 %d",
		opportunity.StartToken, expectedReturn, len(opportunity.Path))
}

func formatOpportunityPath(opportunity ArbitrageOpportunity) string {
	if len(opportunity.Path) == 0 {
		return ""
	}
	var builder strings.Builder
	for idx, step := range opportunity.Path {
		if idx > 0 {
			builder.WriteString(" => ")
		}
		builder.WriteString(step.Protocol)
		builder.WriteString("[")
		builder.WriteString(step.Pool.Address.Hex())
		builder.WriteString("] ")
		builder.WriteString(step.FromToken)
		builder.WriteString(" -> ")
		builder.WriteString(step.ToToken)
		builder.WriteString(" (token0=")
		builder.WriteString(step.Pool.Token0.Hex())
		builder.WriteString(", token1=")
		builder.WriteString(step.Pool.Token1.Hex())
		builder.WriteString(")")
	}
	return builder.String()
}
