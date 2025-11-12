# BSC 池子监控器

一个用于监控 BSC（Binance Smart Chain）区块链上 Uniswap V2 和 V3 协议流动性池子发现的工具。

## 功能特性

- 🔍 **实时区块订阅**：通过 WebSocket 订阅 BSC 新区块
- 🏊 **池子发现**：自动发现 Uniswap V2 和 V3 协议的新流动性池子
- ⚡ **并发处理**：使用 goroutine 并发处理交易，提高处理效率
- 📊 **池子信息**：获取池子的 token0、token1 和费率信息
- 🌐 **HTTP API**：提供简单的 HTTP 接口用于健康检查

## 支持的协议

- **Uniswap V2 Like**：支持所有基于 Uniswap V2 的 DEX（如 PancakeSwap）
- **Uniswap V3**：支持 Uniswap V3 协议

## 环境要求

- Go 1.18 或更高版本
- 可访问的 BSC WebSocket 节点（默认使用公共节点）

## 安装

1. 克隆或下载项目代码

2. 安装依赖：
```bash
go mod download
```

## 运行

### 方式一：直接运行

```bash
go run .
```

### 方式二：编译后运行

```bash
# 编译
go build -o claam_go_v2 .

# 运行
./claam_go_v2
```

### 方式三：使用自定义 WebSocket 节点

如果需要使用自定义的 BSC WebSocket 节点，可以修改 `const.go` 中的 `DefaultBSCWssURL` 常量，或通过环境变量设置（需要修改代码支持）。

## 使用说明

1. **启动服务**：运行程序后，会自动：
   - 启动 HTTP 服务器（默认端口 8080）
   - 在后台协程中启动区块监控

2. **查看输出**：程序会在控制台输出：
   - 新区块信息（高度、哈希、时间戳、矿工）
   - 交易总数
   - 发现的新池子信息（协议、地址、token0、token1、费率）
   - 处理耗时

3. **API 接口**：
   - `GET /ping`：返回 `{"message": "pong"}`

## 项目结构

```
.
├── main.go           # 主程序入口，启动 Gin 服务器和监控协程
├── pool_monitor.go   # 池子监控器核心逻辑
├── const.go          # 常量定义（协议配置、ABI、WebSocket URL 等）
├── utils.go          # 工具函数（十六进制转换、合约调用等）
├── go.mod            # Go 模块依赖
├── go.sum            # 依赖校验和
└── README.md         # 本文件
```

## 代码说明

### 主要组件

- **PoolMonitor**：池子监控器结构体，负责区块订阅和池子发现
- **NewPoolMonitor**：创建监控器实例
- **Process**：主处理循环，订阅区块并处理
- **discoverPoolsFromTransactions**：并发扫描交易发现新池子
- **inspectPool**：检查并解析池子信息

### 常量定义（const.go）

- **DefaultBSCWssURL**：BSC WebSocket 节点地址
- **UniswapV2SwapTopic / UniswapV3SwapTopic**：协议 Swap 事件 Topic 哈希
- **ProtocolUniswapV2Like / ProtocolUniswapV3**：协议名称常量
- **UniswapV2StaticFee**：Uniswap V2 固定费率
- **PairABIJSON / UniswapV3ABIJSON**：合约 ABI JSON 字符串
- **GetProtocolsConfig**：根据 ABI 生成协议配置映射

### 工具函数（utils.go）

- **HexToUint64**：十六进制字符串转 uint64
- **HexToBigInt**：十六进制字符串转 *big.Int
- **CallTokenAddress**：调用合约获取代币地址
- **CallPoolFee**：调用合约获取池子费率

## 示例输出

```
订阅成功，订阅ID: 0x5b09d205651be1262a3de61c6fda51e8892f2a8a

新区块:
高度: 0x40cd643 (十进制: 67950147)
哈希: 0xfc953c8194d6e358ac4110f76bfbcdc500faed5e9b9179f0286a01e73df5c050
时间戳: 0x6914b0b2 (UTC: 2025-11-12 16:07:14 +0000 UTC)
矿工: 0x9f1b7fae54be07f4fee34eb1aacb39a1f7b6fc92
交易总数: 163
  [新池子] 协议: UniswapV3Swap 地址: 0x47a90A2d92A8367A91EfA1906bFc8c1E05bf10c4 token0: 0x55d398326f99059fF775485246999027B3197955 token1: 0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c fee: 0.0100%
处理耗时: 2.345s
```

## 注意事项

1. **网络连接**：确保能够访问 BSC WebSocket 节点
2. **性能**：处理大量交易时，并发处理会消耗较多资源
3. **重连机制**：程序支持自动重连，连接断开时会自动恢复
4. **已知池子**：程序会缓存已发现的池子，避免重复处理

## 开发

### 添加新协议支持

1. **在 `const.go` 中添加协议常量**：
   - 添加 Swap Topic 哈希常量
   - 添加协议名称常量
   - 添加协议费率常量（如果适用）
   - 添加合约 ABI JSON 常量

2. **更新 `GetProtocolsConfig` 函数**：
   在 `const.go` 的 `GetProtocolsConfig` 函数中添加新协议配置：

```go
common.HexToHash(新协议SwapTopic): {
    Name:            新协议名称常量,
    SwapTopic:       common.HexToHash(新协议SwapTopic),
    ContractABI:     新协议ABI指针,
    StaticFee:       固定费率（如果适用，否则为0）,
    FeeFromContract: 是否从合约读取费率,
},
```

3. **在 `NewPoolMonitor` 中解析新协议 ABI**：
   在 `pool_monitor.go` 的 `NewPoolMonitor` 函数中解析新协议的 ABI，并将 ABI 指针传递给 `GetProtocolsConfig` 函数。

## 许可证

本项目仅供学习和研究使用。

