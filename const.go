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
	// UniswapV2SwapTopic Uniswap V2 及类似协议的 Swap 事件 Topic
	// 对应事件签名: Swap(address indexed sender, uint amount0In, uint amount1In, uint amount0Out, uint amount1Out, address indexed to)
	UniswapV2SwapTopic = "0xd78ad95fa46c994b6551d0da85fc275fe613a1e06de587873393dff2aea0d903"

	// UniswapV3SwapTopic Uniswap V3 协议的 Swap 事件 Topic
	// 对应事件签名: Swap(address indexed sender, address indexed recipient, int256 amount0, int256 amount1, uint160 sqrtPriceX96, uint128 liquidity, int24 tick)
	UniswapV3SwapTopic = "0xc42079f94a6350d7e6235f29174924f928cc2ac818eb64fed8004e115fbcca67"
)

// 协议名称
const (
	// ProtocolUniswapV2Like Uniswap V2 及类似协议名称
	ProtocolUniswapV2Like = "UniswapV2LikeSwap"

	// ProtocolUniswapV3 Uniswap V3 协议名称
	ProtocolUniswapV3 = "UniswapV3Swap"
)

// 协议费率
const (
	// UniswapV2StaticFee Uniswap V2 及类似协议的固定费率（0.25%）
	UniswapV2StaticFee = 0.25
)

// 合约 ABI JSON 字符串
const (
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

// GetProtocolsConfig 获取协议配置映射
// 返回配置好的协议映射，key 为 Swap Topic 哈希，value 为协议配置
// 注意：此函数需要在 ABI 解析完成后调用，因为配置中包含 ABI 指针
func GetProtocolsConfig(v2ABI, v3ABI *abi.ABI) map[common.Hash]protocolConfig {
	return map[common.Hash]protocolConfig{
		common.HexToHash(UniswapV2SwapTopic): {
			Name:            ProtocolUniswapV2Like,
			SwapTopic:       common.HexToHash(UniswapV2SwapTopic),
			ContractABI:     v2ABI,
			StaticFee:       UniswapV2StaticFee,
			FeeFromContract: false,
		},
		common.HexToHash(UniswapV3SwapTopic): {
			Name:            ProtocolUniswapV3,
			SwapTopic:       common.HexToHash(UniswapV3SwapTopic),
			ContractABI:     v3ABI,
			StaticFee:       0,
			FeeFromContract: true,
		},
	}
}
