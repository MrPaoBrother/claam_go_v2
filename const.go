package main

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// BSC WebSocket 节点地址
const (
	// DefaultBSCWssURL BSC 公共 WebSocket 节点地址（默认值）
	DefaultBSCWssURL = "wss://bsc.drpc.org"
)

// 协议 Swap Topic 哈希值
const (
	// UniswapV1TokenPurchaseTopic Uniswap V1 TokenPurchase 事件 Topic
	// 对应事件签名: TokenPurchase(address buyer, uint256 tokens_sold, uint256 eth_bought)
	UniswapV1TokenPurchaseTopic = "0xcd60aa75dea3072fbc07ae6d7d856b5dc5f4eee88854f5b4abf7b680ef8bc50f"

	// UniswapV1EthPurchaseTopic Uniswap V1 EthPurchase 事件 Topic
	// 对应事件签名: EthPurchase(address buyer, uint256 eth_sold, uint256 tokens_bought)
	UniswapV1EthPurchaseTopic = "0x7f4091b46c33e918a0f3aa42307641d17bb67029427a5369e54b353984238705"

	// UniswapV2SwapTopic Uniswap V2 及类似协议的 Swap 事件 Topic
	// 对应事件签名: Swap(address indexed sender, uint256 amount0In, uint256 amount1In, uint256 amount0Out, uint256 amount1Out, address indexed to)
	UniswapV2SwapTopic = "0xd78ad95fa46c994b6551d0da85fc275fe613ce37657fb8d5e3d130840159d822"

	// UniswapV3SwapTopic Uniswap V3 协议的 Swap 事件 Topic
	// 对应事件签名: Swap(address indexed sender, address indexed recipient, int256 amount0, int256 amount1, uint160 sqrtPriceX96, uint128 liquidity, int24 tick)
	UniswapV3SwapTopic = "0xc42079f94a6350d7e6235f29174924f928cc2ac818eb64fed8004e115fbcca67"

	// UniswapV4SwapTopic Uniswap V4 协议的 Swap 事件 Topic（基于当前公开规范）
	// 对应事件签名: Swap(address sender, bytes32 poolId, int128 amount0, int128 amount1, uint160 sqrtPriceX96, int24 tick)
	UniswapV4SwapTopic = "0x017b45c007bc4ff26fb88674c8e55e9c705cf8b79157c48987a35b92e5c2cece"
)

// 协议名称
const (
	// ProtocolUniswapV1 Uniswap V1 及类似协议名称
	ProtocolUniswapV1 = "UniswapV1LikeSwap"

	// ProtocolUniswapV2Like Uniswap V2 及类似协议名称
	ProtocolUniswapV2Like = "UniswapV2LikeSwap"

	// ProtocolUniswapV3 Uniswap V3 协议名称
	ProtocolUniswapV3 = "UniswapV3Swap"

	// ProtocolUniswapV4 Uniswap V4 协议名称
	ProtocolUniswapV4 = "UniswapV4Swap"
)

// 协议费率
const (
	// UniswapV1StaticFee Uniswap V1 及类似协议的费率（默认 0.30%）
	UniswapV1StaticFee = 0.30

	// UniswapV2StaticFee Uniswap V2 及类似协议的基准费率（默认 0.30%）
	UniswapV2StaticFee = 0.30
)

// 合约 ABI JSON 字符串
const (
	// UniswapV1ExchangeABIJSON Uniswap V1 Exchange 合约 ABI
	// 包含 tokenAddress 方法
	UniswapV1ExchangeABIJSON = `
[
	{
		"constant": true,
		"inputs": [],
		"name": "tokenAddress",
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

	// PairABIJSON Uniswap V2 及类似协议的 Pair 合约 ABI
	// 包含 token0 和 token1 方法
	PairABIJSON = `
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

	// UniswapV3ABIJSON Uniswap V3 协议的 Pool 合约 ABI
	// 包含 token0、token1 和 fee 方法
	UniswapV3ABIJSON = `
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
)

// 常用地址
const (
	// WBNBAddressHex BSC 主网 WBNB 合约地址
	WBNBAddressHex = "0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c"
)

// GetProtocolsConfig 获取协议配置映射
// 返回配置好的协议映射，key 为 Swap Topic 哈希，value 为协议配置
// 注意：此函数需要在 ABI 解析完成后调用，因为配置中包含 ABI 指针
func GetProtocolsConfig(v1ABI, v2ABI, v3ABI *abi.ABI) map[common.Hash]protocolConfig {
	configs := map[common.Hash]protocolConfig{}

	wbnbPtr := addressPtr(common.HexToAddress(WBNBAddressHex))

	// Uniswap V1 (TokenPurchase & EthPurchase)
	if v1ABI != nil {
		v1Config := protocolConfig{
			Name:            ProtocolUniswapV1,
			ContractABI:     v1ABI,
			StaticFee:       UniswapV1StaticFee,
			FeeFromContract: false,
			Token0Method:    "tokenAddress",
			Token1Method:    "",
			FixedToken1:     wbnbPtr,
		}

		v1TokenCfg := v1Config
		v1TokenCfg.SwapTopic = common.HexToHash(UniswapV1TokenPurchaseTopic)
		configs[common.HexToHash(UniswapV1TokenPurchaseTopic)] = v1TokenCfg

		v1EthCfg := v1Config
		v1EthCfg.SwapTopic = common.HexToHash(UniswapV1EthPurchaseTopic)
		configs[common.HexToHash(UniswapV1EthPurchaseTopic)] = v1EthCfg
	}

	// Uniswap V2
	if v2ABI != nil {
		configs[common.HexToHash(UniswapV2SwapTopic)] = protocolConfig{
			Name:            ProtocolUniswapV2Like,
			SwapTopic:       common.HexToHash(UniswapV2SwapTopic),
			ContractABI:     v2ABI,
			StaticFee:       UniswapV2StaticFee,
			FeeFromContract: false,
			Token0Method:    "token0",
			Token1Method:    "token1",
		}
	}

	// Uniswap V3
	if v3ABI != nil {
		configs[common.HexToHash(UniswapV3SwapTopic)] = protocolConfig{
			Name:            ProtocolUniswapV3,
			SwapTopic:       common.HexToHash(UniswapV3SwapTopic),
			ContractABI:     v3ABI,
			StaticFee:       0,
			FeeFromContract: true,
			Token0Method:    "token0",
			Token1Method:    "token1",
		}

		// Uniswap V4（沿用 V3 ABI，若实际 ABI 有差异需单独处理）
		configs[common.HexToHash(UniswapV4SwapTopic)] = protocolConfig{
			Name:            ProtocolUniswapV4,
			SwapTopic:       common.HexToHash(UniswapV4SwapTopic),
			ContractABI:     v3ABI,
			StaticFee:       0,
			FeeFromContract: true,
			Token0Method:    "token0",
			Token1Method:    "token1",
		}
	}

	return configs
}

func addressPtr(addr common.Address) *common.Address {
	return &addr
}
