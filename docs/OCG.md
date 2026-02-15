# OCG Process Manager

OCG process manager, unified management of embedding, agent, and gateway lifecycle.

## Quick Start

```bash
# Start all services
./bin/ocg start

# Check status
./bin/ocg status

# Stop all services
./bin/ocg stop

# Restart
./bin/ocg restart
```

## Commands

### start

Starts embedding → agent → gateway in order, waits for all services to be ready, then **exits**.

```bash
./bin/ocg start [options]
```

**Options:**

| Option | Default | Description |
|--------|---------|-------------|
| `--config` | `./env.config` | config file path |
| `--pid-dir` | `/tmp/ocg` | PID file directory |

**Flow:**

1. Start embedding service
2. Wait for embedding health check
3. Start agent service
4. Wait for agent socket
5. Start gateway service
6. Wait for gateway health check
7. ocg process exits

### stop

Stops gateway → agent → embedding in order, using **escalating signals**:

```
SIGTERM (3s) → SIGINT (3s) → SIGKILL
```

```bash
./bin/ocg stop [options]
```

**Options:**

| Option | Default | Description |
|--------|---------|-------------|
| `--pid-dir` | `/tmp/ocg` | PID file directory |

### status

Shows running status and health check results for each process.

```bash
./bin/ocg status [options]
```

**Example Output:**

```
embedding  running (pid 7963)
agent      running (pid 7983)
gateway    running (pid 7995)
embedding health: true
gateway health: true
```

**Options:**

| Option | Default | Description |
|--------|---------|-------------|
| `--config` | `./env.config` | config file path |
| `--pid-dir` | `/tmp/ocg` | PID file directory |

### restart

Equivalent to `stop` + `start`.

```bash
./bin/ocg restart [options]
```

## File Structure

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

## Configuration File

Defaults to reading `env.config`, supports these lookup methods:

1. path specified via `--config`
2. current directory `env.config`
3. same directory as executable `env.config`
4. parent directory of executable `env.config`

**Key Configuration:**

| Variable | Description |
|----------|-------------|
| `EMBEDDING_SERVER_URL` | embedding service address |
| `OPENCLAW_AGENT_SOCK` | agent Unix socket path |
| `OPENCLAW_PORT` | gateway port (default 55003) |
| `OPENCLAW_UI_TOKEN` | Web UI auth token |

## Health Checks

- **embedding**: `http://localhost:50000/health`
- **gateway**: `http://localhost:55003/health` (requires token)

## Relationship with Gateway

- **Old version**: `ocg-gateway` auto-starts agent/embedding
- **New version (ocg)**: ocg manages lifecycle, gateway only connects to agent

Gateway no longer auto-starts other services; you must run `ocg start` first.

## Troubleshooting

### Port Already in Use

```
listen tcp 0.0.0.0:50000: bind: address already in use
```

Solution: Stop the conflicting process or change the port in `env.config`.

### Service Startup Timeout

Check logs:

```bash
tail -f /tmp/ocg/logs/embedding.log
tail -f /tmp/ocg/logs/agent.log
tail -f /tmp/ocg/logs/gateway.log
```

### Stale PID Files

If process exits abnormally, manually clean up:

```bash
rm /tmp/ocg/*.pid
```
