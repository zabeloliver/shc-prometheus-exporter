package shcJsonRpc

import "github.com/zabeloliver/shc-api/shcStructs"

type JsonRpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type JsonRPC struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type JsonRPCResult struct {
	Jsonrpc string `json:"jsonrpc"`
	Result  string `json:"result"`
}

type PollResult struct {
	Jsonrpc string                   `json:"jsonrpc"`
	Result  []shcStructs.DeviceEvent `json:"result"`
	Error   JsonRpcError             `json:"error,omitempty"`
}
