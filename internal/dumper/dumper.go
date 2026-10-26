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

package dumper

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"

	"github.com/trustservers-hosting/mymagicdump/internal/compress"
	"github.com/trustservers-hosting/mymagicdump/internal/config"
	"github.com/trustservers-hosting/mymagicdump/internal/logging"
	"github.com/trustservers-hosting/mymagicdump/internal/mysqlutil"
	"github.com/trustservers-hosting/mymagicdump/internal/process"
)

type Runner struct {
	ctx           context.Context
	Opts          *config.Options
	ConnFlags     []string
	DumpFlagsList [][]string
	OutputFiles   []string
	pendingSchema string
}

func NewRunner(opts *config.Options) *Runner {
	return NewRunnerWithContext(context.Background(), opts)
}

func NewRunnerWithContext(ctx context.Context, opts *config.Options) *Runner {
	return &Runner{Opts: opts, ctx: ctx}
}

func (r *Runner) Prepare() error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	r.ConnFlags = mysqlutil.BuildConnectionFlags(*r.Opts)
	if err := mysqlutil.CheckConnection(r.ctx, r.ConnFlags); err != nil {
		return err
	}
	if r.Opts.AllDatabases && r.Opts.SeparateDumps {
		databases, err := mysqlutil.ListDatabases(r.ctx, r.ConnFlags)
		if err != nil {
			return err
		}
		if len(databases) == 0 {
			return fmt.Errorf("no databases available for separate dumps")
		}
		r.Opts.Databases = databases
	}
	// Expand potential glob patterns in database list unless dumping all databases
	if !r.Opts.AllDatabases && len(r.Opts.Databases) > 0 {
		expanded, err := mysqlutil.ExpandDatabaseList(r.ctx, r.ConnFlags, r.Opts.Databases)
		if err != nil {
			return fmt.Errorf("expand database patterns: %w", err)
		}
		r.Opts.Databases = expanded
		if len(r.Opts.Databases) == 0 {
			logging.Error("No databases matched the provided patterns.")
			return fmt.Errorf("no databases matched provided patterns")
		}
	}
	// compute tables based on patterns
	excluded, err := constructExcludedTables(r.ctx, r.ConnFlags, r.Opts.ExcludeTables)
	if err != nil {
		return err
	}
	excludedData, err := constructExcludedTables(r.ctx, r.ConnFlags, r.Opts.ExcludeTablesData)
	if err != nil {
		return err
	}
	// build dump flags
	r.DumpFlagsList = buildDumpFlags(*r.Opts, excluded, excludedData)
	return r.ctx.Err()
}

func (r *Runner) Run() error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if r.Opts.DryRun {
		logging.Info("Dry-run mode enabled. No commands will be executed.")
	}
	for _, mysqlDumpFlags := range r.DumpFlagsList {
		targetDatabases := mysqlutil.ExtractDatabasesFromFlags(r.ctx, mysqlDumpFlags)
		dbSize, err := mysqlutil.CalculateDatabaseSize(r.ctx, r.ConnFlags, targetDatabases)
		if err != nil {
			logging.Warn("Error calculating expected dump size: %v", err)
		} else {
			logging.Info("Estimated database size: %d bytes", dbSize)
		}
		logging.Info("Starting backup for databases: %s", strings.Join(targetDatabases, ", "))

		if err := r.dumpWithRetries(mysqlDumpFlags, int64(dbSize)); err != nil {
			if r.ctx.Err() != nil {
				logging.Error("Backup canceled.")
				return r.ctx.Err()
			}
			logging.Error("Backup failed after all retries.")
			return err
		}
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	// post-process
	if r.Opts.DryRun {
		return nil
	}
	if r.Opts.RemoveDefiners {
		if err := r.removeDefiners(); err != nil {
			logging.Error("%v", err)
			return err
		}
	}
	// compression prefix
	prefix := compressionPrefix(r.Opts, r.OutputFiles)
	if err := compress.ApplyCompression(prefix, r.Opts.Compression, r.OutputFiles); err != nil {
		logging.Error("%v", err)
		return err
	}
	return nil
}

// dumpWithRetries runs a single dump command with retries.
func (r *Runner) dumpWithRetries(mysqlDumpFlags []string, dbSize int64) error {
	if r.Opts.Retries < -1 {
		return fmt.Errorf("--retries must be -1 (unlimited) or greater")
	}
	if r.Opts.RetryInterval < 0 {
		return fmt.Errorf("--retry-interval must be non-negative")
	}
	for attempt := uint64(0); ; attempt++ {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if r.Opts.Retries == -1 {
			logging.Info("Attempt %d for dumping database(s) (unlimited retries)", attempt+1)
		} else {
			logging.Info("Attempt %d/%d for dumping database(s)", attempt+1, uint64(r.Opts.Retries)+1)
		}
		err := r.singleDump(mysqlDumpFlags, dbSize)
		if err == nil {
			return nil
		}
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if r.Opts.Retries != -1 && attempt >= uint64(r.Opts.Retries) {
			return err
		}
		logging.Warn("Dump failed: %v. Retrying in %d seconds.", err, r.Opts.RetryInterval)
		timer := time.NewTimer(time.Duration(r.Opts.RetryInterval) * time.Second)
		select {
		case <-r.ctx.Done():
			timer.Stop()
			return r.ctx.Err()
		case <-timer.C:
		}
	}
}

// singleDump executes one mysqldump/mariadb-dump run and waits for completion with a progress bar.
func (r *Runner) singleDump(mysqlDumpFlags []string, dbSize int64) error {
	// Prepare the dump command, appending any passthrough flags
	mysqldumpArgs := r.buildDumpArgs(mysqlDumpFlags)

	// Prefer mariadb-dump, falling back to mysqldump when unavailable.
	binaryPath, err := resolveDumpBinary()
	if err != nil {
		logging.Error("mymagicdump is a mysqldump/mariadb-dump wrapper tool, cannot find mysqldump or mariadb-dump in PATH.")
		return err
	}
	dumpCmd := process.CommandContext(r.ctx, binaryPath, mysqldumpArgs...)
	logging.Debug("Executing command: %s", commandForLog(dumpCmd.Args))

	// Exit early if in dry-run mode
	if r.Opts.DryRun {
		return nil
	}

	if err := os.MkdirAll(r.Opts.OutputPath, os.ModePerm); err != nil {
		return err
	}
	finalFilePath := filepath.Join(r.Opts.OutputPath, outputNameFromFlags(r.Opts, mysqlDumpFlags))
	outputFilePath := finalFilePath + ".partial"
	// Combined dumps share one file; data and triggers append after the schema.
	combinedDump := !r.Opts.SeparateDumps && len(r.Opts.ExcludeTablesData) > 0
	appendPass := combinedDump && slices.Contains(mysqlDumpFlags, "--no-create-info")
	triggersOnly := slices.Contains(mysqlDumpFlags, "--no-data") && slices.Contains(mysqlDumpFlags, "--no-create-info")
	openFlags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if appendPass {
		if r.pendingSchema != outputFilePath {
			return fmt.Errorf("cannot append without a successful schema dump: %s", outputFilePath)
		}
		openFlags = os.O_WRONLY | os.O_APPEND
	}
	outf, err := os.OpenFile(outputFilePath, openFlags, 0666)
	if err != nil {
		logging.Error("Failed to create output file %s: %v", outputFilePath, err)
		return err
	}
	defer outf.Close()
	completed := false
	if appendPass {
		info, err := outf.Stat()
		if err != nil {
			return err
		}
		// Roll back a failed append pass before retrying to avoid duplicates.
		defer func() {
			if !completed {
				if err := os.Truncate(outputFilePath, info.Size()); err != nil {
					logging.Error("Failed to roll back partial dump: %v", err)
				}
			}
		}()
	}

	// Set output and error streams for the command
	dumpCmd.Stdout = outf
	dumpCmd.Stderr = os.Stderr

	// Start and wait for the command to complete
	startTime := time.Now()
	if err = dumpCmd.Start(); err != nil {
		logging.Error("Failed to start mysqldump: %v", err)
		return err
	}
	logging.Info("Dump process started...")

	// Use a channel to detect when `mysqldump` completes
	done := make(chan error, 1)
	go func() { done <- dumpCmd.Wait() }()

	// Create a progress bar
	var bar *progressbar.ProgressBar
	if !r.Opts.Silent {
		bar = progressbar.NewOptions64(
			dbSize,
			progressbar.OptionSetDescription("Dumping database... [elapsed: 0s - estimated total: ?]"),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowTotalBytes(true),
			progressbar.OptionSetWidth(10),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowCount(),
			progressbar.OptionOnCompletion(func() {
				fmt.Fprint(os.Stderr, "\n")
			}),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetRenderBlankState(true),
			progressbar.OptionSetPredictTime(false),
			progressbar.OptionSetElapsedTime(false),
		)
	}

	// Monitor file size and update progress bar
	if err := r.monitorDump(done, outputFilePath, bar); err != nil {
		return err
	}

	if err := r.ctx.Err(); err != nil {
		return err
	}

	// Publish only after all passes have succeeded and the file is safely closed.
	if err := outf.Sync(); err != nil {
		return fmt.Errorf("sync dump %s: %w", outputFilePath, err)
	}
	if err := outf.Close(); err != nil {
		return fmt.Errorf("close dump %s: %w", outputFilePath, err)
	}
	if combinedDump && !triggersOnly {
		r.pendingSchema = outputFilePath
	} else {
		if err := os.Rename(outputFilePath, finalFilePath); err != nil {
			return fmt.Errorf("publish dump %s: %w", finalFilePath, err)
		}
		if appendPass {
			r.pendingSchema = ""
		}
		if !slices.Contains(r.OutputFiles, finalFilePath) {
			r.OutputFiles = append(r.OutputFiles, finalFilePath)
		}
	}
	completed = true
	elapsed := time.Since(startTime)
	if bar != nil {
		bar.Describe(fmt.Sprintf("Dumping database... [elapsed: %s]", elapsed.Truncate(time.Second)))
		bar.Finish()
	}
	logging.Info("Dump completed successfully in %s", elapsed)
	return nil
}

// commandForLog masks password values without changing the command's actual arguments.
func commandForLog(args []string) string {
	masked := append([]string{}, args...)
	for i := 1; i < len(masked); i++ {
		switch {
		case strings.HasPrefix(masked[i], "--password="):
			masked[i] = "--password=****"
		case strings.HasPrefix(masked[i], "-p") && len(masked[i]) > 2:
			masked[i] = "-p****"
		}
	}
	return strings.Join(masked, " ")
}

// buildDumpArgs builds the full mysqldump argument slice from connection, passthrough, and dump flags.
func (r *Runner) buildDumpArgs(mysqlDumpFlags []string) []string {
	mysqldumpArgs := append([]string{}, r.ConnFlags...)
	if len(r.Opts.Passthrough) > 0 {
		mysqldumpArgs = append(mysqldumpArgs, r.Opts.Passthrough...)
	}
	mysqldumpArgs = append(mysqldumpArgs, mysqlDumpFlags...)
	return mysqldumpArgs
}

// resolveDumpBinary prefers mariadb-dump to avoid MariaDB's deprecated mysqldump alias.
func resolveDumpBinary() (string, error) {
	if path, err := exec.LookPath("mariadb-dump"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("mysqldump"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("cannot find mysqldump or mariadb-dump in PATH")
}

// monitorDump updates the progress bar until the dump command finishes.
func (r *Runner) monitorDump(done <-chan error, outputFilePath string, bar *progressbar.ProgressBar) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				if exiterr, ok := err.(*exec.ExitError); ok {
					logging.Error("mysqldump exited with status %d.", exiterr.ExitCode())
				}
				return err
			}
			return nil
		case <-ticker.C:
			if bar != nil {
				if fi, err := os.Stat(outputFilePath); err == nil {
					bar.Set64(fi.Size())

					state := bar.State()
					elapsed := time.Duration(state.SecondsSince * float64(time.Second)).Truncate(time.Second)
					if state.CurrentPercent >= 1.0 {
						bar.Describe(fmt.Sprintf("Dumping database... [elapsed: %s]", elapsed))
					} else {
						est := time.Duration(state.SecondsLeft * float64(time.Second)).Truncate(time.Second)
						if state.SecondsLeft > 0 {
							bar.Describe(fmt.Sprintf("Dumping database... [elapsed: %s - estimated total: %s]", elapsed, est))
						} else {
							bar.Describe(fmt.Sprintf("Dumping database... [elapsed: %s - estimated total: ?]", elapsed))
						}
					}
				}
			}
		}
	}
}

func outputNameFromFlags(opts *config.Options, mysqlDumpFlags []string) string {
	if opts.SeparateDumps {
		currDatabase := mysqlDumpFlags[len(mysqlDumpFlags)-1]
		if slices.Contains(mysqlDumpFlags, "--no-data") && slices.Contains(mysqlDumpFlags, "--no-create-info") {
			currDatabase = currDatabase + "_triggers"
		} else if slices.Contains(mysqlDumpFlags, "--no-data") {
			currDatabase = currDatabase + "_schema"
		} else if slices.Contains(mysqlDumpFlags, "--no-create-info") {
			currDatabase = currDatabase + "_data"
		}
		return currDatabase + ".sql"
	}
	if len(opts.Databases) == 1 {
		return opts.Databases[0] + ".sql"
	}
	return "multiple_databases.sql"
}

func compressionPrefix(opts *config.Options, files []string) string {
	if len(files) == 1 {
		return files[0]
	}
	return filepath.Join(opts.OutputPath, "multiple_databases")
}

func buildDumpFlags(opts config.Options, excludedTables, excludedTablesData []string) [][]string {
	argsList := [][]string{}
	baseArgs := []string{}
	schemaArgs := []string{}
	dataArgs := []string{}
	triggerArgs := []string{}

	// Common arguments for all dumps
	for _, table := range excludedTables {
		baseArgs = append(baseArgs, "--ignore-table="+table)
	}
	if opts.AllDatabases && !opts.SeparateDumps {
		baseArgs = append(baseArgs, "--all-databases")
	} else if opts.AllDatabases && opts.SeparateDumps {
		// Each command must dump only its selected database, including CREATE/USE.
		baseArgs = append(baseArgs, "--databases")
	} else if !opts.SeparateDumps {
		baseArgs = append(baseArgs, "--databases")
		baseArgs = append(baseArgs, opts.Databases...)
	}

	// Restore triggers after data so loading rows cannot fire them. The trigger
	// pass includes data-excluded tables, but still honors full table exclusions.
	if len(opts.ExcludeTablesData) > 0 {
		schemaArgs = append([]string{}, baseArgs...)
		schemaArgs = append(schemaArgs, "--no-data", "--skip-triggers")
		dataArgs = append([]string{}, baseArgs...)
		dataArgs = append(dataArgs, "--no-create-info", "--skip-triggers")
		triggerArgs = append([]string{}, baseArgs...)
		triggerArgs = append(triggerArgs, "--no-data", "--no-create-info")
		for _, t := range excludedTablesData {
			dataArgs = append(dataArgs, "--ignore-table="+t)
		}
	}

	// If SeparateDumps is enabled, create individual dump commands for each database
	if opts.SeparateDumps {
		for _, database := range opts.Databases {
			// Check to see if we need separate schema/data for this database
			separatedDump := false
			for _, dbtable := range opts.ExcludeTablesData {
				if strings.HasPrefix(dbtable, database+".") {
					separatedDump = true
				}
			}
			if separatedDump {
				sa := append([]string{}, schemaArgs...)
				sa = append(sa, database)
				da := append([]string{}, dataArgs...)
				da = append(da, database)
				ta := append([]string{}, triggerArgs...)
				ta = append(ta, database)
				argsList = append(argsList, sa, da, ta)
			} else {
				ba := append([]string{}, baseArgs...)
				ba = append(ba, database)
				argsList = append(argsList, ba)
			}
		}
	} else {
		if len(opts.ExcludeTablesData) > 0 {
			sa := append([]string{}, schemaArgs...)
			da := append([]string{}, dataArgs...)
			ta := append([]string{}, triggerArgs...)
			argsList = append(argsList, sa, da, ta)
		} else {
			argsList = append(argsList, baseArgs)
		}
	}
	return argsList
}

// constructExcludedTables creates --ignore-table flags for excluded tables.
func constructExcludedTables(ctx context.Context, connFlags []string, patternsList []string) ([]string, error) {
	excludeFlags := []string{}
	for _, pattern := range patternsList {
		parts := strings.Split(pattern, ".")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid exclude pattern: %s. Expected 'database.table'", pattern)
		}
		dbName := parts[0]
		globPattern := parts[1]
		matchedTables, err := mysqlutil.GetTablesMatchingGlob(ctx, connFlags, dbName, globPattern)
		if err != nil {
			return nil, fmt.Errorf("retrieve tables for pattern %s: %w", pattern, err)
		}
		for _, table := range matchedTables {
			excludeFlags = append(excludeFlags, dbName+"."+table)
		}
	}
	return excludeFlags, nil
}

// Removes the DEFINER clauses from any .sql files generated
func (r *Runner) removeDefiners() error {
	// Match complete user@host accounts. Backtick-quoted parts may contain
	// whitespace, @ characters, and backticks escaped by doubling them.
	accountPart := "(?:`(?:``|[^`])*`|[^\\s`@]+)"
	definerRe := regexp.MustCompile(`DEFINER=` + accountPart + `@` + accountPart + `\s+`)
	for _, dumpFile := range r.OutputFiles {
		// Read entire file
		data, err := os.ReadFile(dumpFile)
		if err != nil {
			return fmt.Errorf("remove definers from %s: read dump: %w", dumpFile, err)
		}

		// Replace all occurrences
		processed := definerRe.ReplaceAll(data, []byte{})

		// If no change, skip rewrite
		if string(processed) == string(data) {
			logging.Info("No DEFINER clauses found in %s", dumpFile)
			continue
		}

		// Write to a temp file in same dir then atomically rename
		dir := filepath.Dir(dumpFile)
		tmp, err := os.CreateTemp(dir, filepath.Base(dumpFile)+".nodefiner-*")
		if err != nil {
			return fmt.Errorf("remove definers from %s: create temporary file: %w", dumpFile, err)
		}
		tmpPath := tmp.Name()
		if _, err := tmp.Write(processed); err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("remove definers from %s: write temporary file: %w", dumpFile, err)
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("remove definers from %s: close temporary file: %w", dumpFile, err)
		}
		if err := os.Rename(tmpPath, dumpFile); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("remove definers from %s: replace dump: %w", dumpFile, err)
		}
		logging.Info("Successfully removed DEFINER clauses from %s", dumpFile)
	}
	logging.Info("DEFINER clauses removal process completed.")
	return nil
}
