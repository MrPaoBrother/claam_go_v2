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
}

type poolDetail struct {
	Address  common.Address
	Token0   common.Address
	Token1   common.Address
	Fee      float64
	Protocol string
}

// PoolMonitor 池子监控器
type PoolMonitor struct {
	wsURL      string
	ethClient  *ethclient.Client
	chainID    *big.Int
	protocols  map[common.Hash]protocolConfig
	knownPools *sync.Map
	pairABI    *abi.ABI
	uniV3ABI   *abi.ABI
}

const pairABIJSON = `
[
	{
		"constant": true,
		"inputs": [],
		"name": "token0",
		"outputs": [
			{
				"name": "",
				"type": "address"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "token1",
		"outputs": [
			{
				"name": "",
				"type": "address"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]
`

const uniswapV3ABIJSON = `
[
	{
		"inputs": [],
		"name": "token0",
		"outputs": [
			{
				"internalType": "address",
				"name": "",
				"type": "address"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "token1",
		"outputs": [
			{
				"internalType": "address",
				"name": "",
				"type": "address"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "fee",
		"outputs": [
			{
				"internalType": "uint24",
				"name": "",
				"type": "uint24"
			}
		],
		"stateMutability": "view",
		"type": "function"
	}
]
`

// NewPoolMonitor 创建新的池子监控器
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

	v2ABI, err := abi.JSON(strings.NewReader(pairABIJSON))
	if err != nil {
		ethCli.Close()
		return nil, fmt.Errorf("解析 V2 ABI 失败: %w", err)
	}

	v3ABI, err := abi.JSON(strings.NewReader(uniswapV3ABIJSON))
	if err != nil {
		ethCli.Close()
		return nil, fmt.Errorf("解析 V3 ABI 失败: %w", err)
	}

	protocols := map[common.Hash]protocolConfig{
		common.HexToHash("0xd78ad95fa46c994b6551d0da85fc275fe613a1e06de587873393dff2aea0d903"): {
			Name:            "UniswapV2LikeSwap",
			SwapTopic:       common.HexToHash("0xd78ad95fa46c994b6551d0da85fc275fe613a1e06de587873393dff2aea0d903"),
			ContractABI:     &v2ABI,
			StaticFee:       0.25,
			FeeFromContract: false,
		},
		common.HexToHash("0xc42079f94a6350d7e6235f29174924f928cc2ac818eb64fed8004e115fbcca67"): {
			Name:            "UniswapV3Swap",
			SwapTopic:       common.HexToHash("0xc42079f94a6350d7e6235f29174924f928cc2ac818eb64fed8004e115fbcca67"),
			ContractABI:     &v3ABI,
			StaticFee:       0,
			FeeFromContract: true,
		},
	}

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

// Close 关闭监控器
func (pm *PoolMonitor) Close() error {
	if pm.ethClient != nil {
		pm.ethClient.Close()
	}
	return nil
}

// Process 开始处理区块订阅和池子发现
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
		blockNumber := hexToUint64(head.Number)
		blockTimestamp := hexToUint64(head.Timestamp)
		fmt.Printf("高度: %s (十进制: %d)\n", head.Number, blockNumber)
		fmt.Printf("哈希: %s\n", head.Hash)
		fmt.Printf("时间戳: %s (UTC: %s)\n", head.Timestamp, time.Unix(int64(blockTimestamp), 0).UTC())
		fmt.Printf("矿工: %s\n", head.Miner)

		if err := pm.processBlock(ctx, head); err != nil {
			log.Printf("处理区块详情失败: %v", err)
		}
	}
}

func (pm *PoolMonitor) processBlock(ctx context.Context, head BlockHead) error {
	number, err := hexToBigInt(head.Number)
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

	return nil
}

// discoverPoolsFromTransactions 并发扫描交易，发现所有新池子
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

	token0, err := pm.callTokenAddress(ctx, contract, "token0")
	if err != nil {
		return false, poolDetail{}, err
	}
	token1, err := pm.callTokenAddress(ctx, contract, "token1")
	if err != nil {
		return false, poolDetail{}, err
	}

	poolFee := cfg.StaticFee
	if cfg.FeeFromContract {
		poolFee, err = pm.callPoolFee(ctx, contract)
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

func (pm *PoolMonitor) callTokenAddress(ctx context.Context, contract *bind.BoundContract, method string) (common.Address, error) {
	var raw []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &raw, method); err != nil {
		return common.Address{}, err
	}
	if len(raw) != 1 {
		return common.Address{}, fmt.Errorf("unexpected %s return length %d", method, len(raw))
	}

	switch v := raw[0].(type) {
	case common.Address:
		return v, nil
	case [20]byte:
		return common.BytesToAddress(v[:]), nil
	case string:
		return common.HexToAddress(v), nil
	default:
		return common.Address{}, fmt.Errorf("unexpected %s return type %T", method, raw[0])
	}
}

func (pm *PoolMonitor) callPoolFee(ctx context.Context, contract *bind.BoundContract) (float64, error) {
	var raw []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &raw, "fee"); err != nil {
		return 0, err
	}
	if len(raw) != 1 {
		return 0, fmt.Errorf("unexpected fee return length %d", len(raw))
	}

	var feeValue uint64
	switch v := raw[0].(type) {
	case uint8:
		feeValue = uint64(v)
	case uint16:
		feeValue = uint64(v)
	case uint32:
		feeValue = uint64(v)
	case uint64:
		feeValue = v
	case *big.Int:
		feeValue = v.Uint64()
	default:
		return 0, fmt.Errorf("unexpected fee return type %T", raw[0])
	}

	// Uniswap V3 fee 返回单位为 1e-6，换算为百分比需除以 1e4
	return float64(feeValue) / 1e4, nil
}

// 辅助函数：将十六进制字符串转为uint64
func hexToUint64(hexStr string) uint64 {
	var n uint64
	fmt.Sscanf(hexStr, "0x%x", &n)
	return n
}

func hexToBigInt(hexStr string) (*big.Int, error) {
	clean := strings.TrimPrefix(hexStr, "0x")
	if clean == "" {
		return big.NewInt(0), nil
	}
	number, ok := new(big.Int).SetString(clean, 16)
	if !ok {
		return nil, fmt.Errorf("invalid hex: %s", hexStr)
	}
	return number, nil
}
