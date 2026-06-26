# PolyProxy

> 一个带有**上游代理池**、**免费代理采集**、**动态轮换**、**自动入池管理**和 **Web 仪表盘**的本地 HTTP + SOCKS5 代理 — 用 Go 编写，单静态二进制文件，支持 Linux / macOS / Windows。

灵感来源于 Clash / Clash Verge 的 *Connections* 面板。PolyProxy 给你一个本地端口，将流量分发到 N 个上游代理，并实时展示流经的每一个字节。外加：自动从公开源采集、验证、轮换免费代理。

```
┌────────────────────────┐      ┌─────────────────────────────┐
│  你的应用 / curl /     │      │     PolyProxy               │
│  浏览器 / 系统代理     │ ───▶ │  ┌──────────┐  ┌────────┐  │
│                        │      │  │ HTTP     │  │ Web UI │  │
│  127.0.0.1:7890 (HTTP) │      │  │ :7890    │  │ :9090  │  │
│  127.0.0.1:7891 (SOCKS5)│     │  ├──────────┤  └────────┘  │
└────────────────────────┘      │  │ SOCKS5   │              │
                                │  │ :7891    │              │
                                │  └────┬─────┘              │
                                │       │                    │
                                │   ┌───▼──────────┐         │
                                │   │ 代理池       │         │
                                │   │ random / rr / │         │
                                │   │ hash / name  │         │
                                │   └─┬───┬───┬─────┘         │
                                └─────┼───┼───┼───────────────┘
                                      │   │   │
                          ┌───────────┘   │   └────────────┐
                          ▼               ▼                ▼
                    upstream-A       upstream-B        upstream-C
                     (HTTP)           (SOCKS5)         (direct)
```

---

## 特性

| 能力 | 详情 |
|---|---|
| **本地代理端口** | HTTP（`7890`，支持 `CONNECT` 和普通 `GET` 转发）+ SOCKS5（`7891`） |
| **代理池** | 自由混合 `direct`、`http`、`socks5` 上游 |
| **选择策略** | `random`（默认）、`round-robin`、`hash`（按目标主机粘性）、`name`（首个） |
| **按请求指定** | 代理认证用户名 = 上游名称 → 强制该请求使用指定上游 |
| **连接故障转移** | 自动跳过不健康上游，回退到下一个可用代理 |
| **实时连接** | 每个活跃隧道：ID、主机、目标、代理、来源、上传/下载字节数、开始时间、持续时长 |
| **Web 仪表盘** | 每 1 秒自动刷新，可按主机/目标/代理过滤，关闭单个或全部，深色主题，中/EN 双语 |
| **免费代理采集** | 内置稻壳代理、谷德代理等公开源 — 一键获取免费代理列表 |
| **异步验证** | 并发 TCP + HTTP 验证，带进度追踪 |
| **动态代理轮换** | 按可配置间隔（最低 10s）自动采集→验证循环 |
| **自动入池** | 每轮验证完成后自动将有效代理加入代理池 |
| **REST API** | 完整的连接、代理、池管理、动态控制 API |
| **跨平台** | Linux（amd64 / arm64 / armv7）、macOS（amd64 / arm64）、Windows（amd64 / arm64）— 单静态二进制，零运行时依赖 |
| **可嵌入** | UI 资源通过 `go:embed` 编译进二进制 — 无文件系统依赖 |
| **可配置** | 按用户路径的 YAML 配置；可通过 CLI 参数覆盖 |
| **健康检查** | 可选的后台 TCP 拨号健康检查，标记不可用上游 |
| **低资源占用** | ~6 MB 二进制，100 个活跃连接时 ~12 MB RSS |

---

## 快速开始（60 秒）

### 下载预构建二进制

前往 [Releases](https://github.com/Samsepik9/PolyProxy/releases) 下载对应平台的二进制：

| 平台 | 二进制文件 |
|---|---|
| macOS (Apple Silicon) | `polyproxy-darwin-arm64` |
| macOS (Intel) | `polyproxy-darwin-amd64` |
| Linux (x86_64) | `polyproxy-linux-amd64` |
| Linux (ARM64) | `polyproxy-linux-arm64` |
| Linux (ARMv7) | `polyproxy-linux-arm` |
| Windows (x86_64) | `polyproxy-windows-amd64.exe` |
| Windows (ARM64) | `polyproxy-windows-arm64.exe` |

### 或从源码构建

```bash
git clone https://github.com/Samsepik9/PolyProxy.git
cd PolyProxy

# 为当前主机构建
./scripts/build.sh

# 或为所有平台构建
./scripts/build.sh all
```

### 运行

```bash
# 使用示例配置启动
./bin/polyproxy-darwin-arm64 -config configs/config.example.yaml

# 打开仪表盘
open http://127.0.0.1:9090
```

然后在另一个终端中：

```bash
# 作为 HTTP 代理使用
curl -x http://127.0.0.1:7890 https://api.ipify.org

# 作为 SOCKS5 代理使用
curl --proxy socks5h://127.0.0.1:7891 https://api.ipify.org
```

---

## 配置

配置文件是纯 YAML。完整示例见：[`configs/config.example.yaml`](configs/config.example.yaml)。

```yaml
server:
  http_listen:   "127.0.0.1:7890"
  socks5_listen: "127.0.0.1:7891"
  api_listen:    "127.0.0.1:9090"
  api_enable:    true

pool:
  strategy: random
  health_check: true
  proxies:
    - { name: direct, type: direct }
    - name: us-1
      type: http
      server: 1.2.3.4
      port: 8080

# 免费代理采集（可选）
freeproxy:
  enabled: true
  test_urls:
    - "http://myip.ipip.net"
    - "http://www.baidu.com"
  crawl_timeout: 30
  timeout: 8
  concurrency: 50
```

### 配置路径

| 系统 | 路径 |
|---|---|
| Linux | `~/.config/PolyProxy/config.yaml` |
| macOS | `~/Library/Application Support/PolyProxy/config.yaml` |
| Windows | `%APPDATA%\PolyProxy\config.yaml` |

---

## Web 仪表盘

在浏览器中打开 **`http://127.0.0.1:9090`**。

### 标签页

- **连接** — 实时隧道列表，可过滤、逐个关闭或一键全部关闭
- **代理采集** — 从公开源获取免费代理、验证、加入代理池
- **代理池** — 查看/管理上游代理、切换策略、保存配置
- **运行日志** — 实时操作日志

### 代理采集功能

| 按钮 | 功能 |
|---|---|
| 🔍 获取代理 | 从所有内置源采集代理 |
| ✅ 验证代理 | 并发验证已采集的代理（带进度条） |
| 🔄 动态代理 | 启动/停止定时采集→验证循环（可配置间隔） |
| 🤖 自动入池 | 开关：开启后每轮动态循环自动将有效代理加入代理池 |
| 📥 移入代理池 | 手动将勾选的代理加入代理池 |

### 优先展示

**稻壳代理 (docip)** 和 **谷德代理 (goodips)** 来源的代理优先显示。结果每页 50 条，支持智能翻页。

---

## 选择策略

| 策略 | 行为 | 最适合 |
|---|---|---|
| `random` | 在健康上游中均匀随机选择 | 分散负载，规避按 IP 的速率限制 |
| `round-robin` | 按顺序循环上游 | 均匀分配 |
| `hash` | `FNV32a(host) % len(pool)` — 按目标主机粘性 | 保持会话在同一出口 IP |
| `name` | 始终使用第一个健康上游 | 调试，"始终使用我的私有代理" |

---

## 按请求指定上游

将上游名称编码为代理认证用户名：

```bash
# 强制使用 "us-1"
curl -x http://us-1:any@127.0.0.1:7890 https://api.ipify.org

# 强制 "direct"（绕过所有上游）
curl -x http://direct:any@127.0.0.1:7890 https://api.ipify.org

# SOCKS5 变体
curl --proxy-user 'jp-1:any' --proxy socks5h://127.0.0.1:7891 https://api.ipify.org
```

---

## REST API

| 方法 | 路径 | 描述 |
|---|---|---|
| `GET` | `/api/healthz` | 存活探针 |
| `GET` | `/api/connections` | 活跃连接快照 |
| `DELETE` | `/api/connections` | 关闭所有连接 |
| `DELETE` | `/api/connections/:id` | 关闭单个连接 |
| `GET` | `/api/proxies` | 上游列表及健康状态 |
| `DELETE` | `/api/proxies/:name` | 从池中移除代理 |
| `GET` | `/api/stats` | 聚合计数器 |
| `POST` | `/api/proxies/fetch` | 采集免费代理 |
| `POST` | `/api/proxies/validate` | 验证已缓存的代理 |
| `GET` | `/api/proxies/validate/:id` | 验证进度 |
| `POST` | `/api/proxies/dynamic` | 启动/停止动态循环 |
| `GET` | `/api/proxies/dynamic` | 动态循环状态 |
| `POST` | `/api/proxies/auto-pool` | 切换自动入池 |
| `POST` | `/api/pool/add` | 添加代理到池 |
| `PUT` | `/api/pool/strategy` | 更改选择策略 |
| `POST` | `/api/pool/save` | 保存池到配置文件 |
| `GET` | `/api/logs` | 操作日志 |

---

## 项目结构

```
.
├── cmd/proxypool/          # 入口
├── internal/
│   ├── api/                # REST API + 动态运行器
│   ├── config/             # YAML 配置加载
│   ├── conntrack/          # 连接追踪
│   ├── freeproxy/          # 免费代理采集与验证
│   ├── pool/               # 上游代理池 + 健康检查
│   ├── proxy/              # HTTP/SOCKS5 代理服务器 + 故障转移
│   └── web/                # 嵌入式 Web 仪表盘
├── configs/                # 示例配置
├── scripts/                # 构建与启动脚本
└── .github/workflows/      # CI（3 系统）+ Release（7 平台）
```

---

## 开发

```bash
# 前置条件：Go 1.24+

# 构建
go build -o bin/polyproxy ./cmd/proxypool

# 运行测试
go test ./... -v

# 交叉编译所有平台
./scripts/build.sh all
```

---

## 许可证

MIT — 详见 [LICENSE](LICENSE)。

---

[English Documentation](README.md)
