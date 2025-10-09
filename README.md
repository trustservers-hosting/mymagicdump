# mymagicdump

![License](https://img.shields.io/badge/license-GPL--3.0-blue)
![Go Version](https://img.shields.io/badge/go-%3E%3D1.20-blue)

## Table of Contents
- [Overview](#overview)
- [Why mymagicdump?](#why-mymagicdump)
- [Background](#background)
- [Requirements](#requirements)
- [Installation](#installation)
  - [Using Pre-built Binaries](#using-pre-built-binaries)
  - [Using Go Install](#using-go-install)
  - [From Source](#from-source)
- [Quick Start](#quick-start)
- [Usage](#usage)
  - [Connection Options](#connection-options)
  - [Database Selection](#database-selection)
  - [Table Filtering](#table-filtering)
  - [Output Options](#output-options)
  - [Execution Control](#execution-control)
- [Examples](#examples)
- [Disclaimer](#disclaimer)
- [License](#license)

## Overview

A fast and feature-rich mysqldump wrapper that supercharges your MySQL backup workflow with useful enhancements.

## Why mymagicdump?

Traditional mysqldump workflows can be cumbersome and error-prone. mymagicdump solves common pain points:

- **Select Databases**: Backup multiple databases in one command, using pattern-based filtering
- **Advanced table filtering**: Exclude tables by pattern (e.g., `logs_2024_*`)
- **Schema-only dumps for some tables**: Export only schema without data for some tables
- **Compressed dumps**: Support for tar.gz, tar.bz2, and zip
- **Separate files or single-file dumps**: One file per database or a consolidated backup file
- **Automatic retries**: Handle transient connection issues (with configurable retry logic)
- **More features**: Remove DEFINERs for cross-server restoration, real-time progress bar with size estimation, dry-run, and more

## Background

mymagicdump was born from a simple need: As a web hosting company, we backup many MySQL databases with different requirements, but existing tools made us choose between writing complex scripts or backing up everything we didn't need.

We needed flexibility and extra goodies derived from real-world everyday scenarios (like pattern-based exclusions, schema-only dumps for selected tables, and separate files per database), all in one simple, robust, and clean command.

## Requirements

- **Go**: Version 1.20 or higher (for building from source)
- **mysqldump**: Installed and accessible in PATH
- **MySQL/MariaDB**: Compatible with MySQL 5.7+, MySQL 8.x, and MariaDB 10.x+
- **Operating Systems**: Linux, macOS, Windows (with appropriate shell)

## Installation

### Using Pre-built Binaries

Download the latest binary for your operating system from the [Releases page](https://github.com/trustservers-hosting/mymagicdump/releases).
```bash
# Make executable and install to system PATH
chmod +x mymagicdump
sudo mv mymagicdump /usr/local/bin/mymagicdump
mymagicdump --help
```

### Using Go Install

```bash
go install github.com/trustservers-hosting/mymagicdump@latest
```

### From Source

```bash
# Clone the repository
git clone https://github.com/trustservers-hosting/mymagicdump.git
cd mymagicdump

# Build the binary
make build

# Optional: Install to system path
sudo mv mymagicdump /usr/local/bin/mymagicdump

# Verify installation
mymagicdump --help
```

## Quick Start

Backup all databases to separate files with compression:

```bash
mymagicdump \
  --user=dbuser \
  --password=YOUR_PASSWORD \
  --host=localhost \
  --all-databases \
  --separate-dumps \
  --output=/backups/ \
  --compression=tgz
```

This creates a separate compressed backup file per database in `/backups/`.

**Security Note**: Avoid passing passwords via `--password` in production. Use `--defaults-file` instead to keep credentials secure.

## Usage

```bash
mymagicdump [OPTIONS]
```

### Connection Options

- `-u, --user=USER` - MySQL username
- `-p, --password=PASSWORD` - MySQL password (Not recommended for production)
- `-h, --host=HOST` - MySQL host address
- `-P, --port=PORT` - MySQL port (default: 3306)
- `-s, --socket=SOCKET` - Path to MySQL socket
- `--defaults-file=FILE` - Path to MySQL defaults file (default: ~/.my.cnf)
- `--defaults-group-suffix=SUFFIX` - Suffix to append to the default group name

### Database Selection

- `--databases=DB1,DB2` - Comma-separated list of databases. Supports glob patterns (`*`, `?`)
- `--all-databases` - Dump all databases
- `--separate-dumps` - Create separate dump files for each database

### Table Filtering

- `--exclude=DB1.TABLE1,DB2.TABLE2` - Exclude tables completely. Supports patterns like `logs.app_log_*`
- `--exclude-data=DB1.TABLE1,DB2.TABLE2` - Exclude data but keep schema. Supports patterns

### Output Options

- `--output=PATH` - Output directory path (default: ./) - If multiple files are produced, the compressed file is named `multiple_databases`
- `--compression=TYPE` - Compression: `tgz`, `tbz2`, `zip`, or `none` (default: none)
- `--remove-definers` - Remove DEFINER statements for cross-server compatibility

### Execution Control

- `--dry-run` - Simulate without executing real mysqldump
- `--retries=NUM` - Number of retries on failure (default: 3)
- `--retry-interval=SECONDS` - Seconds between retries (default: 30)
- `-q, --silent` - Only print errors to stderr
- `-v, --verbose` - Enable verbose (debug) logging

### Forwarding Additional Flags to mysqldump

Any unrecognized flags are forwarded directly to mysqldump. This allows you to use standard mysqldump options like:

- `--no-tablespaces` - Disable tablespace handling
- `--single-transaction` - Use consistent snapshot
- `--where="condition"` - Apply WHERE clause to all tables

## Examples

### Backup Multiple Databases Separately

```bash
mymagicdump \
  --user=dbuser \
  --password=YOUR_PASSWORD \
  --host=localhost \
  --databases=shop_db,analytics_db,logs_db \
  --separate-dumps \
  --output=/backups/daily/ \
  --compression=tgz
```

Creates: `shop_db.tar.gz`, `analytics_db.tar.gz`, `logs_db.tar.gz`

### Exclude Some Tables

```bash
mymagicdump \
  --user=dbuser \
  --password=YOUR_PASSWORD \
  --databases=myapp \
  --exclude="myapp.logs,myapp.temp_*" \
  --output=/backups/
```

This excludes table `logs` and all tables starting with `temp_` in myapp database.

### Keep Schema but Exclude Large Data

```bash
mymagicdump \
  --user=dbuser \
  --password=YOUR_PASSWORD \
  --databases=analytics \
  --exclude-data="analytics.raw_events,analytics.page_views_*" \
  --output=/backups/ \
  --compression=tgz
```

Export database `analytics`, for tables named `raw_events` and any tables starting with `page_views_` dump schema only without any data.

### Backup All Databases Matching Pattern

```bash
mymagicdump \
  --user=dbuser \
  --password=YOUR_PASSWORD \
  --databases="client_*" \
  --separate-dumps \
  --output=/backups/clients/
```

Backs up all databases starting with `client_`.

### Using MySQL Defaults File

```bash
# Create ~/.my.cnf with credentials
mymagicdump \
  --defaults-file=~/.my.cnf \
  --databases=production_db \
  --output=/backups/ \
  --compression=tgz \
  --remove-definers
```

### Cron Job Example

```bash
# Daily backup at 2 AM
0 2 * * * /usr/local/bin/mymagicdump \
  --defaults-file=/root/.my.cnf \
  --all-databases \
  --separate-dumps \
  --output=/backups/$(date +\%Y-\%m-\%d)/ \
  --compression=tgz \
  --silent \
  2>&1 | logger -t mymagicdump
```

### Dry Run to Preview Commands

```bash
mymagicdump \
  --user=dbuser \
  --password=YOUR_PASSWORD \
  --databases=test_db \
  --exclude="test_db.cache_*" \
  --dry-run
```

Shows exactly what mysqldump commands would be executed.

### Passing Extra mysqldump Flags

```bash
mymagicdump \
  --user=dbuser \
  --password=YOUR_PASSWORD \
  --databases=mydb \
  --single-transaction \
  --where="created_at >= '2025-01-01'" \
  --output=/backups/
```

Any additional flags not recognized by `mymagicdump` are forwarded to `mysqldump` unchanged. In this example, `--single-transaction` and `--where` flags are passed through.

## Disclaimer

**IMPORTANT**: This software is provided "as is" without warranty of any kind, express or implied. The authors and contributors are not responsible for any data loss, corruption, or other damages that may occur from using this tool.

**Backup Best Practices:**
- Always test your backups by performing test restores
- Verify backup integrity before deleting source data
- Maintain multiple backup copies in different locations
- Never rely on a single backup solution
- Test this tool thoroughly in a non-production environment first

While mymagicdump is designed to be reliable and has been tested extensively, **you are solely responsible for ensuring your backup strategy meets your needs**. No backup tool can guarantee 100% data safety.

By using this software, you acknowledge that you have read and understood the GNU General Public License v3.0 and accept all risks associated with database backup operations.

## License

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

This project is licensed under the GNU General Public License v3.0 or later (GPL-3.0-or-later).
See the [LICENSE](./LICENSE) file for the full text.

Contributions are welcome and will be licensed under the same terms.
