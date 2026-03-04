# eth-watch

A terminal dashboard for monitoring multiple Ethereum (or EVM-compatible) RPC nodes in real time.

![demo](https://raw.githubusercontent.com/shidaxi/eth-watch/main/assets/demo.png)

## Features

- **Batch RPC queries** — all data for each node is fetched in a single HTTP request using JSON-RPC batch mode, minimizing latency
- **Live TUI dashboard** — built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), the table refreshes automatically at a configurable interval
- **Per-column sorting** — sort by any column with `←`/`→` to select and `Space`/`Enter` to toggle direction, just like `htop`
- **Block lag indicators** — Safe and Finalized block heights show their lag behind Latest in red `(-N)`
- **Multi-node concurrency** — all nodes are queried in parallel

## Columns

| Column    | Source RPC method               | Description                                      |
|-----------|---------------------------------|--------------------------------------------------|
| URL       | —                               | RPC endpoint URL                                 |
| ChainID   | `eth_chainId`                   | Chain ID (decimal)                               |
| Latest    | `eth_getBlockByNumber("latest")`| Latest block height                              |
| Hash      | `eth_getBlockByNumber("latest")`| Latest block hash (first 4 + last 4 hex chars)   |
| Safe      | `eth_getBlockByNumber("safe")`  | Safe head height, with lag vs Latest             |
| Finalized | `eth_getBlockByNumber("finalized")` | Finalized head height, with lag vs Latest    |
| Syncing   | `eth_syncing`                   | `synced` (green) or `syncing` (yellow)           |
| Peers     | `net_peerCount`                 | Connected peer count                             |
| Version   | `web3_clientVersion`            | Client software version string                   |
| Updated   | —                               | Time since last successful query                 |

## Installation

### Pre-built binaries

Download the latest binary for your platform from the [Releases](../../releases) page.

### Build from source

```bash
git clone https://github.com/shidaxi/eth-watch.git
cd eth-watch
go build -o eth-watch .
```

Requires Go 1.21+.

## Configuration

Create a `config.yaml` file (default path: `./config.yaml`):

```yaml
# Poll interval in seconds (default: 12)
interval: 12

rpcs:
  - https://eth.drpc.org
  - https://rpc.ankr.com/eth
  - https://cloudflare-eth.com
```

| Field      | Type     | Default | Description                        |
|------------|----------|---------|------------------------------------|
| `interval` | integer  | `12`    | Seconds between each refresh cycle |
| `rpcs`     | string[] | —       | List of RPC endpoint URLs to monitor |

## Usage

```bash
# Use default config path (./config.yaml)
./eth-watch

# Specify a custom config file
./eth-watch -config /path/to/config.yaml
```

## Keyboard Controls

| Key              | Action                                      |
|------------------|---------------------------------------------|
| `←` / `h`        | Move sort column left                       |
| `→` / `l`        | Move sort column right                      |
| `Space` / `Enter`| Toggle sort direction (ascending/descending)|
| `a`              | Sort ascending                              |
| `d`              | Sort descending                             |
| `1` – `9`        | Jump to column N directly                   |
| `q` / `Ctrl+C`   | Quit                                        |

## License

MIT
