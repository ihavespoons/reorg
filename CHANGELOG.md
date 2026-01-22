# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release
- Hierarchical organization: Areas > Projects > Tasks
- Markdown storage with YAML frontmatter
- CLI commands for managing areas, projects, and tasks
- Apple Notes import with AI categorization
- Obsidian vault import
- Inbox processing
- Server mode with gRPC and REST APIs
- Remote client for connecting to server
- Multiple Claude authentication methods (API key, OAuth, credentials file)
- Homebrew installation via `ihavespoons/tap`
- Automated releases via GitHub Actions
- Version command

### Commands
- `reorg init` - Initialize data directory
- `reorg status` - Show overview
- `reorg area list/create/show/delete` - Manage areas
- `reorg project list/create/show/complete/delete` - Manage projects
- `reorg task list/create/show/start/complete/delete` - Manage tasks
- `reorg import notes/obsidian/inbox` - Import from external sources
- `reorg serve` - Start gRPC/REST server
- `reorg version` - Show version information
