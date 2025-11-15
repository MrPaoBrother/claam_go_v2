package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type poolDetail struct {
	Address  common.Address
	Token0   common.Address
	Token1   common.Address
	Fee      float64
	Protocol string
	Reserve0 *big.Int // token0 储备量
	Reserve1 *big.Int // token1 储备量
}

// PoolDiscoverer 从队列中消费区块，发现新的池子并写入存储
type PoolDiscoverer struct {
	queue      *BlockQueue
	client     *ethclient.Client
	store      *PoolStore
	protocols  map[common.Hash]protocolConfig
	knownPools *sync.Map
}

// NewPoolDiscoverer 创建池子发现者
func NewPoolDiscoverer(queue *BlockQueue, client *ethclient.Client, store *PoolStore, protocols map[common.Hash]protocolConfig) *PoolDiscoverer {
	return &PoolDiscoverer{
		queue:      queue,
		client:     client,
		store:      store,
		protocols:  protocols,
		knownPools: &sync.Map{},
	}
}

// Start 开始消费区块
func (pd *PoolDiscoverer) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-pd.queue.Subscribe():
			go pd.handleBlock(ctx, event)
		}
	}
}

func (pd *PoolDiscoverer) handleBlock(ctx context.Context, event BlockEvent) {
	start := time.Now()

	block, err := pd.client.BlockByHash(ctx, event.Hash)
	if err != nil {
		block, err = pd.client.BlockByNumber(ctx, event.Number)
		if err != nil {
			log.Printf("获取区块失败 %s: %v", event.Number.String(), err)
			return
		}
	}

	txs := block.Transactions()
	log.Printf("区块 %s 交易总数: %d", event.Number.String(), len(txs))

	discovered := pd.discoverPoolsFromTransactions(ctx, txs)
	for _, pool := range discovered {
		if err := pd.store.InsertPoolIfNotExists(pool); err != nil {
			log.Printf("写入池子失败 %s: %v", pool.Address.Hex(), err)
			continue
		}
		log.Printf("记录池子 %s 协议 %s", pool.Address.Hex(), pool.Protocol)
	}

	log.Printf("区块 %s 处理耗时: %v", event.Number.String(), time.Since(start))
}

// discoverPoolsFromTransactions 并发扫描交易，发现所有新池子
// 参数 ctx 是上下文，txs 是交易列表
// 使用 goroutine 并发处理每个交易，获取交易回执并分析日志
// 根据协议配置的 Swap Topic 筛选出相关的池子日志，并调用合约获取池子信息
// 返回所有新发现的池子信息列表
func (pd *PoolDiscoverer) discoverPoolsFromTransactions(ctx context.Context, txs []*types.Transaction) []poolDetail {
	type poolResult struct {
		pool poolDetail
		err  error
	}

	poolChan := make(chan poolResult, len(txs))
	var wg sync.WaitGroup

	for _, tx := range txs {
		wg.Add(1)
		go func(tx *types.Transaction) {
			defer wg.Done()

			receipt, err := pd.client.TransactionReceipt(ctx, tx.Hash())
			if err != nil {
				return
			}

			for _, lg := range receipt.Logs {
				if len(lg.Topics) == 0 {
					continue
				}

				topic0 := lg.Topics[0].Hex()
				cfg, ok := pd.protocols[lg.Topics[0]]
				if !ok {
					// V4 可能使用与 V3 相同的事件签名（因为池子结构类似）
					// 尝试使用 V3 的配置来处理 V4 事件
					v3TopicHash := common.HexToHash(UniswapV3SwapTopic)
					if lg.Topics[0] == v3TopicHash {
						// 检查是否是 V4 池子（可能需要通过地址或其他方式判断）
						// 暂时也当作 V3 处理，后续可以根据实际需求区分
						v3Cfg, v3Ok := pd.protocols[v3TopicHash]
						if v3Ok {
							cfg = v3Cfg
							ok = true
						}
					}
					if !ok {
						// 调试：记录未匹配的事件 Topic（限制频率，避免日志过多）
						// 只记录以 0x01 或 0x02 开头的，可能是 V4 相关
						if len(topic0) >= 4 && (topic0[2:4] == "01" || topic0[2:4] == "02" || topic0[2:4] == "c4") {
							log.Printf("未匹配的事件 Topic: %s, 地址: %s, 区块: %s", topic0, lg.Address.Hex(), receipt.BlockNumber.String())
						}
						continue
					}
				}

				isNew, poolInfo, err := pd.inspectPool(ctx, lg, cfg)
				if err != nil {
					continue
				}

				if isNew {
					poolChan <- poolResult{pool: poolInfo}
				}
			}
		}(tx)
	}

	go func() {
		wg.Wait()
		close(poolChan)
	}()

	var discovered []poolDetail
	for result := range poolChan {
		if result.err == nil {
			discovered = append(discovered, result.pool)
		}
	}

	return discovered
}

// inspectPool 检查并解析池子信息
func (pd *PoolDiscoverer) inspectPool(ctx context.Context, lg *types.Log, cfg protocolConfig) (bool, poolDetail, error) {
	poolAddr := lg.Address.Hex()

	if _, exists := pd.knownPools.Load(poolAddr); exists {
		return false, poolDetail{}, nil
	}

	if cfg.ContractABI == nil {
		return false, poolDetail{}, fmt.Errorf("协议 %s 未配置 ABI", cfg.Name)
	}

	contract := bind.NewBoundContract(lg.Address, *cfg.ContractABI, pd.client, pd.client, pd.client)

	token0Method := cfg.Token0Method
	if token0Method == "" {
		token0Method = "token0"
	}
	token1Method := cfg.Token1Method
	if token1Method == "" {
		token1Method = "token1"
	}

	var token0, token1 common.Address
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

	// 获取储备量
	var reserve0, reserve1 *big.Int
	if cfg.Name == ProtocolUniswapV2Like {
		// V2 协议使用 getReserves 方法
		reserve0, reserve1, err = CallGetReserves(ctx, contract)
		if err != nil {
			// 如果获取储备量失败，使用默认值 0
			reserve0 = big.NewInt(0)
			reserve1 = big.NewInt(0)
		}
	} else if cfg.Name == ProtocolUniswapV3 || cfg.Name == ProtocolUniswapV4 {
		// V3/V4 协议通过 ERC20 balanceOf 获取池子合约的代币余额
		poolAddr := lg.Address
		reserve0, err = CallERC20BalanceOf(ctx, pd.client, token0, poolAddr)
		if err != nil {
			reserve0 = big.NewInt(0)
		}
		reserve1, err = CallERC20BalanceOf(ctx, pd.client, token1, poolAddr)
		if err != nil {
			reserve1 = big.NewInt(0)
		}
	} else {
		// V1 暂时不支持储备量获取，设为 0
		reserve0 = big.NewInt(0)
		reserve1 = big.NewInt(0)
	}

	pd.knownPools.Store(poolAddr, true)

	return true, poolDetail{
		Address:  lg.Address,
		Token0:   token0,
		Token1:   token1,
		Fee:      poolFee,
		Protocol: cfg.Name,
		Reserve0: reserve0,
		Reserve1: reserve1,
	}, nil
}
