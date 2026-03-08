# Go Project Template

A production-ready Go project template following [golang-standards/project-layout](https://github.com/golang-standards/project-layout) conventions.

## Features

- **Modular Design**: Config and logger as independent, reusable packages
- **Hot Reload**: Configuration hot-reload with file watcher (fsnotify)
- **Concurrent Safe**: Atomic operations for global variables
- **Observer Pattern**: Loose coupling between config and logger via callbacks
- **Graceful Shutdown**: Signal handling for clean exit
- **Structured Logging**: Zap logger with rotation (lumberjack)

## Project Structure

```
.
├── cmd/
│   └── app/
│       └── main.go              # Entry point
├── internal/
│   ├── app/
│   │   └── app.go               # Business logic
│   ├── config/
│   │   ├── config.go            # Config toolkit: singleton, Load, Get, AddWatch
│   │   └── watcher.go           # File watcher with debounce
│   ├── logger/
│   │   └── logger.go            # Logger toolkit: global methods, dynamic update
│   └── signal/
│       └── signal.go            # Signal handling
├── pkg/
│   └── version/
│       └── version.go
├── configs/
│   └── config.yaml
├── build/
│   └── .goreleaser.yaml
├── scripts/
├── go.mod
└── README.md
```

## Quick Start

```bash
# Download dependencies
go mod download

# Run the application
go run ./cmd/app

# Run with custom config
go run ./cmd/app -c /path/to/config.yaml
```

## Configuration

Edit `configs/config.yaml`:

```yaml
app:
  name: myapp
  env: dev

log:
  level: info
  format: console
  path: logs/app.log
  max_size: 200
  max_age: 60
  max_backups: 60
  compress: true
  log_to_console: true
```

### Hot Reload

Modify `configs/config.yaml` while the application is running. The config will be reloaded automatically and callbacks triggered.

## Usage Examples

### Using Logger

```go
import "go-template/internal/logger"

// Structured logging
logger.Info("message", zap.String("key", "value"))

// Formatted logging
logger.Infof("User %s logged in", username)
```

### Using Config

```go
import "go-template/internal/config"

// Get config
cfg := config.Get()
fmt.Println(cfg.App.Name)

// Watch config changes
config.AddWatch(func(newCfg, oldCfg *config.Config) {
    // Handle config change
})
```

## Build

```bash
# Build binary
go build -o bin/app ./cmd/app

# GoReleaser
goreleaser release --snapshot --skip=publish --clean
```

## License

MIT
