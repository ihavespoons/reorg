# reorg

A personal organization tool that helps you manage areas, projects, and tasks using markdown files stored in git.

## Features

- **Hierarchical Organization**: Areas > Projects > Tasks
- **Markdown Storage**: Human-readable files with YAML frontmatter
- **Git Integration**: Automatic commits for all changes
- **CLI Interface**: Fast, keyboard-driven workflow

## Installation

```bash
# Clone and build
git clone https://github.com/beng/reorg.git
cd reorg
make build

# Or install directly
make install
```

## Quick Start

```bash
# Initialize a new data directory
reorg init

# Create areas, projects, and tasks
reorg area create "Work"
reorg project create "Website Redesign" --area work
reorg task create "Create wireframes" --project website-redesign

# View status
reorg status

# List and manage tasks
reorg task list
reorg task start <task-id>
reorg task complete <task-id>
```

## Data Structure

All data is stored in `~/.reorg/` (configurable) as markdown files:

```
~/.reorg/
├── config.yaml
├── areas/
│   ├── work/
│   │   ├── work.md
│   │   └── projects/
│   │       └── website-redesign/
│   │           ├── website-redesign.md
│   │           └── tasks/
│   │               └── create-wireframes.md
│   └── personal/
├── inbox/
└── archive/
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
tags: [design, ux]
---

# Create Wireframes

Task description and notes in markdown.

## Checklist
- [x] Home page
- [ ] Contact page
```

## Commands

| Command | Description |
|---------|-------------|
| `reorg init` | Initialize data directory |
| `reorg status` | Show overview |
| `reorg area list/create/show/delete` | Manage areas |
| `reorg project list/create/show/complete/delete` | Manage projects |
| `reorg task list/create/show/start/complete/delete` | Manage tasks |

## Configuration

Edit `~/.reorg/config.yaml`:

```yaml
mode: embedded
git:
  enabled: true
  auto_commit: true
cli:
  color: true
defaults:
  priority: medium
```

## Roadmap

- [ ] LLM integration (Claude API)
- [ ] Apple Notes import
- [ ] Web interface
- [ ] Server mode for multi-client access
- [ ] Claude Code skill integration

## License

MIT
