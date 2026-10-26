/*
    mymagicdump

	A fast, scriptable MySQL/MariaDB backup wrapper around mysqldump
	with exclusions, retries, progress, optional compression and more

	Copyright (C) 2025 Trustservers PC

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.

*/

package mysqlutil

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/trustservers-hosting/mymagicdump/internal/config"
	"github.com/trustservers-hosting/mymagicdump/internal/logging"
	"github.com/trustservers-hosting/mymagicdump/internal/process"
)

// CheckConnection verifies that the client can authenticate before backup preparation.
func CheckConnection(ctx context.Context, mysqlConnFlags []string) error {
	if _, err := queryMySQL(ctx, mysqlConnFlags, "SELECT 1;"); err != nil {
		return fmt.Errorf("connection check failed: %w", err)
	}
	return nil
}

// ListDatabases discovers databases using the supplied connection settings.
// The information_schema, performance_schema, and sys schemas are omitted,
// matching the schemas skipped by mysqldump's --all-databases option.
func ListDatabases(ctx context.Context, mysqlConnFlags []string) ([]string, error) {
	output, err := queryMySQL(ctx, mysqlConnFlags, "SHOW DATABASES;")
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}
	var databases []string
	for _, line := range strings.Split(string(output), "\n") {
		database := strings.TrimSpace(line)
		if database == "" || isDefaultExcludedSchema(database) {
			continue
		}
		databases = append(databases, database)
	}
	return databases, nil
}

// queryMySQL keeps client diagnostics out of query results, while preserving
// stderr in errors when the client fails.
func queryMySQL(ctx context.Context, mysqlConnFlags []string, query string) ([]byte, error) {
	args := append([]string{}, mysqlConnFlags...)
	args = append(args, "-sNe", query)
	cmd := process.CommandContext(ctx, "mysql", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("command failed: %w; %s", err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func isDefaultExcludedSchema(database string) bool {
	return database == "information_schema" || database == "performance_schema" || database == "sys"
}

// BuildConnectionFlags creates the flags for connecting to the MySQL instance.
func BuildConnectionFlags(opts config.Options) []string {
	args := []string{"--no-defaults"}
	defaultsFile := opts.DefaultsFile
	if defaultsFile == "" && opts.User == "" && opts.Password == "" && opts.Host == "" && opts.Port == "" && opts.Socket == "" {
		defaultsFile = "~/.my.cnf"
	}
	// MySQL option-file flags must precede ordinary connection options.
	if defaultsFile != "" {
		expanded := config.ExpandTilde(defaultsFile)
		if _, err := os.Stat(expanded); err == nil {
			args = []string{"--defaults-file=" + expanded}
			if opts.DefaultsGroupSuffix != "" {
				args = append(args, "--defaults-group-suffix="+opts.DefaultsGroupSuffix)
			}
		} else {
			logging.Info("Skipping --defaults-file: %s not found", expanded)
		}
	}
	if opts.User != "" {
		args = append(args, "-u", opts.User)
	}
	if opts.Password != "" {
		args = append(args, "-p"+opts.Password)
	}
	if opts.Host != "" {
		args = append(args, "-h", opts.Host)
	}
	if opts.Port != "" {
		args = append(args, "-P", opts.Port)
	}
	if opts.Socket != "" {
		args = append(args, "--socket", opts.Socket)
	}
	return args
}

func GetTablesMatchingGlob(ctx context.Context, mysqlConnFlags []string, dbName, globPattern string) ([]string, error) {
	likePattern := globToLike(globPattern)
	quotedDBName := "`" + strings.ReplaceAll(dbName, "`", "``") + "`"
	output, err := queryMySQL(ctx, mysqlConnFlags, fmt.Sprintf("SHOW TABLES IN %s LIKE '%s';", quotedDBName, likePattern))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")
	var tables []string
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			tables = append(tables, t)
		}
	}
	return tables, nil
}

// ExpandDatabaseList expands shell-like patterns in the provided database list.
// Supports '*' and '?' wildcards. If an entry contains wildcards, it is resolved
// against MySQL via SHOW DATABASES LIKE ... Otherwise the name is used as-is.
// Wildcard matches omit the same default-excluded schemas as ListDatabases; explicit
// database names are preserved.
func ExpandDatabaseList(ctx context.Context, mysqlConnFlags []string, entries []string) ([]string, error) {
	var out []string
	for _, e := range entries {
		if hasGlobWildcards(e) {
			likePattern := globToLike(e)
			matched, err := resolveDBLike(ctx, mysqlConnFlags, likePattern)
			if err != nil {
				return nil, fmt.Errorf("resolve databases for pattern %q: %w", e, err)
			}
			for _, database := range matched {
				if !isDefaultExcludedSchema(database) {
					out = append(out, database)
				}
			}
		} else {
			out = append(out, e)
		}
	}
	// Deduplicate while preserving order
	seen := map[string]struct{}{}
	dedup := make([]string, 0, len(out))
	for _, d := range out {
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		dedup = append(dedup, d)
	}
	return dedup, nil
}

func hasGlobWildcards(s string) bool {
	return strings.ContainsAny(s, "*?")
}

// Convert shell-style glob to SQL LIKE pattern.
// '*' -> '%', '?' -> '_', and escape existing '%' and '_' to avoid accidental matches.
func globToLike(glob string) string {
	// First escape '%' and '_' by replacing with escaped versions using backslash
	// MySQL default escape character is '\\' in string literals, so we need '\\\\' for a literal '\\'
	esc := strings.ReplaceAll(glob, "%", "\\\\%")
	esc = strings.ReplaceAll(esc, "_", "\\\\_")
	// Replace glob wildcards
	esc = strings.ReplaceAll(esc, "*", "%")
	esc = strings.ReplaceAll(esc, "?", "_")
	return esc
}

func resolveDBLike(ctx context.Context, mysqlConnFlags []string, likePattern string) ([]string, error) {
	query := fmt.Sprintf("SHOW DATABASES LIKE '%s';", likePattern)
	output, err := queryMySQL(ctx, mysqlConnFlags, query)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(output), "\n")
	var dbs []string
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			dbs = append(dbs, t)
		}
	}
	return dbs, nil
}

func ExtractDatabasesFromFlags(ctx context.Context, mysqlDumpFlags []string) []string {
	for _, flag := range mysqlDumpFlags {
		if flag == "--all-databases" {
			output, err := queryMySQL(ctx, nil, "SHOW DATABASES;")
			if err != nil {
				logging.Warn("Failed to list databases: %v", err)
				return nil
			}
			all := strings.Split(string(output), "\n")
			if len(all) > 0 {
				all = all[:len(all)-1]
			}
			return all
		}
	}
	for i, flag := range mysqlDumpFlags {
		if flag == "--databases" {
			databases := []string{}
			for _, flag2 := range mysqlDumpFlags[i+1:] {
				if !strings.HasPrefix(flag2, "--") {
					databases = append(databases, flag2)
				}
			}
			return databases
		}
	}
	if len(mysqlDumpFlags) > 0 {
		return []string{mysqlDumpFlags[len(mysqlDumpFlags)-1]}
	}
	return nil
}

func CalculateDatabaseSize(ctx context.Context, mysqlConnFlags []string, targetDatabases []string) (int, error) {
	query := `
        SELECT ROUND(SUM(data_length + index_length), 0)
        FROM information_schema.TABLES
        WHERE table_schema IN ('` + strings.Join(targetDatabases, "', '") + `')
    `
	output, err := queryMySQL(ctx, mysqlConnFlags, query)
	if err != nil {
		return 0, fmt.Errorf("failed to execute query: %w", err)
	}
	if string(output) == "" {
		return 0, fmt.Errorf("query returned empty output; check if databases exist")
	}
	var sz int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &sz); err != nil {
		return 0, fmt.Errorf("failed to convert output to int: %w", err)
	}
	return sz, nil
}
