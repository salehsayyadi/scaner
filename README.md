# Piyazche

A CDN IP scanner that tests connectivity through xray-core proxy or raw ICMP/TCP.

## Scaner 2.0.3 — native graphical application

The new **Scaner** desktop/mobile frontend uses the original Go/Xray engine.
The complete CLI/TUI remains available. Every field in `config.Config` gets
a native editable control; an automated test verifies that no field is omitted.

### نصب در ویندوز — تک‌فایلی

1. از [Releases همین مخزن](https://github.com/salehsayyadi/scaner/releases)
   فایل **`Scaner-Setup-2.0.3.exe`** را دانلود و اجرا کن.
2. برنامه برای کاربر فعلی نصب می‌شود؛ Go، Python، WebView2، فایل Xray یا
   DLL جدا لازم نیست. میان‌بر Start و، در صورت انتخاب، دسکتاپ ساخته می‌شود.
3. برنامه **Scaner** را باز کن. در **Connection** لینک `vless://` یا JSON را
   وارد کن و **Apply connection** را بزن.
4. در **Scan** رنج یا IP بده، یا preset اضافه کن. تمام تنظیمات در
   **Settings** هستند. دکمه‌های **Start / Pause / Stop** همیشه در دسترس‌اند.
5. برای به‌روزرسانی، Setup نسخه جدید را نصب کن؛ مسیر نصب و داده‌های کاربر
   همان می‌مانند.

**نسخه بدون نصب:** `Scaner-2.0.3-windows-amd64.exe` خود برنامه کامل است؛
هیچ فایل config، فونت، IP، DLL یا Xray لازم نیست کنارش بگذاری. presetها و
منابع گرافیکی داخل EXE جاسازی شده‌اند.

**یک فایل برای نصب، نه صفر فایل هنگام کار:** تنظیمات، نتایج و لاگ خودکار
در **`%APPDATA%\Scaner`** ذخیره می‌شوند، نه کنار فایل اجرایی. نصب‌کننده
فایل‌های معمول حذف نصب را نیز مدیریت می‌کند.

Desktop target: **Windows 10/11 x64**, with an OpenGL-compatible graphics driver.
Binaries are not Authenticode-signed, so Windows may display an unknown-publisher
warning. Verify the repository and `SHA256SUMS.txt`; do not disable security
software to install the program.

### Windows quick reference (English)

| File in the release | What it is |
|---|---|
| `Scaner-Setup-2.0.3.exe` | **Installer.** Per-user install, no administrator rights needed. Installs to `%LOCALAPPDATA%\Programs\Scaner`, adds a Start menu entry (`Scaner`) and, if selected, a desktop shortcut. |
| `Scaner-2.0.3-windows-amd64.exe` | **Standalone GUI.** The same program as the installed `Scaner.exe` (identical file). Download and double-click; nothing else is needed beside it. |
| `Scaner-cli-2.0.3-windows-amd64.exe` | Original command-line/TUI program. Run it from `cmd` or PowerShell. |
| `Scaner-2.0.3-source.zip`, `SHA256SUMS.txt` | Source of this release and checksums of every file. |

- **Launch:** Start menu → *Scaner* (installed), or double-click the standalone EXE.
- **Update:** run the newer `Scaner-Setup-<version>.exe`; it replaces the installed copy in place and keeps your data. For the standalone EXE, replace the file.
- **Uninstall:** Settings → Apps → Installed apps → *Scaner* → Uninstall. Your settings in `%APPDATA%\Scaner` are kept; delete that folder to remove them.
- **Data:** `%APPDATA%\Scaner` (`session.json`, results, `piyazche.log`) for both the installed and standalone copies.
- **Architecture:** Windows 10/11 **x64** only. No native ARM64 or 32-bit build. Needs a graphics driver with OpenGL 2.1 or newer; very old or GPU-less virtual machines may not be able to open the window.
- **Signing:** the files are **not code-signed**. SmartScreen may show "Windows protected your PC" or "Unknown publisher". First check the file with `Get-FileHash .\Scaner-Setup-2.0.3.exe -Algorithm SHA256` against `SHA256SUMS.txt`; only if it matches, choose *More info → Run anyway*. Do not disable SmartScreen or antivirus.

### Android — بدون روت

- فایل **`Scaner-2.0.3-android.apk`** در Release قرار می‌گیرد (یک APK برای ARM64 و ARMv7؛
  گوشی‌های x86 پشتیبانی نمی‌شوند). APK شامل خود برنامه و هسته است؛ Termux، Go یا روت لازم نیست.
- APK برای **ARM64 و ARMv7** ساخته و امضا و معماری‌های آن در Actions بررسی
  می‌شود. حداقل نسخه Android در خروجی بررسی APK ثبت می‌شود.
- فایل را از همین مخزن دانلود کن و اجازه نصب را فقط برای برنامه‌ای بده که
  APK را باز می‌کند. داده‌ها در فضای خصوصی برنامه نگه‌داری می‌شوند.
- تست‌های پراکسی و TCP با گوشی معمولی اجرا می‌شوند. ICMP خام دسترسی ویژه
  می‌خواهد؛ fallback اصلی برنامه به **TCP connect** استفاده می‌شود و حالت
  واقعی در رابط نمایش داده می‌شود. TCP، پینگ ICMP واقعی نیست.
- اسکنر VPN سیستمی نیست. هنگام اسکن، برنامه را در پیش‌زمینه نگه دار؛
  Android ممکن است اجرای پس‌زمینه را محدود کند.
- **Copy good IPs / Copy JSON** برای انتقال نتایج کار می‌کنند. فایل‌گزین
  Android ممکن است محدود به فضای خصوصی برنامه باشد؛ کپی/پیست ورودی و
  خروجی به مجوز storage نیاز ندارد.
- بدون کلید اختصاصی، APK با **کلید عمومی تست و گواهی ثابت** امضا می‌شود:
  مناسب نصب شخصی/تست، نه هویت امن تولیدی یا Google Play. فقط از همین مخزن
  دانلود کن. گواهی ثابت اجازه نصب نسخه‌های بعدی این سری روی نسخه قبلی را
  می‌دهد.
- برای امضای اختصاصی این GitHub Actions Secrets را تنظیم کن:
  `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`,
  `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`.
  کلید خصوصی را commit نکن. تغییر از امضای تست به امضای اختصاصی نیازمند
  حذف نصب نسخه قبلی است؛ ابتدا کانفیگ و نتایج را پشتیبان بگیر.

### Where every feature lives

| Original capability | Graphical location |
|---|---|
| VLESS, app/Xray JSON, Base64, file import | Connection |
| TLS/Reality, WS/XHTTP/gRPC/TCP/HTTP upgrade; every transport field | Connection; expandable sections |
| Workers, timeout, retries, sampling, max IPs, shuffle | Settings → Scan & quality |
| Warm ping, handshake, loss samples/filter, upload/download tests and thresholds | Settings → Scan & quality |
| ICMP/TCP scan and single-connection check, including Reality | Scan → Scan mode |
| Manual/auto/off fragment; packets, length, interval, ranges, thresholds, test IP | Settings → TLS fragmentation |
| Independent, cancellable fragment optimizer | Settings → Optimize fragments |
| Mux/concurrency/XUDP/log level | Settings → Xray & multiplexing |
| Cloudflare, Cloudflare London, Fastly and custom ranges | Scan |
| Range scan (generate N random IPs per CIDR, 0 = all) or IP list (scan exactly as entered) | Scan → Targets, IPs per range |
| Per-range host count, queued/tested/good counts, IPs being scanned now | Scan → per-range list and action bar |
| Pause/resume/stop | Scan buttons, kept outside the scrolling form |
| Quality sorting, top-N, good/all filter, all result metrics/errors | Results; horizontal scroll or Details |
| Selected/all IP copy, CSV/JSON export | Results |
| Automatic/intermediate saving, history, logs | Output settings and Activity |
| Every legacy CLI flag and TUI | Optional `Scaner-cli-<version>-windows-amd64.exe`; original root entry |

**Scan targets.** *Range scan* builds the IP list from each CIDR: "IPs per
range" random addresses per range (`0` = every address; the same value as
Settings → IPs per subnet). *IP list* scans the entered addresses exactly as
given — in order, with no sampling or shuffling. *Check connection* tests only
the server address from the Connection tab and does not scan the ranges; the
app warns and offers Proxy scan when ranges are entered. A single run is
limited to 4,000,000 generated IPs.

راهنمای کوتاه: در «Range scan» برنامه از هر رنج به تعداد «IPs per range» آی‌پی
تصادفی می‌سازد (۰ = همه)، و در «IP list» دقیقاً همان آی‌پی‌های واردشده به همان
ترتیب اسکن می‌شوند. «Check connection» فقط آدرس سرور را تست می‌کند، نه رنج‌ها را.

**Speed (3.1.4).** Before starting Xray for an IP, the scanner does a fast TCP
connect to ip:port (Settings → Fast TCP pre-check, on by default; skipped for
QUIC/h3). Dead IPs fail in about 2 s instead of an Xray start plus timeout ×
retries. The log is buffered instead of flushed to disk on every line, the
result table re-sorts only when visible (at most every 0.4–1.5 s), and
intermediate saves are spaced out as results grow.

Keyboard: **Tab / Shift+Tab**, standard entry copy/paste and arrows;
**Ctrl+O** applies pasted connection, **Ctrl+S** saves, **Ctrl+R** starts/stops,
**Ctrl+P** pauses/resumes, **Ctrl+Shift+C** copies the selected IP.

### Build and automatic releases

Go **1.25.6** and a platform C compiler are required to build the GUI, not to run
the downloaded app. The original Xray dependency needs `-checklinkname=0`.

```bash
# Desktop GUI on the target platform (with graphics development libraries)
go build -ldflags="-checklinkname=0 -X main.Version=$(cat VERSION)" -o Scaner ./cmd/scaner

# Preserve the complete original CLI/TUI
CGO_ENABLED=0 go build -ldflags="-checklinkname=0 -X main.Version=$(cat VERSION)" -o piyazche .

# Engine and native-widget tests
go test -tags ci -ldflags=-checklinkname=0 ./...
python -m unittest discover -s tools -p 'test_*.py'
```

The exact Windows MinGW/Inno Setup 6 and Android SDK/NDK/Fyne build commands
are in `.github/workflows/native-release.yml`. Android uses
`bash tools/build_android.sh`. Pillow is an icon-build dependency only.

- Initial native version: **2.0.3**. Each later published update automatically
  increases the patch: **2.0.4**, **2.0.5**, etc.
- Push to **main** to test, build the Windows GUI/installer, attempt Android,
  and publish executable(s), a source ZIP, and SHA256 checksums in Releases.
- Failed builds do not skip a reserved version. Tests and Windows installer
  verification must succeed before publication. If Android fails, its APK is
  explicitly marked unavailable; it does not block Windows publication.
- For a major/minor version, use **Native apps and automatic releases → Run
  workflow** and enter a newer version.
- `.github/scaner-build-status.json` records the real result and run/release links.
  Code committed to the repository is **not** proof a binary was built.
- Actions must be enabled and allowed **contents: write**. Branch rules,
  Actions policies and runner quota can block builds.
- Repository visibility is preserved. Source/release downloads do not make
  a private repository public.

---

## Original CLI / TUI

## What it does

Scans a list of IPs (usually Cloudflare ranges) and finds the ones that work best for your specific proxy configuration. For every IP it measures:

- **Realistic ping** — the tunnel is warmed up first, then several requests are sent over the *same* keep-alive connection and the minimum is reported. This matches the "real delay" you see in v2rayN/v2rayNG, instead of the inflated cold-start number (handshake + TLS-fragment delays) the old scanner reported.
- **Packet loss %** — measured from the same sample loop.
- **Upload throughput** (optional) — POSTs a payload through the tunnel so you can find IPs that actually have good upload, not just download.
- **Download throughput** (optional).

It also ships with an **interactive TUI** (just run `piyazche` with no arguments): paste a `vless://` link or an xray JSON config in one place and it pulls out all the parts itself, set speed/threads/everything in a settings tab, give it a range in a dedicated tab, and watch results live.

## Install

### Download pre-built binaries

Download the latest release from the [Releases page](https://github.com/salehsayyadi/scaner/releases).

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
