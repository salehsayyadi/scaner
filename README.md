# Piyazche

A CDN IP scanner that tests connectivity through xray-core proxy or raw ICMP/TCP.

## What it does

Scans a list of IPs (usually Cloudflare ranges) and finds the ones that work best for your specific proxy configuration. For every IP it measures:

- **Realistic ping** — the tunnel is warmed up first, then several requests are sent over the *same* keep-alive connection and the minimum is reported. This matches the "real delay" you see in v2rayN/v2rayNG, instead of the inflated cold-start number (handshake + TLS-fragment delays) the old scanner reported.
- **Packet loss %** — measured from the same sample loop.
- **Upload throughput** (optional) — POSTs a payload through the tunnel so you can find IPs that actually have good upload, not just download.
- **Download throughput** (optional).

It also ships with an **interactive TUI** (just run `piyazche` with no arguments): paste a `vless://` link or an xray JSON config in one place and it pulls out all the parts itself, set speed/threads/everything in a settings tab, give it a range in a dedicated tab, and watch results live.

## Install

### Download pre-built binaries

Download the latest release from the [Releases page](https://github.com/redhatx7/piyazche-scanner/releases).

Available platforms:
- Linux (amd64, arm64)
- Windows (amd64)
- macOS (amd64, arm64 / Apple Silicon)

### Build from source

```bash
go build -ldflags="-checklinkname=0" -o piyazche .
```

> `-checklinkname=0` is required because xray-core's `sing` dependency uses a
> `go:linkname` that the Go 1.23+ linker rejects by default. The `Makefile`
> already includes it (`make build`).

## Usage

### Interactive TUI (no config files needed)

Just run it with no arguments:

```bash
./piyazche
```

This opens a tabbed terminal UI. **Navigation:** you land on the top menu —
use `←/→` (or `Tab`) to pick a tab, `Enter` (or `↓`) to open it, and `Esc` to go
back up to the menu. The tabs:

- **Config** — paste a `vless://` link, a full xray JSON config, or this app's `config.json`; press `Ctrl+O` and it extracts every part automatically.
- **Settings** — `↑/↓` to pick a field (the list scrolls), `←/→` to change it, `Enter` to type a value. Set threads (speed), timeout, ping samples, packet-loss / upload / download tests, SNI, port, and everything else.
- **Range** — type the CIDRs / IPs to scan, one per line (the queued-IP count updates live).
- **Fragment** — set TLS-fragment settings, and press `o` to **auto-optimize**: it tests fragment sizes/zones against your server and applies the best. This runs *separately* from the IP scan, so it never disrupts it.
- **Run** — watch live results, sorted by **quality** (lowest loss first, then latency), with loss / upload / download columns. Press `p` to **pause/resume** mid-scan; `↑/↓` scroll the results.

**Auto-save:** everything you edit (config, settings, ranges) is saved to
`config.json` + `ranges.txt` immediately, and restored next launch — no need to
re-enter anything. `Ctrl+S` also saves manually.

`Ctrl+R` starts/stops a scan from anywhere · `Ctrl+C` quits.

A detailed run log is written to **`piyazche.log`** (every IP result with its
ping / loss / upload, plus scan/pause/stop events) — handy for diagnosing issues.

You can also preload a link: `./piyazche tui --link "vless://..."`.

### CLI

```bash
# Scan straight from a share link — no config.json required
./piyazche --link "vless://..." -s 104.16.0.0/24 -t 16

# Basic scan with a config file
./piyazche -c config.json -s ipv4.txt -t 16

# ICMP ping scan (no proxy, just find reachable IPs)
sudo ./piyazche --scan-mode icmp -s ipv4.txt -t 64

# Check single IP connection
./piyazche -c config.json --check

# Auto-optimize fragment settings
./piyazche -c config.json --fragment-mode auto --test-ip 104.27.68.140
```

## How the xray scan timing works

```
                    Per-IP Test Flow
                    ================

    ┌─────────────────────────────────────────────────────────┐
    │                      For each IP                        │
    └─────────────────────────────────────────────────────────┘
                              │
                              ▼
    ┌─────────────────────────────────────────────────────────┐
    │  1. Generate xray config with target IP + fragment      │
    │  2. Start xray-core instance on random local port       │
    │  3. Wait for xray to be ready (up to 10s)               │
    └─────────────────────────────────────────────────────────┘
                              │
                              ▼
         ┌────────────────────────────────────────┐
         │         Retry Loop (default: 3)        │
         │                                        │
         │  ┌──────────────────────────────────┐  │
         │  │  HTTP GET through SOCKS5 proxy   │  │
         │  │  Target: testUrl (gstatic 204)   │  │
         │  │  Timeout: scan.timeout seconds   │  │
         │  └──────────────────────────────────┘  │
         │                 │                      │
         │     ┌───────────┴───────────┐          │
         │     ▼                       ▼          │
         │  Success                  Fail         │
         │     │                       │          │
         │     │              Retry if attempts   │
         │     │                 remaining        │
         │     │                       │          │
         └─────┼───────────────────────┼──────────┘
               │                       │
               ▼                       ▼
    ┌─────────────────┐     ┌─────────────────────┐
    │ Check latency   │     │ Mark as failed      │
    │ vs maxLatency   │     │ Move to next IP     │
    └─────────────────┘     └─────────────────────┘
               │
       ┌───────┴───────┐
       ▼               ▼
    Within           Exceeds
    limit            limit
       │               │
       ▼               ▼
    Record          Discard
    result          result
```

**Timing parameters:**
- `timeout`: How long to wait for HTTP response (per attempt)
- `maxLatency`: Discard results slower than this (in ms)
- `retries`: Number of attempts before giving up on an IP

Example: With `timeout=4`, `maxLatency=2000`, `retries=3`:
- Each attempt waits up to 4 seconds
- If all 3 attempts fail, IP is marked failed
- If any attempt succeeds but takes >2000ms, result is discarded

## How fragment auto-optimization works

```
                Fragment Optimizer Flow
                =======================

    ┌─────────────────────────────────────────────────────────┐
    │  Test 3 zones in sequence: tlshello, 1-3, 1-5          │
    │  Each zone = different packet fragmentation strategy    │
    └─────────────────────────────────────────────────────────┘
                              │
                              ▼
    ┌─────────────────────────────────────────────────────────┐
    │              For each zone (e.g. tlshello)              │
    │                                                         │
    │  Start with full range:                                 │
    │    size = [10-60]  interval = [10-32]                   │
    └─────────────────────────────────────────────────────────┘
                              │
                              ▼
         ┌────────────────────────────────────────┐
         │      Test Loop (max 20 attempts)       │
         │                                        │
         │  1. Test current size+interval range   │
         │  2. Measure success/failure + latency  │
         └────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
           Success                          Failure
              │                               │
              ▼                               ▼
    ┌─────────────────┐           ┌─────────────────────┐
    │ Save if best    │           │ Shift to unexplored │
    │ latency so far  │           │ region (4 strategies│
    │                 │           │ rotate each fail):  │
    │ Narrow range:   │           │                     │
    │ [10-60]->[17-48]│           │ 0: shift lower      │
    │ (shrink 38%     │           │ 1: shift upper      │
    │  from edges)    │           │ 2: small+slow combo │
    │                 │           │ 3: widen range      │
    └─────────────────┘           └─────────────────────┘
              │                               │
              └───────────────┬───────────────┘
                              │
                              ▼
         ┌────────────────────────────────────────┐
         │           Exit conditions:             │
         │                                        │
         │  - Hit success threshold (50% of max)  │
         │  - 3 consecutive failures + have best  │
         │  - Max attempts reached                │
         └────────────────────────────────────────┘
                              │
                              ▼
    ┌─────────────────────────────────────────────────────────┐
    │  Pick best result across all 3 zones (lowest latency)  │
    │  Apply: packets=zone, length=size, interval=interval   │
    └─────────────────────────────────────────────────────────┘
```

**How narrowing works (step by step):**

Each success shrinks the range by ~38% (19% from each edge), converging toward the sweet spot:

```
Fragment SIZE range narrowing
─────────────────────────────

Step 0 (initial):
    10                                                    60
    ├──────────────────────────────────────────────────────┤
    [==========================================================]

Step 1 (success → narrow):
         17                                          48
          ├──────────────────────────────────────────┤
          [============================================]
    cut ──┘▲                                        ▲└── cut
           └── 19% from left                19% from right

Step 2 (success → narrow):
              22                                39
               ├────────────────────────────────┤
               [================================]

Step 3 (success → narrow):
                  26                       35
                   ├───────────────────────┤
                   [=======================]

Step 4 (converged):
                      28              33
                       ├──────────────┤
                       [==============]  ← sweet spot found


Fragment INTERVAL range narrowing
─────────────────────────────────

Step 0:  [10 ─────────────────────────────────────── 32]
Step 1:  [10 ─────────────────────────────────────── 32]  (same range)
              ↓ narrow on success
Step 1:      [14 ─────────────────────────────── 28]
              ↓ narrow on success
Step 2:          [17 ─────────────────────── 25]
              ↓ narrow on success  
Step 3:              [19 ───────────── 23]
              ↓ converged
Step 4:                  [20 ───── 22]  ← optimal interval
```

**The math:**
- Golden ratio constant: `φ = 0.618`
- Narrow ratio: `(1 - φ) / 2 ≈ 0.19` (19% cut from each side)
- Each success: `newMin = min + range×0.19`, `newMax = max - range×0.19`

**Shift strategies on failure:**
- Rotates through 4 approaches to explore different regions
- Avoids getting stuck in a non-working area

```
Shift strategies visualization (on failure)
───────────────────────────────────────────

Current range that failed:
         [==========]
          20     40

Strategy 0 - Shift Lower (explore smaller values):
    [==========]
     10     30
    ←── moved left

Strategy 1 - Shift Upper (explore larger values):
              [==========]
               30     50
               moved right ──→

Strategy 2 - Small + Slow (conservative combo):
       [======]
        15   30
    smaller range, lower values

Strategy 3 - Widen (expand search area):
    [==================]
     10            50
    ←── wider range ──→
```

## Config parameters

### proxy

| Field | Description |
|-------|-------------|
| `uuid` | Your vless UUID |
| `address` | Server address (domain or IP) |
| `port` | Server port (usually 443) |
| `method` | Security method: `tls` or `reality` |
| `type` | Transport type: `ws`, `xhttp`, `grpc`, `httpupgrade`, `tcp` |

### proxy.tls (when method=tls)

| Field | Description |
|-------|-------------|
| `sni` | Server Name Indication |
| `alpn` | ALPN protocols, e.g. `["http/1.1", "h2"]` |
| `fingerprint` | TLS fingerprint: `chrome`, `firefox`, `safari`, etc. |
| `allowInsecure` | Skip certificate verification (not recommended) |

### proxy.reality (when method=reality)

| Field | Description |
|-------|-------------|
| `publicKey` | Reality public key from server |
| `shortId` | Reality short ID |
| `spiderX` | Spider path (usually `/`) |
| `fingerprint` | uTLS fingerprint |
| `serverName` | SNI for reality (usually a real website like `www.domain.com`) |

### proxy.ws (when type=ws)

| Field | Description |
|-------|-------------|
| `host` | WebSocket Host header |
| `path` | WebSocket path |

### proxy.xhttp (when type=xhttp)

| Field | Description |
|-------|-------------|
| `host` | HTTP host |
| `path` | HTTP path |
| `mode` | Mode: `auto`, `stream`, etc. |

### proxy.grpc (when type=grpc)

| Field | Description |
|-------|-------------|
| `serviceName` | gRPC service name |
| `authority` | gRPC authority (optional) |
| `multiMode` | Enable multi-mode |

### fragment

| Field | Description |
|-------|-------------|
| `enabled` | Enable TLS fragmentation |
| `mode` | `manual` (use settings below) or `auto` (discover optimal) |
| `packets` | Which packets to fragment: `tlshello`, `1-3`, `1-5` |
| `manual.length` | Fragment size range, e.g. `"10-20"` |
| `manual.interval` | Delay between fragments in ms, e.g. `"10-20"` |

### scan

| Field | Description |
|-------|-------------|
| `threads` | Number of concurrent workers |
| `timeout` | HTTP request timeout in seconds |
| `testUrl` | URL to test (default: gstatic 204) |
| `maxLatency` | Max acceptable latency in ms |
| `retries` | Retry count per IP |
| `sampleSize` | IPs to sample per subnet |
| `realisticPing` | Warm the tunnel, then report the min of several keep-alive samples (matches v2rayN). Default `true` |
| `handshakeTest` | Require + time a full cold TLS handshake through the tunnel; a failed handshake fails the IP. Default `true` |
| `pingSamples` | Number of measured requests per IP for ping + loss (default 8) |
| `lossTest` | Compute packet-loss % from the sample loop. A sample that fails **or is slower than `maxLatency`** counts as loss, so laggy IPs are not reported as good (default `true`) |
| `maxLoss` | Drop IPs with loss above this percent (0 = off) |
| `uploadTest` | Measure upload throughput through the tunnel |
| `uploadUrl` | Upload POST endpoint (default Cloudflare `__up`) |
| `uploadBytes` | Bytes to upload per test (default 1,000,000) |
| `minUpload` | Drop IPs with upload below this Mbps (0 = off) |
| `downloadTest` | Measure download throughput through the tunnel |
| `downloadUrl` | Download GET endpoint (default Cloudflare `__down`) |
| `downloadBytes` | Bytes to download per test (default 2,000,000) |
| `minDownload` | Drop IPs with download below this Mbps (0 = off) |

> **Finding upload-capable IPs:** set `uploadTest: true` and (optionally) `minUpload`
> to a Mbps floor. The scan then POSTs through the tunnel for each IP and keeps only
> the ones that actually upload well — useful when your current IPs download fine but
> have no upload. The results table and CSV gain `Upload (Mbps)` / `Loss (%)` columns.

### xray.mux

| Field | Description |
|-------|-------------|
| `enabled` | Enable mux multiplexing |
| `concurrency` | Number of concurrent streams |

## Sample configs

### WebSocket + TLS

```json
{
  "proxy": {
    "uuid": "your-uuid",
    "address": "your-server.com",
    "port": 443,
    "method": "tls",
    "type": "ws",
    "tls": {
      "sni": "your-sni.com",
      "alpn": ["http/1.1"],
      "fingerprint": "chrome"
    },
    "ws": {
      "host": "your-host.com",
      "path": "/ws-path"
    }
  },
  "fragment": {
    "enabled": true,
    "mode": "manual",
    "packets": "tlshello",
    "manual": {
      "length": "10-20",
      "interval": "10-20"
    }
  },
  "scan": {
    "threads": 8,
    "timeout": 4,
    "testUrl": "https://www.gstatic.com/generate_204",
    "maxLatency": 3000,
    "retries": 2
  }
}
```

### WebSocket + Reality

```json
{
  "proxy": {
    "uuid": "your-uuid",
    "address": "your-server-ip",
    "port": 443,
    "method": "reality",
    "type": "ws",
    "reality": {
      "publicKey": "your-public-key",
      "shortId": "your-short-id",
      "spiderX": "/",
      "fingerprint": "chrome",
      "serverName": "www.domain.com"
    },
    "ws": {
      "host": "",
      "path": "/"
    }
  },
  "fragment": {
    "enabled": true,
    "mode": "auto"
  },
  "scan": {
    "threads": 1,
    "timeout": 10,
    "maxLatency": 3000,
    "retries": 2
  }
}
```

### XHTTP + TLS

```json
{
  "proxy": {
    "uuid": "your-uuid",
    "address": "your-server.com",
    "port": 443,
    "method": "tls",
    "type": "xhttp",
    "tls": {
      "sni": "your-sni.com",
      "alpn": ["h2", "http/1.1"],
      "fingerprint": "chrome"
    },
    "xhttp": {
      "host": "your-host.com",
      "path": "/xhttp-path",
      "mode": "auto"
    }
  },
  "fragment": {
    "enabled": true,
    "mode": "manual",
    "packets": "tlshello",
    "manual": {
      "length": "15-30",
      "interval": "10-20"
    }
  },
  "scan": {
    "threads": 8,
    "timeout": 4,
    "maxLatency": 2500,
    "retries": 3
  }
}
```

### XHTTP + Reality

```json
{
  "proxy": {
    "uuid": "your-uuid",
    "address": "your-server-ip",
    "port": 443,
    "method": "reality",
    "type": "xhttp",
    "reality": {
      "publicKey": "your-public-key",
      "shortId": "your-short-id",
      "spiderX": "/",
      "fingerprint": "chrome",
      "serverName": "www.domain.com"
    },
    "xhttp": {
      "host": "",
      "path": "/",
      "mode": "auto"
    }
  },
  "fragment": {
    "enabled": true,
    "mode": "auto"
  },
  "scan": {
    "threads": 1,
    "timeout": 10,
    "maxLatency": 3000,
    "retries": 2
  }
}
```

### gRPC + TLS

```json
{
  "proxy": {
    "uuid": "your-uuid",
    "address": "your-server.com",
    "port": 443,
    "method": "tls",
    "type": "grpc",
    "tls": {
      "sni": "your-sni.com",
      "alpn": ["h2"],
      "fingerprint": "chrome"
    },
    "grpc": {
      "serviceName": "your-grpc-service",
      "multiMode": false
    }
  },
  "fragment": {
    "enabled": true,
    "mode": "manual",
    "packets": "tlshello",
    "manual": {
      "length": "10-20",
      "interval": "10-20"
    }
  },
  "scan": {
    "threads": 8,
    "timeout": 5,
    "maxLatency": 3000,
    "retries": 2
  }
}
```

### gRPC + Reality

```json
{
  "proxy": {
    "uuid": "your-uuid",
    "address": "your-server-ip",
    "port": 443,
    "method": "reality",
    "type": "grpc",
    "reality": {
      "publicKey": "your-public-key",
      "shortId": "your-short-id",
      "spiderX": "/",
      "fingerprint": "chrome",
      "serverName": "www.domain.com"
    },
    "grpc": {
      "serviceName": "grpc-service",
      "multiMode": false
    }
  },
  "fragment": {
    "enabled": true,
    "mode": "auto"
  },
  "scan": {
    "threads": 1,
    "timeout": 10,
    "maxLatency": 3000,
    "retries": 2
  }
}
```

## CLI flags

```
    (no args)        Launch the interactive TUI
    tui              Subcommand: launch the TUI (optionally --link to preload)
-c, --config         Config file path (default: config.json)
-l, --link           vless:// link or JSON config (skips --config)
-s, --subnets        IP list file or CIDR (default: ipv4.txt)
-t, --threads        Worker count (overrides config)
-o, --output         Output format: csv, json
    --max-ips        Limit number of IPs to scan
    --shuffle        Randomize IP order (default: true)
    --top            Show top N results (default: 10)
    --tui            Launch the interactive TUI
    --debug          Print xray config for first IP
    --fragment-mode  Fragment mode: manual, auto, off
    --test-ip        IP for fragment optimization
    --check          Test single connection (required for reality)
    --mux            Enable mux: true, false
    --scan-mode      Scan mode: xray (default), icmp
```

## Notes

- Reality mode requires `--check` flag or `--fragment-mode auto` (scanner mode not supported)
- ICMP scan mode needs root for real ICMP, falls back to TCP connect without root
- Results are saved to `results/` directory as CSV or JSON
- Higher thread count = faster scan but more resource usage
