package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gorilla/websocket"
)

// JSON-RPC 请求结构
type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

// JSON-RPC 响应结构（通用）
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// 错误信息结构
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// 区块头数据结构（订阅 newHeads 时返回）
type BlockHead struct {
	Number           string `json:"number"`
	Hash             string `json:"hash"`
	ParentHash       string `json:"parentHash"`
	Nonce            string `json:"nonce"`
	Sha3Uncles       string `json:"sha3Uncles"`
	LogsBloom        string `json:"logsBloom"`
	TransactionsRoot string `json:"transactionsRoot"`
	StateRoot        string `json:"stateRoot"`
	Miner            string `json:"miner"`
	Difficulty       string `json:"difficulty"`
	TotalDifficulty  string `json:"totalDifficulty"`
	ExtraData        string `json:"extraData"`
	Size             string `json:"size"`
	GasLimit         string `json:"gasLimit"`
	GasUsed          string `json:"gasUsed"`
	Timestamp        string `json:"timestamp"`
}

type protocolConfig struct {
	Name            string
	SwapTopic       common.Hash
	ContractABI     *abi.ABI
	StaticFee       float64
	FeeFromContract bool
	Token0Method    string
	Token1Method    string
	FixedToken0     *common.Address
	FixedToken1     *common.Address
}

type poolDetail struct {
	Address  common.Address
	Token0   common.Address
	Token1   common.Address
	Fee      float64
	Protocol string
}

// PoolMonitor 池子监控器，负责订阅 BSC 区块并发现新的流动性池子
// 支持 Uniswap V2 和 V3 协议的池子发现
type PoolMonitor struct {
	wsURL      string
	ethClient  *ethclient.Client
	chainID    *big.Int
	protocols  map[common.Hash]protocolConfig
	knownPools *sync.Map
	pairABI    *abi.ABI
	uniV3ABI   *abi.ABI
}

// NewPoolMonitor 创建新的池子监控器实例
// 参数 wsURL 是 BSC WebSocket 节点地址
// 返回 PoolMonitor 实例和错误信息
// 初始化时会连接以太坊客户端、解析 ABI 并配置支持的协议
func NewPoolMonitor(wsURL string) (*PoolMonitor, error) {
	ctx := context.Background()

	ethCli, err := ethclient.DialContext(ctx, wsURL)
	if err != nil {
		return nil, fmt.Errorf("无法创建以太坊客户端: %w", err)
	}

	chainID, err := ethCli.NetworkID(ctx)
	if err != nil {
		ethCli.Close()
		return nil, fmt.Errorf("获取链ID失败: %w", err)
	}

	v1ABI, err := abi.JSON(strings.NewReader(UniswapV1ExchangeABIJSON))
	if err != nil {
		ethCli.Close()
		return nil, fmt.Errorf("解析 V1 ABI 失败: %w", err)
	}

	v2ABI, err := abi.JSON(strings.NewReader(PairABIJSON))
	if err != nil {
		ethCli.Close()
		return nil, fmt.Errorf("解析 V2 ABI 失败: %w", err)
	}

	v3ABI, err := abi.JSON(strings.NewReader(UniswapV3ABIJSON))
	if err != nil {
		ethCli.Close()
		return nil, fmt.Errorf("解析 V3 ABI 失败: %w", err)
	}

	protocols := GetProtocolsConfig(&v1ABI, &v2ABI, &v3ABI)

	return &PoolMonitor{
		wsURL:      wsURL,
		ethClient:  ethCli,
		chainID:    chainID,
		protocols:  protocols,
		knownPools: &sync.Map{},
		pairABI:    &v2ABI,
		uniV3ABI:   &v3ABI,
	}, nil
}

// Close 关闭监控器并释放资源
// 关闭以太坊客户端连接
// 返回关闭过程中的错误
func (pm *PoolMonitor) Close() error {
	if pm.ethClient != nil {
		pm.ethClient.Close()
	}
	return nil
}

// Process 开始处理区块订阅和池子发现的主循环
// 参数 ctx 用于控制协程的生命周期，当 ctx 被取消时，函数会退出
// 通过 WebSocket 订阅新区块头，当收到新区块时，会分析区块中的交易并发现新的流动性池子
// 支持自动重连机制，当连接断开时会自动重新连接并重新订阅
// 返回处理过程中的错误
func (pm *PoolMonitor) Process(ctx context.Context) error {
	// 连接 WebSocket
	c, _, err := websocket.DefaultDialer.Dial(pm.wsURL, nil)
	if err != nil {
		return fmt.Errorf("无法连接到节点: %w", err)
	}
	defer c.Close()

	// 发送订阅请求
	subReq := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "eth_subscribe",
		Params:  []interface{}{"newHeads"},
	}
	if err := c.WriteJSON(subReq); err != nil {
		return fmt.Errorf("发送订阅请求失败: %w", err)
	}

	// 接收订阅响应
	var subResp rpcResponse
	if err := c.ReadJSON(&subResp); err != nil {
		return fmt.Errorf("读取订阅响应失败: %w", err)
	}
	if subResp.Error != nil {
		return fmt.Errorf("订阅失败: code=%d, msg=%s", subResp.Error.Code, subResp.Error.Message)
	}
	var subID string
	if err := json.Unmarshal(subResp.Result, &subID); err != nil {
		return fmt.Errorf("解析订阅ID失败: %w", err)
	}
	log.Printf("订阅成功，订阅ID: %s", subID)

	// 循环监听新区块通知
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var blockResp rpcResponse
		if err := c.ReadJSON(&blockResp); err != nil {
			log.Printf("读取区块数据失败: %v，尝试重连...", err)
			time.Sleep(3 * time.Second)
			_ = c.Close()
			c, _, err = websocket.DefaultDialer.Dial(pm.wsURL, nil)
			if err != nil {
				log.Printf("重连失败: %v，3秒后重试", err)
				continue
			}
			if err := c.WriteJSON(subReq); err != nil {
				log.Printf("重新订阅失败: %v", err)
			}
			continue
		}

		if blockResp.Error != nil {
			log.Printf("收到错误响应: code=%d, msg=%s", blockResp.Error.Code, blockResp.Error.Message)
			continue
		}

		// 处理非订阅通知
		if blockResp.Method != "eth_subscription" {
			if blockResp.ID != 0 && len(blockResp.Result) > 0 {
				var newSubID string
				if err := json.Unmarshal(blockResp.Result, &newSubID); err != nil {
					log.Printf("解析订阅ID失败: %v", err)
					continue
				}
				subID = newSubID
				log.Printf("订阅更新成功，订阅ID: %s", subID)
			}
			continue
		}

		// 解析订阅通知
		var params struct {
			Subscription string    `json:"subscription"`
			Result       BlockHead `json:"result"`
		}
		if err := json.Unmarshal(blockResp.Params, &params); err != nil {
			log.Printf("解析区块数据失败: %v", err)
			continue
		}

		// 处理区块
		head := params.Result
		fmt.Printf("\n新区块:\n")
		blockNumber := HexToUint64(head.Number)
		blockTimestamp := HexToUint64(head.Timestamp)
		fmt.Printf("高度: %s (十进制: %d)\n", head.Number, blockNumber)
		fmt.Printf("哈希: %s\n", head.Hash)
		fmt.Printf("时间戳: %s (UTC: %s)\n", head.Timestamp, time.Unix(int64(blockTimestamp), 0).UTC())
		fmt.Printf("矿工: %s\n", head.Miner)

		if err := pm.processBlock(ctx, head); err != nil {
			log.Printf("处理区块详情失败: %v", err)
		}
	}
}

// processBlock 处理单个区块，获取区块详情并扫描交易发现新池子
// 参数 ctx 是上下文，head 是区块头信息
// 会获取完整区块数据，然后并发扫描所有交易以发现新的流动性池子
// 返回处理过程中的错误
func (pm *PoolMonitor) processBlock(ctx context.Context, head BlockHead) error {
	startTime := time.Now()

	number, err := HexToBigInt(head.Number)
	if err != nil {
		return fmt.Errorf("解析区块高度失败: %w", err)
	}

	block, err := pm.ethClient.BlockByNumber(ctx, number)
	if err != nil {
		return fmt.Errorf("获取区块失败: %w", err)
	}

	txs := block.Transactions()
	fmt.Printf("交易总数: %d\n", len(txs))

	// 并发扫描交易，发现新池子
	discoveredPools := pm.discoverPoolsFromTransactions(ctx, txs)

	// 打印发现的池子信息
	for _, pool := range discoveredPools {
		fmt.Printf("  [新池子] 协议: %s 地址: %s token0: %s token1: %s fee: %.4f%%\n",
			pool.Protocol, pool.Address.Hex(), pool.Token0.Hex(), pool.Token1.Hex(), pool.Fee)
	}

	// 输出处理耗时
	elapsed := time.Since(startTime)
	fmt.Printf("处理耗时: %v\n", elapsed)

	return nil
}

// discoverPoolsFromTransactions 并发扫描交易，发现所有新池子
// 参数 ctx 是上下文，txs 是交易列表
// 使用 goroutine 并发处理每个交易，获取交易回执并分析日志
// 根据协议配置的 Swap Topic 筛选出相关的池子日志，并调用合约获取池子信息
// 返回所有新发现的池子信息列表
func (pm *PoolMonitor) discoverPoolsFromTransactions(ctx context.Context, txs []*types.Transaction) []poolDetail {
	type poolResult struct {
		pool poolDetail
		err  error
	}

	// 使用 channel 收集结果
	poolChan := make(chan poolResult, len(txs))

	// 使用 WaitGroup 等待所有 goroutine 完成
	var wg sync.WaitGroup

	// 并发处理每个交易
	for _, tx := range txs {
		wg.Add(1)
		go func(tx *types.Transaction) {
			defer wg.Done()
			// TODO 由于免费节点不提供批量查，暂时先单个操作
			receipt, err := pm.ethClient.TransactionReceipt(ctx, tx.Hash())
			if err != nil {
				// 获取回执失败，不发送结果
				return
			}

			// 处理交易日志
			for _, lg := range receipt.Logs {
				if len(lg.Topics) == 0 {
					continue
				}

				cfg, ok := pm.protocols[lg.Topics[0]]
				if !ok {
					continue
				}

				isNew, poolInfo, err := pm.inspectPool(ctx, lg, cfg)
				if err != nil {
					// 池子解析失败，继续处理下一个
					continue
				}

				if isNew {
					// 发现新池子，发送结果
					poolChan <- poolResult{pool: poolInfo, err: nil}
				}
			}
		}(tx)
	}

	// 等待所有 goroutine 完成
	go func() {
		wg.Wait()
		close(poolChan)
	}()

	// 收集所有发现的池子
	var discoveredPools []poolDetail
	for result := range poolChan {
		if result.err == nil {
			discoveredPools = append(discoveredPools, result.pool)
		}
	}

	return discoveredPools
}

// inspectPool 检查并解析池子信息
// 参数 ctx 是上下文，lg 是日志信息，cfg 是协议配置
// 首先检查池子是否已知，如果已知则返回 false
// 如果未知，则调用合约获取 token0、token1 和费率信息
// 返回是否为新池子、池子详情和错误信息
func (pm *PoolMonitor) inspectPool(ctx context.Context, lg *types.Log, cfg protocolConfig) (bool, poolDetail, error) {
	poolAddr := lg.Address.Hex()

	// 检查是否已知池子
	if _, exists := pm.knownPools.Load(poolAddr); exists {
		return false, poolDetail{}, nil
	}

	if cfg.ContractABI == nil {
		return false, poolDetail{}, fmt.Errorf("协议 %s 未配置 ABI", cfg.Name)
	}

	contract := bind.NewBoundContract(lg.Address, *cfg.ContractABI, pm.ethClient, pm.ethClient, pm.ethClient)

	token0Method := cfg.Token0Method
	if token0Method == "" {
		token0Method = "token0"
	}
	token1Method := cfg.Token1Method
	if token1Method == "" {
		token1Method = "token1"
	}

	var token0 common.Address
	var token1 common.Address
	var err error

	if cfg.FixedToken0 != nil {
		token0 = *cfg.FixedToken0
	} else if token0Method != "" {
		token0, err = CallTokenAddress(ctx, contract, token0Method)
		if err != nil {
			return false, poolDetail{}, err
		}
	}

	if cfg.FixedToken1 != nil {
		token1 = *cfg.FixedToken1
	} else if token1Method != "" {
		token1, err = CallTokenAddress(ctx, contract, token1Method)
		if err != nil {
			return false, poolDetail{}, err
		}
	}

	poolFee := cfg.StaticFee
	if cfg.FeeFromContract {
		poolFee, err = CallPoolFee(ctx, contract)
		if err != nil {
			return false, poolDetail{}, err
		}
	}

	// 标记为已知池子
	pm.knownPools.Store(poolAddr, true)

	return true, poolDetail{
		Address:  lg.Address,
		Token0:   token0,
		Token1:   token1,
		Fee:      poolFee,
		Protocol: cfg.Name,
	}, nil
}
