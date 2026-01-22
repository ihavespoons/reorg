# reorg

A personal organization tool that helps you manage areas, projects, and tasks using markdown files stored in git.

## Features

- **Hierarchical Organization**: Areas > Projects > Tasks
- **Markdown Storage**: Human-readable files with YAML frontmatter
- **Git Integration**: Version control for all your organizational data
- **AI-Powered Import**: Import and categorize notes from Apple Notes or Obsidian
- **Hybrid Architecture**: Run locally (embedded) or connect to a server (remote mode)
- **CLI Interface**: Fast, keyboard-driven workflow

## Installation

### Homebrew (recommended)

```bash
brew tap ihavespoons/tap
brew install reorg
```

### From Source

```bash
git clone https://github.com/ihavespoons/reorg.git
cd reorg
go build -o reorg ./cmd/reorg
sudo mv reorg /usr/local/bin/
```

## Quick Start

```bash
# Initialize with default areas (Work, Personal, Life Admin)
reorg init

# Check status
reorg status

# Create a project
reorg project create "Website Redesign" --area work

# Create and manage tasks
reorg task create "Create wireframes" --project website-redesign
reorg task start <task-id>
reorg task complete <task-id>
```

## Commands

### Areas
```bash
reorg area list                    # List all areas
reorg area create "Side Projects"  # Create a new area
reorg area show work               # Show area details
reorg area delete side-projects    # Delete an area (must be empty)
```

### Projects
```bash
reorg project list                           # List all projects
reorg project list --area work               # Filter by area
reorg project create "New Project" -a work   # Create in specific area
reorg project show my-project                # Show details
reorg project complete my-project            # Mark as completed
```

### Tasks
```bash
reorg task list                              # List all tasks
reorg task list --project my-project         # Filter by project
reorg task list --status in_progress         # Filter by status
reorg task create "Do something" -p project  # Create task
reorg task start <id>                        # Mark as in progress
reorg task complete <id>                     # Mark as completed
reorg task show <id>                         # Show details
```

### Import

Import notes from external sources with AI-powered categorization:

```bash
# Import from Apple Notes
reorg import notes                  # Notes from last 24 hours
reorg import notes --since 7d       # Notes from last 7 days
reorg import notes --folder "Work"  # From specific folder
reorg import notes --auto           # Auto-accept categorizations
reorg import notes --dry-run        # Preview without changes

# Import from Obsidian
reorg import obsidian /path/to/vault
reorg import obsidian --vault ~/Documents/Obsidian

# Process inbox
reorg import inbox
```

### Server Mode

Run reorg as a server for multi-client access:

```bash
# Start the server
reorg serve                         # gRPC on :50051, REST on :8080
reorg serve --grpc-port 9000        # Custom ports
reorg serve --rest-port 8888

# Connect from another client
reorg --mode remote --server localhost:50051 status
```

## Configuration

Configuration is stored in `~/.reorg/config.yaml`:

```yaml
# Operation mode: embedded (local) or remote (server)
mode: embedded

# Server settings (for remote mode)
server:
  address: localhost:50051

# Git integration
git:
  enabled: true
  auto_commit: true

# LLM settings for AI features
llm:
  provider: claude
  model: claude-sonnet-4-20250514
  # API key (or use ANTHROPIC_API_KEY env var)
  api_key: sk-ant-...

# Integrations
integrations:
  obsidian:
    vault_path: ~/Documents/Obsidian
```

## AI Authentication

The import features require Claude API access. Multiple authentication methods are supported:

### 1. API Key (Claude Max or API Plan)

Get your API key from [console.anthropic.com](https://console.anthropic.com/settings/keys):

```bash
# Environment variable
export ANTHROPIC_API_KEY="sk-ant-..."

# Or in config.yaml
llm:
  api_key: sk-ant-...
```

### 2. Claude Code OAuth (Claude Max)

If you have Claude Code installed and logged in, reorg will automatically use your OAuth session:

```bash
claude login  # If not already logged in
reorg import notes  # Uses Claude Code credentials
```

### 3. Credentials File

```bash
mkdir -p ~/.config/anthropic
echo "sk-ant-your-key" > ~/.config/anthropic/credentials
```

## Data Structure

All data is stored in `~/.reorg/` as markdown files:

```
~/.reorg/
├── config.yaml
├── work/
│   ├── _area.md
│   └── website-redesign/
│       ├── _project.md
│       ├── create-wireframes.md
│       └── design-homepage.md
├── personal/
│   └── _area.md
├── life-admin/
│   └── _area.md
└── inbox/
```

### File Format

Tasks are stored as markdown with YAML frontmatter:

```yaml
---
id: task-abc123
title: Create Wireframes
type: task
project_id: proj-xyz789
area_id: area-work-001
status: in_progress
priority: high
due_date: 2025-02-01
tags:
  - design
  - ux
created: 2025-01-22T10:00:00Z
updated: 2025-01-22T14:30:00Z
---

Task description and notes in markdown.

## Checklist
- [x] Home page
- [ ] Contact page
```

## Global Flags

```bash
--config string     # Config file (default ~/.reorg/config.yaml)
--data-dir string   # Data directory (default ~/.reorg)
--mode string       # Operation mode: embedded or remote
--server string     # Server address for remote mode
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Claude API key for AI features |
| `CLAUDE_API_KEY` | Alternative API key variable |
| `REORG_DATA_DIR` | Override data directory |
| `REORG_MODE` | Set operation mode |

## Development

```bash
# Build
go build ./cmd/reorg

# Run tests
go test ./...

# Run with race detector
go test -race ./...
```

## License

MIT
