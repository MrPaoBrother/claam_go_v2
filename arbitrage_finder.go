package main

import (
	"context"
	"log"
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
	store      *PoolStore
	queue      *ArbitrageQueue
	cfg        *AppConfig
	mu         sync.RWMutex
	tokens     []common.Address
	tokenIndex map[common.Address]int
	adjacency  map[int][]graphEdge
	seenPaths  map[string]struct{}
}

// NewArbitrageFinder 创建套利路径发现者
func NewArbitrageFinder(store *PoolStore, queue *ArbitrageQueue, cfg *AppConfig) *ArbitrageFinder {
	return &ArbitrageFinder{
		store:      store,
		queue:      queue,
		cfg:        cfg,
		tokens:     make([]common.Address, 0),
		tokenIndex: make(map[common.Address]int),
		adjacency:  make(map[int][]graphEdge),
		seenPaths:  make(map[string]struct{}),
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
	tokenIndex := make(map[common.Address]int)
	tokens := make([]common.Address, 0)
	adjacency := make(map[int][]graphEdge)

	getIndex := func(addr common.Address) int {
		if idx, ok := tokenIndex[addr]; ok {
			return idx
		}
		idx := len(tokens)
		tokenIndex[addr] = idx
		tokens = append(tokens, addr)
		return idx
	}

	for _, p := range pools {
		idx0 := getIndex(p.Token0)
		idx1 := getIndex(p.Token1)

		rate := 1 - p.Fee/100.0
		if rate <= 0 {
			rate = 1e-6
		}

		edgeAB := graphEdge{
			Pool:      p,
			Protocol:  p.Protocol,
			Fee:       p.Fee,
			Rate:      rate,
			FromToken: p.Token0,
			ToToken:   p.Token1,
			FromIndex: idx0,
			ToIndex:   idx1,
		}
		edgeBA := graphEdge{
			Pool:      p,
			Protocol:  p.Protocol,
			Fee:       p.Fee,
			Rate:      rate,
			FromToken: p.Token1,
			ToToken:   p.Token0,
			FromIndex: idx1,
			ToIndex:   idx0,
		}
		adjacency[idx0] = append(adjacency[idx0], edgeAB)
		adjacency[idx1] = append(adjacency[idx1], edgeBA)
	}

	for idx := range tokens {
		if _, ok := adjacency[idx]; !ok {
			adjacency[idx] = nil
		}
	}

	af.mu.Lock()
	defer af.mu.Unlock()
	af.tokens = tokens
	af.tokenIndex = tokenIndex
	af.adjacency = adjacency
	af.seenPaths = make(map[string]struct{})
}

func (af *ArbitrageFinder) enumerateCycles() {
	af.mu.RLock()
	tokens := append([]common.Address(nil), af.tokens...)
	adjacency := af.adjacency
	af.mu.RUnlock()

	n := len(tokens)
	if n == 0 {
		return
	}

	maxDepth := af.cfg.ArbMaxHops
	if maxDepth < 2 {
		maxDepth = 2
	}
	initialAmount := af.cfg.ArbInitialCapital
	minProfit := af.cfg.ArbMinProfit

	components := stronglyConnectedComponents(n, adjacency)
	for _, comp := range components {
		if len(comp) < 2 {
			continue
		}
		compSet := make(map[int]struct{}, len(comp))
		for _, idx := range comp {
			compSet[idx] = struct{}{}
		}

		for _, start := range comp {
			visited := make(map[int]struct{}, len(comp))
			visited[start] = struct{}{}
			af.dfsCycles(start, start, compSet, adjacency, visited, nil, maxDepth, initialAmount, minProfit)
		}
	}
}

func (af *ArbitrageFinder) dfsCycles(start, current int, compSet map[int]struct{},
	adjacency map[int][]graphEdge, visited map[int]struct{},
	path []graphEdge, maxDepth int, initialAmount, minProfit float64) {

	if len(path) >= maxDepth {
		return
	}

	for _, edge := range adjacency[current] {
		if _, ok := compSet[edge.ToIndex]; !ok {
			continue
		}

		path = append(path, edge)

		if edge.ToIndex == start && len(path) >= 2 {
			cycle := append([]graphEdge(nil), path...)
			af.handleCycle(cycle, initialAmount, minProfit)
			path = path[:len(path)-1]
			continue
		}

		if _, seen := visited[edge.ToIndex]; seen {
			path = path[:len(path)-1]
			continue
		}

		visited[edge.ToIndex] = struct{}{}
		af.dfsCycles(start, edge.ToIndex, compSet, adjacency, visited, path, maxDepth, initialAmount, minProfit)
		delete(visited, edge.ToIndex)
		path = path[:len(path)-1]
	}
}

func (af *ArbitrageFinder) handleCycle(cycle []graphEdge, initialAmount, minProfit float64) {
	if len(cycle) < 2 {
		return
	}

	pathKey := hashPath(cycle)
	if af.isPathSeen(pathKey) {
		return
	}

	pathDesc := formatPath(cycle)
	// log.Printf("检测到正循环 (跳数 %d): %s", len(cycle), pathDesc)

	estimated, profitable := af.simulatePath(initialAmount, cycle, minProfit)
	if !profitable {
		return
	}

	log.Printf("初步可盈利套利 (跳数 %d): 初始 %.6f USDT -> 预计 %.6f USDT, 利润 %.6f, 路径: %s",
		len(cycle), initialAmount, estimated, estimated-initialAmount, pathDesc)

	af.markPath(pathKey)
	startToken := cycle[0].FromToken
	af.queue.Publish(convertToOpportunity(cycle, startToken, initialAmount, estimated))
}

func (af *ArbitrageFinder) simulatePath(initial float64, path []graphEdge, minProfit float64) (float64, bool) {
	amount := initial
	for _, step := range path {
		if step.Rate <= 0 {
			return 0, false
		}
		amount *= step.Rate
	}
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

func stronglyConnectedComponents(n int, adjacency map[int][]graphEdge) [][]int {
	index := 0
	stack := make([]int, 0, n)
	onStack := make([]bool, n)
	indices := make([]int, n)
	lowlink := make([]int, n)
	for i := range indices {
		indices[i] = -1
		lowlink[i] = -1
	}

	var result [][]int
	var strongConnect func(v int)

	strongConnect = func(v int) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, edge := range adjacency[v] {
			w := edge.ToIndex
			if indices[w] == -1 {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlink[v] {
					lowlink[v] = indices[w]
				}
			}
		}

		if lowlink[v] == indices[v] {
			var component []int
			for {
				if len(stack) == 0 {
					break
				}
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				component = append(component, w)
				if w == v {
					break
				}
			}
			if len(component) > 0 {
				result = append(result, component)
			}
		}
	}

	for v := 0; v < n; v++ {
		if indices[v] == -1 {
			strongConnect(v)
		}
	}

	return result
}
