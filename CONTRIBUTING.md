## Contributing to mymagicdump

Thank you for your interest in contributing! This document describes how to build, test, and submit changes. General usage and features live in README only.

### Development setup
- Go 1.22+
- A local MySQL/MariaDB for testing (optional)

Clone and build:
```bash
git clone <this-repo-url>
cd mymagicdump
make build
./mymagicdump --version
```

## Project Layout
- `cmd/mymagicdump/`: Main application.
- `internal/dumper/`: Core dump planning and execution.
- `internal/mysqlutil/`: MySQL helpers (table/db discovery, size calculations, flags).
- `internal/compress/`: File compression helpers.
- `internal/logging/`: Logging with verbosity levels.
- `internal/config/`: Input flags and parsing.
- `internal/version/`: Version information and metadata.

## Makefile
Common targets:
```bash
# Build
make build

# Print embedded version
make version

# Run with args
make run ARGS="--help"

# Create and push a tag
make tag VERSION=v0.2.0 MESSAGE="Release v0.2.0"
make tag-push
```

### Versioning and releases
Version metadata is embedded at build time via `-ldflags` (see `Makefile`). Release tags follow `vMAJOR.MINOR.PATCH`.

### License and contributions
By contributing, you agree that your contributions will be licensed under the project’s GPL-3.0-or-later license. See [LICENSE](./LICENSE) for details.
