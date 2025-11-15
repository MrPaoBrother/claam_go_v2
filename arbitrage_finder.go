package main

import (
	"context"
	"log"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type graphEdge struct {
	Pool      poolDetail
	Protocol  string
	Fee       float64
	Rate      float64
	FromToken common.Address
	ToToken   common.Address
	FromIndex int
	ToIndex   int
}

// ArbitrageFinder 负责发现潜在套利路径
type ArbitrageFinder struct {
	store     *PoolStore
	queue     *ArbitrageQueue
	cfg       *AppConfig
	mu        sync.RWMutex
	seenPaths map[string]struct{}
}

// NewArbitrageFinder 创建套利路径发现者
func NewArbitrageFinder(store *PoolStore, queue *ArbitrageQueue, cfg *AppConfig) *ArbitrageFinder {
	return &ArbitrageFinder{
		store:     store,
		queue:     queue,
		cfg:       cfg,
		seenPaths: make(map[string]struct{}),
	}
}

// Start 启动套利路径发现流程
func (af *ArbitrageFinder) Start(ctx context.Context) {
	ticker := time.NewTicker(af.cfg.ArbReloadInterval)
	defer ticker.Stop()

	af.runDiscovery(ctx) // 启动时先执行一次

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			af.runDiscovery(ctx)
		}
	}
}

func (af *ArbitrageFinder) runDiscovery(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pools, err := af.store.ListPools(ctx)
	if err != nil {
		log.Printf("加载池子数据失败: %v", err)
		return
	}
	log.Printf("套利发现者加载到 %d 个池子", len(pools))

	af.buildGraph(pools)
	af.enumerateCycles()
}

func (af *ArbitrageFinder) buildGraph(pools []poolDetail) {
	af.mu.Lock()
	defer af.mu.Unlock()
	af.seenPaths = make(map[string]struct{})
	// 新的算法不需要构建索引图，直接使用 pools
}

func (af *ArbitrageFinder) enumerateCycles() {
	pools, err := af.store.ListPools(context.Background())
	if err != nil {
		log.Printf("加载池子数据失败: %v", err)
		return
	}

	maxHops := af.cfg.ArbMaxHops
	if maxHops < 2 {
		maxHops = 2
	}
	initialAmount := af.cfg.ArbInitialCapital
	minProfit := af.cfg.ArbMinProfit

	// 收集所有唯一的 token 地址作为起点
	tokenSet := make(map[common.Address]struct{})
	for _, p := range pools {
		tokenSet[p.Token0] = struct{}{}
		tokenSet[p.Token1] = struct{}{}
	}

	// 统计信息
	totalPaths := 0
	profitablePaths := 0

	// 对每个 token 作为起点，查找套利路径
	for startToken := range tokenSet {
		var circles []arbitrageCircle
		af.findArb(pools, startToken, startToken, maxHops, nil, []common.Address{startToken}, &circles)
		totalPaths += len(circles)
		for _, circle := range circles {
			if af.handleCircle(circle, initialAmount, minProfit) {
				profitablePaths++
			}
		}
	}

	log.Printf("套利路径统计: 总路径数 %d, 初步盈利路径数 %d", totalPaths, profitablePaths)
}

// arbitrageCircle 表示一个套利环
type arbitrageCircle struct {
	Route []poolDetail     // 路径中的池子列表
	Path  []common.Address // 路径中的代币列表
}

// findArb 递归查找套利路径（参考 Python 代码逻辑）
func (af *ArbitrageFinder) findArb(pairs []poolDetail, tokenIn, tokenOut common.Address, maxHops int,
	currentPairs []poolDetail, path []common.Address, circles *[]arbitrageCircle) {

	for i := range pairs {
		pair := pairs[i]

		// 检查 pair 是否包含 tokenIn
		if pair.Token0 != tokenIn && pair.Token1 != tokenIn {
			continue
		}

		// 检查储备量是否足够（假设 decimal 为 18，储备量需要 >= 1e18）
		// 简化处理：直接比较 big.Int，如果储备量太小则跳过
		minReserve := big.NewInt(1e18) // 1 * 10^18
		if pair.Reserve0 == nil || pair.Reserve0.Cmp(minReserve) < 0 {
			continue
		}
		if pair.Reserve1 == nil || pair.Reserve1.Cmp(minReserve) < 0 {
			continue
		}

		// 确定输出代币
		var tempOut common.Address
		if tokenIn == pair.Token0 {
			tempOut = pair.Token1
		} else {
			tempOut = pair.Token0
		}

		newPath := make([]common.Address, len(path))
		copy(newPath, path)
		newPath = append(newPath, tempOut)

		newPairs := make([]poolDetail, len(currentPairs))
		copy(newPairs, currentPairs)
		newPairs = append(newPairs, pair)

		// 如果找到闭环且路径长度 > 2
		if tempOut == tokenOut && len(path) > 2 {
			*circles = append(*circles, arbitrageCircle{
				Route: newPairs,
				Path:  newPath,
			})
		} else if maxHops > 1 && len(pairs) > 1 {
			// 排除当前 pair，递归查找
			pairsExcludingThis := make([]poolDetail, 0, len(pairs)-1)
			pairsExcludingThis = append(pairsExcludingThis, pairs[:i]...)
			pairsExcludingThis = append(pairsExcludingThis, pairs[i+1:]...)
			af.findArb(pairsExcludingThis, tempOut, tokenOut, maxHops-1, newPairs, newPath, circles)
		}
	}
}

// handleCircle 处理一个套利环，返回是否盈利
func (af *ArbitrageFinder) handleCircle(circle arbitrageCircle, initialAmount, minProfit float64) bool {
	if len(circle.Route) < 2 {
		return false
	}

	// 将 circle 转换为 graphEdge 路径
	path := make([]graphEdge, 0, len(circle.Route))
	for i := 0; i < len(circle.Path)-1; i++ {
		pair := circle.Route[i]
		fromToken := circle.Path[i]
		toToken := circle.Path[i+1]

		rate := 1 - pair.Fee/100.0
		if rate <= 0 {
			rate = 1e-6
		}

		path = append(path, graphEdge{
			Pool:      pair,
			Protocol:  pair.Protocol,
			Fee:       pair.Fee,
			Rate:      rate,
			FromToken: fromToken,
			ToToken:   toToken,
		})
	}

	pathKey := hashPath(path)
	if af.isPathSeen(pathKey) {
		return false
	}

	// pathDesc := formatPath(path)
	// log.Printf("检测到套利环 (跳数 %d): %s", len(path), pathDesc)

	estimated, profitable := af.simulatePath(initialAmount, path, minProfit)
	if !profitable {
		return false
	}

	// log.Printf("初步可盈利套利 (跳数 %d): 初始 %.6f USDT -> 预计 %.6f USDT, 利润 %.6f, 路径: %s",
	// 	len(path), initialAmount, estimated, estimated-initialAmount, pathDesc)

	af.markPath(pathKey)
	startToken := path[0].FromToken
	af.queue.Publish(convertToOpportunity(path, startToken, initialAmount, estimated))
	return true
}

// simulatePath 模拟套利路径，使用实际的 AMM 公式计算
// 参数 initial 是初始投入的 token0 数量（假设为 1 USDT 等值）
// 参数 path 是套利路径，每一步都是一个交易对
// 参数 minProfit 是最小利润要求
// 返回最终得到的 token0 数量和是否盈利
func (af *ArbitrageFinder) simulatePath(initial float64, path []graphEdge, minProfit float64) (float64, bool) {
	if len(path) == 0 {
		return 0, false
	}

	// 假设初始投入 1 USDT 等值的 token0
	amount := initial

	// 遍历路径中的每一步，使用实际的 AMM 公式计算
	for _, step := range path {
		pool := step.Pool

		// 检查储备量是否有效
		if pool.Reserve0 == nil || pool.Reserve1 == nil {
			return 0, false
		}

		reserve0 := new(big.Float).SetInt(pool.Reserve0)
		reserve1 := new(big.Float).SetInt(pool.Reserve1)

		// 检查储备量是否足够
		if reserve0.Cmp(big.NewFloat(0)) <= 0 || reserve1.Cmp(big.NewFloat(0)) <= 0 {
			return 0, false
		}

		amountFloat := big.NewFloat(amount)

		// 根据协议类型选择不同的计算公式
		if pool.Protocol == ProtocolUniswapV2Like {
			// V2 使用恒定乘积公式: x * y = k
			// Uniswap V2 标准公式: amountOut = (amountIn * reserveOut * 997) / ((reserveIn * 1000) + (amountIn * 997))
			// 其中 997/1000 表示扣除 0.3% 手续费
			var reserveIn, reserveOut *big.Float
			if step.FromToken == pool.Token0 {
				reserveIn = reserve0
				reserveOut = reserve1
			} else {
				reserveIn = reserve1
				reserveOut = reserve0
			}

			// 计算手续费率（例如 0.3% 手续费 = 997/1000）
			feeRatio := step.Fee / 100.0          // 例如 0.3 表示 0.3%
			feeMultiplier := 1000.0 - feeRatio*10 // 例如 0.3% = 997
			amountInWithFee := new(big.Float).Mul(amountFloat, big.NewFloat(feeMultiplier))

			// 计算输出量: amountOut = (amountIn * 997 * reserveOut) / ((reserveIn * 1000) + (amountIn * 997))
			numerator := new(big.Float).Mul(amountInWithFee, reserveOut)
			denominatorPart1 := new(big.Float).Mul(reserveIn, big.NewFloat(1000.0))
			denominator := new(big.Float).Add(denominatorPart1, amountInWithFee)
			amountOut := new(big.Float).Quo(numerator, denominator)

			amount, _ = amountOut.Float64()
		} else if pool.Protocol == ProtocolUniswapV3 || pool.Protocol == ProtocolUniswapV4 {
			// V3/V4 使用集中流动性模型，计算更复杂
			// 简化处理：使用类似 V2 的公式，但需要考虑价格范围
			// 这里使用简化的恒定乘积公式作为近似
			var reserveIn, reserveOut *big.Float
			if step.FromToken == pool.Token0 {
				reserveIn = reserve0
				reserveOut = reserve1
			} else {
				reserveIn = reserve1
				reserveOut = reserve0
			}

			// V3 手续费通常从合约读取，这里使用配置的费率
			// V3 手续费单位是 1e-6，例如 3000 表示 0.3%
			feeRatio := step.Fee / 100.0          // 例如 0.3 表示 0.3%
			feeMultiplier := 1000.0 - feeRatio*10 // 例如 0.3% = 997
			amountInWithFee := new(big.Float).Mul(amountFloat, big.NewFloat(feeMultiplier))

			// 简化计算：使用类似 V2 的恒定乘积公式
			// 注意：V3 的实际计算需要考虑 tick 和流动性分布，这里使用简化公式作为近似
			numerator := new(big.Float).Mul(amountInWithFee, reserveOut)
			denominatorPart1 := new(big.Float).Mul(reserveIn, big.NewFloat(1000.0))
			denominator := new(big.Float).Add(denominatorPart1, amountInWithFee)
			amountOut := new(big.Float).Quo(numerator, denominator)

			amount, _ = amountOut.Float64()
		} else {
			// V1 或其他协议，使用简化的费率扣除
			feeRatio := step.Fee / 100.0
			amount = amount * (1 - feeRatio)
		}

		// 检查金额是否有效
		if amount <= 0 {
			return 0, false
		}
	}

	// 计算利润
	profit := amount - initial
	return amount, profit >= minProfit
}

func convertToOpportunity(path []graphEdge, startToken common.Address, initialAmount, estimated float64) ArbitrageOpportunity {
	steps := make([]ArbitrageStep, 0, len(path))
	for _, edge := range path {
		steps = append(steps, ArbitrageStep{
			Pool:      edge.Pool,
			FromToken: edge.FromToken.Hex(),
			ToToken:   edge.ToToken.Hex(),
			Protocol:  edge.Protocol,
			Fee:       edge.Fee,
		})
	}
	return ArbitrageOpportunity{
		Path:            steps,
		StartToken:      startToken.Hex(),
		InitialAmount:   initialAmount,
		EstimatedReturn: estimated,
	}
}

func (af *ArbitrageFinder) isPathSeen(key string) bool {
	af.mu.RLock()
	defer af.mu.RUnlock()
	_, exists := af.seenPaths[key]
	return exists
}

func (af *ArbitrageFinder) markPath(key string) {
	af.mu.Lock()
	defer af.mu.Unlock()
	af.seenPaths[key] = struct{}{}
}

func hashPath(path []graphEdge) string {
	if len(path) == 0 {
		return ""
	}
	items := make([]string, 0, len(path))
	for _, edge := range path {
		items = append(items, edge.Protocol+":"+edge.Pool.Address.Hex()+":"+edge.FromToken.Hex()+"->"+edge.ToToken.Hex())
	}
	sort.Strings(items)
	return strings.Join(items, "|")
}

func formatPath(path []graphEdge) string {
	if len(path) == 0 {
		return ""
	}
	var builder strings.Builder
	for idx, edge := range path {
		if idx > 0 {
			builder.WriteString(" => ")
		}
		builder.WriteString(edge.Protocol)
		builder.WriteString("[")
		builder.WriteString(edge.Pool.Address.Hex())
		builder.WriteString("] ")
		builder.WriteString(edge.FromToken.Hex())
		builder.WriteString(" -> ")
		builder.WriteString(edge.ToToken.Hex())
		builder.WriteString(" (token0=")
		builder.WriteString(edge.Pool.Token0.Hex())
		builder.WriteString(", token1=")
		builder.WriteString(edge.Pool.Token1.Hex())
		builder.WriteString(")")
	}
	return builder.String()
}
