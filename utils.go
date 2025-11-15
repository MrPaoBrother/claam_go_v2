package main

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// HexToUint64 将十六进制字符串转换为 uint64
// 参数 hexStr 必须是 "0x" 开头的十六进制字符串
// 返回转换后的 uint64 值
func HexToUint64(hexStr string) uint64 {
	var n uint64
	fmt.Sscanf(hexStr, "0x%x", &n)
	return n
}

// HexToBigInt 将十六进制字符串转换为 *big.Int
// 参数 hexStr 可以是 "0x" 开头或纯十六进制字符串
// 返回转换后的 *big.Int，如果转换失败则返回错误
func HexToBigInt(hexStr string) (*big.Int, error) {
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

// CallTokenAddress 调用合约的 token0 或 token1 方法，获取代币地址
// 参数 ctx 是上下文，contract 是绑定的合约实例，method 是方法名（"token0" 或 "token1"）
// 返回代币地址，如果调用失败则返回错误
func CallTokenAddress(ctx context.Context, contract *bind.BoundContract, method string) (common.Address, error) {
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

// CallPoolFee 调用合约的 fee 方法，获取池子费率
// 参数 ctx 是上下文，contract 是绑定的合约实例
// 返回费率百分比（例如 0.3 表示 0.3%），如果调用失败则返回错误
// 注意：Uniswap V3 的 fee 返回单位为 1e-6，需要除以 1e4 转换为百分比
func CallPoolFee(ctx context.Context, contract *bind.BoundContract) (float64, error) {
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

// CallGetReserves 调用合约的 getReserves 方法，获取池子储备量
// 参数 ctx 是上下文，contract 是绑定的合约实例
// 返回 reserve0、reserve1 和 blockTimestampLast，如果调用失败则返回错误
// 注意：此方法适用于 Uniswap V2 及类似协议的 Pair 合约
func CallGetReserves(ctx context.Context, contract *bind.BoundContract) (*big.Int, *big.Int, error) {
	var raw []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &raw, "getReserves"); err != nil {
		return nil, nil, err
	}
	if len(raw) != 3 {
		return nil, nil, fmt.Errorf("unexpected getReserves return length %d", len(raw))
	}

	var reserve0, reserve1 *big.Int
	switch v := raw[0].(type) {
	case *big.Int:
		reserve0 = v
	case uint64:
		reserve0 = big.NewInt(int64(v))
	case uint32:
		reserve0 = big.NewInt(int64(v))
	default:
		return nil, nil, fmt.Errorf("unexpected reserve0 type %T", raw[0])
	}

	switch v := raw[1].(type) {
	case *big.Int:
		reserve1 = v
	case uint64:
		reserve1 = big.NewInt(int64(v))
	case uint32:
		reserve1 = big.NewInt(int64(v))
	default:
		return nil, nil, fmt.Errorf("unexpected reserve1 type %T", raw[1])
	}

	return reserve0, reserve1, nil
}

// CallERC20BalanceOf 调用 ERC20 合约的 balanceOf 方法，获取指定地址的代币余额
// 参数 ctx 是上下文，client 是以太坊客户端，tokenAddr 是代币合约地址，ownerAddr 是持有者地址
// 返回代币余额（*big.Int），如果调用失败则返回错误
func CallERC20BalanceOf(ctx context.Context, client *ethclient.Client, tokenAddr, ownerAddr common.Address) (*big.Int, error) {
	// 解析 ERC20 ABI
	erc20ABI, err := abi.JSON(strings.NewReader(ERC20ABIJSON))
	if err != nil {
		return nil, fmt.Errorf("解析 ERC20 ABI 失败: %w", err)
	}

	// 使用 bind.NewBoundContract 绑定合约
	contract := bind.NewBoundContract(tokenAddr, erc20ABI, client, client, client)

	// 调用 balanceOf 方法
	var raw []interface{}
	if err := contract.Call(&bind.CallOpts{Context: ctx}, &raw, "balanceOf", ownerAddr); err != nil {
		return nil, fmt.Errorf("调用 balanceOf 失败: %w", err)
	}

	if len(raw) != 1 {
		return nil, fmt.Errorf("unexpected balanceOf return length %d", len(raw))
	}

	// 解析返回值
	var balance *big.Int
	switch v := raw[0].(type) {
	case *big.Int:
		balance = v
	case uint64:
		balance = big.NewInt(int64(v))
	case uint32:
		balance = big.NewInt(int64(v))
	default:
		return nil, fmt.Errorf("unexpected balanceOf return type %T", raw[0])
	}

	return balance, nil
}
