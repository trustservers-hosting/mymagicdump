# mymagicdump

## Overview
A fast mysqldump wrapper, providing several enhancements like per-database file output, table exclusions, automatic retries, progress bar and more.


## Features
- Multiple databases in one go or separate files per DB.
- Pattern-based table exclude and data-only exclude (keep schema).
- Progress bar with estimated size via information_schema.
- Optional `tar.gz`, `tar.bz2`, or `zip` compression 
- Optional mysql DEFINER removal.
- Retries with configurable attempts and interval.

## Installation
```
git clone <this-repo-url>
cd mymagicdump
make build

# optional, place the binary in /usr/local/bin
mv mymagicdump /usr/local/bin/mymagicdump
```

## Usage
Example:
```bash
./mymagicdump \
  --user=root --password=secret --host=localhost \
  --databases=demo_logs,demo_shop \
  --exclude=demo_logs.app_log_2025_* \
  --exclude-data=demo_shop.audit_trail \
  --output=/backups/ \
  --compression=tgz \
  --silent
```

### Available Options
```
Usage:
  mymagicdump [OPTIONS]

Application Options:
  -u, --user=USER                                   MySQL username
  -p, --password=PASSWORD                           MySQL password
  -h, --host=HOST                                   MySQL host address
  -P, --port=PORT                                   MySQL port
  -s, --socket=SOCKET                               Path to MySQL socket
      --defaults-file=FILE                          Path to MySQL defaults file (default: ~/.my.cnf)
      --defaults-group-suffix=                      Suffix to append to the default group name in the MySQL configuration file
      --databases=DATABASE1,DATABASE2               Comma-separated list of databases to dump. Supports glob patterns (* and ?) per entry.
      --separate-dumps                              Create separate dump files for each database provided with --databases
      --all-databases                               Dump all databases
      --exclude=DB1.TABLE1,DB2.TABLE2               Comma-separated list of tables to exclude. Supports glob patterns (* and ?).
      --exclude-data=DB1.TABLE1,DB2.TABLE2          Comma-separated list of tables to exclude data from (but keep the schema). Supports glob patterns (* and ?).
      --output=PATH                                 Output file path (default: ./)
      --compression=[tgz|tbz2|zip|none]             Compression type (tgz, tbz2, zip, none) (default: none)
      --dry-run                                     Simulate the dump process
      --remove-definers                             Remove definer statements
      --retries=NUM_RETRIES                         Number of retries on failure (default: 3)
      --retry-interval=SECONDS                      Seconds between retries (default: 30)
  -q, --silent                                      Only print errors to stderr
  -v, --verbose                                     Enable verbose (debug) logging

Help Options:
  -h, --help                                        Show this help message
```

## Notes
- Output files are written under `--output`. If multiple files are produced, the compressed file is named `multiple_databases`.
- `--dry-run` executes planning and prints commands without running mysqldump.

### Passing Extra mysqldump Flags
Any additional flags not recognized by `mymagicdump` are forwarded to `mysqldump` unchanged.

Examples:
```bash
# Disable tablespace handling
./mymagicdump --databases mydb --no-tablespaces

# Apply a WHERE clause (quote carefully)
./mymagicdump --databases mydb --where "created_at >= '2025-01-01'"
```
## License

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

This project is licensed under the GNU General Public License v3.0 or later (GPL-3.0-or-later).
See the [LICENSE](./LICENSE) file for the full text.

Contributions are welcome and will be licensed under the same terms.