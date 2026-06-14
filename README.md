# osync

Obsidian vault synchronization tool. Sync your Obsidian vaults across multiple devices via a self-hosted server.

## Features

- **Diff-based sync** — Only transfer changed files, not entire vaults
- **Conflict resolution** — Automatic conflict detection with Obsidian-style conflict copies
- **Content-addressed storage** — SHA-256 deduplication saves bandwidth and storage
- **Soft-delete** — Deleted files can be restored within a configurable grace period
- **Self-hosted** — Run your own sync server with Docker

## Installation

### Via Scoop (Windows)

```powershell
scoop bucket add cerio-obsidian-sync https://github.com/AlexCas/scoop-cerio-obsync
scoop install osync
```

### Via Homebrew (Linux/macOS)

```bash
brew tap AlexCas/osync
brew install osync
```

### Manual Download

Download the latest release for your platform from [GitHub Releases](https://github.com/AlexCas/cerio-obsidian-sync/releases).

### From Source

```bash
go install github.com/AlexCas/cerio-obsidian-sync/cmd/osync@latest
```

## Quick Start

### 1. Interactive Menu (Recommended)

```bash
osync menu
```

The interactive TUI guides you through initialization and configuration:
- Initialize a new vault
- Set server URL, API key, and vault ID
- View current configuration
- Exit

Navigate with `↑`/`↓`, select with `Enter`.

### 2. Start the Server

```bash
# Using Docker
docker compose up -d

# Or run directly
osync server --port 8080 --data-dir ./data
```

### 3. Initialize a Vault

```bash
cd /path/to/your/vault
osync init
```

### 4. Configure Connection

```bash
osync config set server_url http://localhost:8080
osync config set api_key osync_your_api_key_here
osync config set vault_id my-vault
```

### 5. Sync

```bash
osync sync
```

## Configuration

| Key | Description | Default |
|-----|-------------|---------|
| `server_url` | Sync server URL | (required) |
| `api_key` | API key for authentication | (required) |
| `vault_id` | Vault identifier | (auto-generated) |
| `excluded_paths` | Paths to exclude from sync | `.obsidian` |
| `page_size` | Manifest page size | 5000 |

## Architecture

```
┌─────────────┐      ┌──────────────┐      ┌─────────────┐
│  Client A   │◄────►│  osync       │◄────►│  Client B   │
│  (vault 1)  │      │  server      │      │  (vault 2)  │
└─────────────┘      └──────────────┘      └─────────────┘
                          │
                          ▼
                   ┌──────────────┐
                   │   SQLite +   │
                   │   File Store │
                   └──────────────┘
```

## License

MIT