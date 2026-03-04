package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type SyncStatus struct {
	IsSyncing     bool
	CurrentBlock  string
	HighestBlock  string
	StartingBlock string
}

type NodeInfo struct {
	URL         string
	ChainID     string
	LatestBlock string
	LatestHash  string
	SafeBlock   string
	FinalBlock  string
	Syncing     string
	Version     string
	PeerCount   string
	Error       string
	UpdatedAt   time.Time
}

const (
	idChainID    = 1
	idLatest     = 2
	idSafe       = 3
	idFinalized  = 4
	idSyncing    = 5
	idVersion    = 6
	idPeerCount  = 7
)

func queryNode(url string, timeout time.Duration) NodeInfo {
	info := NodeInfo{
		URL:       url,
		UpdatedAt: time.Now(),
	}

	requests := []RPCRequest{
		{JSONRPC: "2.0", Method: "eth_chainId", Params: []interface{}{}, ID: idChainID},
		{JSONRPC: "2.0", Method: "eth_getBlockByNumber", Params: []interface{}{"latest", false}, ID: idLatest},
		{JSONRPC: "2.0", Method: "eth_getBlockByNumber", Params: []interface{}{"safe", false}, ID: idSafe},
		{JSONRPC: "2.0", Method: "eth_getBlockByNumber", Params: []interface{}{"finalized", false}, ID: idFinalized},
		{JSONRPC: "2.0", Method: "eth_syncing", Params: []interface{}{}, ID: idSyncing},
		{JSONRPC: "2.0", Method: "web3_clientVersion", Params: []interface{}{}, ID: idVersion},
		{JSONRPC: "2.0", Method: "net_peerCount", Params: []interface{}{}, ID: idPeerCount},
	}

	body, err := json.Marshal(requests)
	if err != nil {
		info.Error = err.Error()
		return info
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		info.Error = err.Error()
		return info
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		info.Error = fmt.Sprintf("request failed: %v", err)
		return info
	}
	defer resp.Body.Close()

	var responses []RPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&responses); err != nil {
		info.Error = fmt.Sprintf("decode failed: %v", err)
		return info
	}

	respByID := make(map[int]*RPCResponse)
	for i := range responses {
		respByID[responses[i].ID] = &responses[i]
	}

	if r, ok := respByID[idChainID]; ok && r.Error == nil {
		var hexStr string
		if err := json.Unmarshal(r.Result, &hexStr); err == nil {
			info.ChainID = fmt.Sprintf("%d", hexToInt64(hexStr))
		}
	}

	if r, ok := respByID[idLatest]; ok && r.Error == nil {
		info.LatestBlock = extractBlockNumber(r.Result)
		info.LatestHash = extractBlockHash(r.Result)
	}

	if r, ok := respByID[idSafe]; ok && r.Error == nil {
		info.SafeBlock = extractBlockNumber(r.Result)
	}

	if r, ok := respByID[idFinalized]; ok && r.Error == nil {
		info.FinalBlock = extractBlockNumber(r.Result)
	}

	if r, ok := respByID[idSyncing]; ok && r.Error == nil {
		var boolVal bool
		if err := json.Unmarshal(r.Result, &boolVal); err == nil {
			if boolVal {
				info.Syncing = "syncing"
			} else {
				info.Syncing = "synced"
			}
		} else {
			var syncObj map[string]interface{}
			if err := json.Unmarshal(r.Result, &syncObj); err == nil {
				info.Syncing = "syncing"
			} else {
				info.Syncing = "unknown"
			}
		}
	}

	if r, ok := respByID[idVersion]; ok && r.Error == nil {
		var ver string
		if err := json.Unmarshal(r.Result, &ver); err == nil {
			info.Version = ver
		}
	}

	if r, ok := respByID[idPeerCount]; ok && r.Error == nil {
		var hexStr string
		if err := json.Unmarshal(r.Result, &hexStr); err == nil {
			info.PeerCount = fmt.Sprintf("%d", hexToInt64(hexStr))
		}
	}

	if info.LatestBlock == "" {
		info.LatestBlock = "N/A"
	}
	if info.LatestHash == "" {
		info.LatestHash = "N/A"
	}
	if info.SafeBlock == "" {
		info.SafeBlock = "N/A"
	}
	if info.FinalBlock == "" {
		info.FinalBlock = "N/A"
	}
	if info.Syncing == "" {
		info.Syncing = "N/A"
	}
	if info.Version == "" {
		info.Version = "N/A"
	}
	if info.PeerCount == "" {
		info.PeerCount = "N/A"
	}
	if info.ChainID == "" {
		info.ChainID = "N/A"
	}

	return info
}

type blockResult struct {
	Number string `json:"number"`
	Hash   string `json:"hash"`
}

func extractBlockNumber(raw json.RawMessage) string {
	if raw == nil {
		return "N/A"
	}
	var block blockResult
	if err := json.Unmarshal(raw, &block); err != nil {
		return "N/A"
	}
	if block.Number == "" {
		return "N/A"
	}
	return fmt.Sprintf("%d", hexToInt64(block.Number))
}

func extractBlockHash(raw json.RawMessage) string {
	if raw == nil {
		return "N/A"
	}
	var block blockResult
	if err := json.Unmarshal(raw, &block); err != nil {
		return "N/A"
	}
	h := block.Hash
	if len(h) < 11 {
		return h
	}
	return h[:6] + "..." + h[len(h)-4:]
}

func hexToInt64(hex string) int64 {
	if len(hex) < 2 {
		return 0
	}
	if hex[:2] == "0x" || hex[:2] == "0X" {
		hex = hex[2:]
	}
	var result int64
	for _, c := range hex {
		result <<= 4
		switch {
		case c >= '0' && c <= '9':
			result |= int64(c - '0')
		case c >= 'a' && c <= 'f':
			result |= int64(c-'a') + 10
		case c >= 'A' && c <= 'F':
			result |= int64(c-'A') + 10
		}
	}
	return result
}
