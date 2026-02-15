# OCG Process Manager

OCG 进程管理器，统一管理 embedding、agent、gateway 三个进程的启动与关闭。

## 快速开始

```bash
# 启动所有服务
./bin/ocg start

# 查看状态
./bin/ocg status

# 停止所有服务
./bin/ocg stop

# 重启
./bin/ocg restart
```

## 命令

### start

依次启动 embedding → agent → gateway，等待所有服务就绪后 **ocg 进程退出**。

```bash
./bin/ocg start [options]
```

**选项：**

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `--config` | `./env.config` | 配置文件路径 |
| `--pid-dir` | `/tmp/ocg` | PID 文件目录 |

**流程：**

1. 启动 embedding 服务
2. 等待 embedding health check 通过
3. 启动 agent 服务
4. 等待 agent socket 就绪
5. 启动 gateway 服务
6. 等待 gateway health check 通过
7. ocg 进程退出

### stop

依次停止 gateway → agent → embedding，每个进程采用**逐级信号**关闭：

```
SIGTERM (3s) → SIGINT (3s) → SIGKILL
```

```bash
./bin/ocg stop [options]
```

**选项：**

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `--pid-dir` | `/tmp/ocg` | PID 文件目录 |

### status

显示各进程运行状态和健康检查结果。

```bash
./bin/ocg status [options]
```

**输出示例：**

```
embedding  running (pid 7963)
agent      running (pid 7983)
gateway    running (pid 7995)
embedding health: true
gateway health: true
```

**选项：**

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `--config` | `./env.config` | 配置文件路径 |
| `--pid-dir` | `/tmp/ocg` | PID 文件目录 |

### restart

等价于 `stop` + `start`。

```bash
./bin/ocg restart [options]
```

## 文件结构

```
/tmp/ocg/
├── ocg-embedding.pid   # embedding PID
├── ocg-agent.pid       # agent PID
├── ocg-gateway.pid    # gateway PID
└── logs/
    ├── embedding.log
    ├── agent.log
    └── gateway.log
```

## 配置文件

默认读取 `env.config`，支持以下方式查找：

1. `--config` 指定路径
2. 当前目录 `env.config`
3. 可执行文件同目录 `env.config`
4. 可执行文件父目录 `env.config`

**关键配置项：**

| 变量 | 说明 |
|------|------|
| `EMBEDDING_SERVER_URL` | embedding 服务地址 |
| `OPENCLAW_AGENT_SOCK` | agent Unix socket 路径 |
| `OPENCLAW_PORT` | gateway 端口 (默认 55003) |
| `OPENCLAW_UI_TOKEN` | Web UI 认证 token |

## 健康检查

- **embedding**: `http://localhost:50000/health`
- **gateway**: `http://localhost:55003/health` (需 token)

## 与 Gateway 的关系

- **旧版本**: `ocg-gateway` 自拉起 agent/embedding
- **新版本 (ocg)**: ocg 统一管理生命周期，gateway 只负责连接 agent

Gateway 启动时不再自动启动其他服务，需要先运行 `ocg start`。

## 故障排查

### 端口占用

```
listen tcp 0.0.0.0:50000: bind: address already in use
```

解决：先停掉占用进程或修改 `env.config` 中的端口。

### 服务启动超时

检查日志：

```bash
tail -f /tmp/ocg/logs/embedding.log
tail -f /tmp/ocg/logs/agent.log
tail -f /tmp/ocg/logs/gateway.log
```

### PID 文件残留

如果进程异常退出，手动清理：

```bash
rm /tmp/ocg/*.pid
```
