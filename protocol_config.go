package main

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// protocolConfig 定义协议相关配置
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
