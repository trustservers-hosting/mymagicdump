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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"

	"mymagicdump/internal/compress"
	"mymagicdump/internal/config"
	"mymagicdump/internal/logging"
	"mymagicdump/internal/mysqlutil"
)

type Runner struct {
	Opts                 *config.Options
	ConnFlags            []string
	DumpFlagsList        [][]string
	OutputFiles          []string
}

func NewRunner(opts *config.Options) *Runner {
	return &Runner{Opts: opts}
}

func (r *Runner) Prepare() error {
	r.ConnFlags = mysqlutil.BuildConnectionFlags(*r.Opts)
	// Expand potential glob patterns in database list unless dumping all databases
	if !r.Opts.AllDatabases && len(r.Opts.Databases) > 0 {
		expanded, err := mysqlutil.ExpandDatabaseList(r.ConnFlags, r.Opts.Databases)
		if err != nil {
			logging.Warn("Failed to expand database patterns: %v", err)
		} else {
			r.Opts.Databases = expanded
		}
		if len(r.Opts.Databases) == 0 {
			logging.Error("No databases matched the provided patterns.")
			return fmt.Errorf("no databases matched provided patterns")
		}
	}
	// compute tables based on patterns
	excluded := constructExcludedTables(r.ConnFlags, r.Opts.ExcludeTables)
	excludedData := constructExcludedTables(r.ConnFlags, r.Opts.ExcludeTablesData)
	// build dump flags
	r.DumpFlagsList = buildDumpFlags(*r.Opts, excluded, excludedData)
	return nil
}

func (r *Runner) Run() error {
	if r.Opts.DryRun { logging.Info("Dry-run mode enabled. No commands will be executed.") }
OuterLoop:
	for _, mysqlDumpFlags := range r.DumpFlagsList {
		targetDatabases := mysqlutil.ExtractDatabasesFromFlags(mysqlDumpFlags)
		dbSize, err := mysqlutil.CalculateDatabaseSize(r.ConnFlags, targetDatabases)
		if err != nil {
			logging.Warn("Error calculating expected dump size: %v", err)
		} else {
			logging.Info("Estimated database size: %d bytes", dbSize)
		}
		logging.Info("Starting backup for databases: %s", strings.Join(targetDatabases, ", "))
	NextTry:
		for i := 0; i <= r.Opts.Retries; i++ {
			logging.Info("Attempt %d/%d for dumping database(s)", i+1, r.Opts.Retries+1)
			
			// Prepare the dump command, appending any passthrough flags
			mysqldumpArgs := append([]string{}, r.ConnFlags...)
			if len(r.Opts.Passthrough) > 0 {
				mysqldumpArgs = append(mysqldumpArgs, r.Opts.Passthrough...)
			}
			mysqldumpArgs = append(mysqldumpArgs, mysqlDumpFlags...)
			// Resolve mysqldump binary from PATH with fallback to mariadb-dump
			BinaryPath := ""
			if path, err := exec.LookPath("mysqldump"); err == nil {
				BinaryPath = path
			} else if path, err := exec.LookPath("mariadb-dump"); err == nil {
				BinaryPath = path
			} else {
				logging.Error("mymagicdump is a mysqldump/mariadb-dump wrapper tool, cannot find mysqldump or mariadb-dump in PATH.")
				continue NextTry
			}
			dumpCmd := exec.Command(BinaryPath, mysqldumpArgs...)
			logging.Debug("Executing command: %s", strings.Join(dumpCmd.Args, " "))

			// Exit early if in dry-run mode
			if r.Opts.DryRun { continue OuterLoop }
			
			 // Create output file
			os.MkdirAll(r.Opts.OutputPath, os.ModePerm)
			outputFilePath := filepath.Join(r.Opts.OutputPath, outputNameFromFlags(r.Opts, mysqlDumpFlags))
			outf, err := os.Create(outputFilePath)
			if err != nil { logging.Error("Failed to create output file %s: %v", outputFilePath, err); continue NextTry }
			defer outf.Close()

			// Set output and error streams for the command
			dumpCmd.Stdout = outf
			dumpCmd.Stderr = os.Stderr

			// Start and wait for the command to complete
			startTime := time.Now()
			if err = dumpCmd.Start(); err != nil { logging.Error("Failed to start mysqldump: %v", err); continue NextTry }
			logging.Info("Dump process started...")
			
			// Use a channel to detect when `mysqldump` completes
			done := make(chan error, 1)
			go func(){ done <- dumpCmd.Wait() }()

			// Create a progress bar 
			var bar *progressbar.ProgressBar
			if !r.Opts.Silent {
				bar = progressbar.DefaultBytes(int64(dbSize), "Dumping database...")
			}

			// Monitor file size and update progress bar
			for {
				select {
				case err := <-done:
					if exiterr, ok := err.(*exec.ExitError); ok { logging.Error("mysqldump exited with status %d.", exiterr.ExitCode()); continue NextTry }
					elapsed := time.Since(startTime)
					// Mark success and record output
					r.OutputFiles = append(r.OutputFiles, outputFilePath)
					if bar != nil { bar.Finish() }
					logging.Info("Dump completed successfully in %s", elapsed)
					continue OuterLoop
				default:
					if fi, err := os.Stat(outputFilePath); err == nil && bar != nil { bar.Set64(fi.Size()) }
					time.Sleep(time.Second)
				}
			}
		}
		logging.Error("Backup failed after all retries.")
	}
	// post-process
	if r.Opts.DryRun { return nil }
	if r.Opts.RemoveDefiners { r.removeDefiners() }
	// compression prefix
	prefix := compressionPrefix(r.Opts, r.OutputFiles)
	compress.ApplyCompression(prefix, r.Opts.Compression, r.OutputFiles)
	return nil
}

func outputNameFromFlags(opts *config.Options, mysqlDumpFlags []string) string {
	if opts.SeparateDumps {
		currDatabase := mysqlDumpFlags[len(mysqlDumpFlags)-1]
		if slices.Contains(mysqlDumpFlags, "--no-data") { currDatabase = currDatabase + "_schema" }
		if slices.Contains(mysqlDumpFlags, "--no-create-info") { currDatabase = currDatabase + "_data" }
		return currDatabase + ".sql"
	}
	if len(opts.Databases) == 1 { return opts.Databases[0] + ".sql" }
	return "multiple_databases.sql"
}

func compressionPrefix(opts *config.Options, files []string) string {
	if len(files) == 1 { return files[0] }
	return filepath.Join(opts.OutputPath, "multiple_databases")
}

func buildDumpFlags(opts config.Options, excludedTables, excludedTablesData []string) [][]string {
	argsList := [][]string{}
	baseArgs := []string{}
	schemaArgs := []string{}
	dataArgs := []string{}


	// Common arguments for all dumps
	for _, table := range excludedTables { baseArgs = append(baseArgs, "--ignore-table="+table) }
	if opts.AllDatabases {
		baseArgs = append(baseArgs, "--all-databases")
	} else if !opts.SeparateDumps {
		baseArgs = append(baseArgs, "--databases")
		baseArgs = append(baseArgs, opts.Databases...)
	}

	// Create schema only and data only arguments, if ExcludeTablesData is set
	if len(opts.ExcludeTablesData) > 0 {
		schemaArgs = append([]string{}, baseArgs...)
		schemaArgs = append(schemaArgs, "--no-data", "--skip-triggers")
		dataArgs = append([]string{}, baseArgs...)
		dataArgs = append(dataArgs, "--no-create-info")
		for _, t := range excludedTablesData { dataArgs = append(dataArgs, "--ignore-table="+t) }
	}
	// Remove "--triggers" if it exists
	for i, arg := range schemaArgs {
		if arg == "--triggers" { schemaArgs = append(schemaArgs[:i], schemaArgs[i+1:]...); break }
	}

	// If SeparateDumps is enabled, create individual dump commands for each database
	if opts.SeparateDumps {
		for _, database := range opts.Databases {
			// Check to see if we need separate schema/data for this database
			separatedDump := false
			for _, dbtable := range opts.ExcludeTablesData { if strings.HasPrefix(dbtable, database+".") { separatedDump = true } }
			if separatedDump {
				sa := append([]string{}, schemaArgs...); sa = append(sa, database)
				da := append([]string{}, dataArgs...); da = append(da, database)
				argsList = append(argsList, sa, da)
			} else {
				ba := append([]string{}, baseArgs...); ba = append(ba, database)
				argsList = append(argsList, ba)
			}
		}
	} else {
		if len(opts.ExcludeTablesData) > 0 {
			sa := append([]string{}, schemaArgs...)
			da := append([]string{}, dataArgs...)
			argsList = append(argsList, sa, da)
		} else {
			argsList = append(argsList, baseArgs)
		}
	}
	return argsList
}

// constructExcludedTables creates --ignore-table flags for excluded tables.
func constructExcludedTables(connFlags []string, patternsList []string) []string {
	excludeFlags := []string{}
	for _, pattern := range patternsList {
		parts := strings.Split(pattern, ".")
		if len(parts) != 2 { logging.Error("Invalid exclude pattern: %s. Expected 'database.table'", pattern); continue }
		dbName := parts[0]
		globPattern := parts[1]
		matchedTables, err := mysqlutil.GetTablesMatchingGlob(connFlags, dbName, globPattern)
		if err != nil { logging.Error("Error retrieving tables for pattern %s: %v", pattern, err); continue }
		for _, table := range matchedTables { excludeFlags = append(excludeFlags, dbName+"."+table) }
	}
	return excludeFlags
}

// Removes the DEFINER clauses from any .sql files generated
func (r *Runner) removeDefiners() {
	// Remove `DEFINER=... `
	definerRe := regexp.MustCompile(`DEFINER=[^\s]+ `)
	for _, dumpFile := range r.OutputFiles {
		// Read entire file
		data, err := os.ReadFile(dumpFile)
		if err != nil {
			logging.Error("Couldn't remove Definers from %s: Failed to read: %v", dumpFile, err)
			continue
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
			logging.Error("Failed to create temp file for %s: %v", dumpFile, err)
			return
		}
		tmpPath := tmp.Name()
		if _, err := tmp.Write(processed); err != nil {
			tmp.Close()
			os.Remove(tmpPath)
			logging.Error("Failed to write temp file for %s: %v", dumpFile, err)
			return
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmpPath)
			logging.Error("Failed to close temp file for %s: %v", dumpFile, err)
			return
		}
		if err := os.Rename(tmpPath, dumpFile); err != nil {
			os.Remove(tmpPath)
			logging.Error("Failed to replace original file %s: %v", dumpFile, err)
			return
		}
		logging.Info("Successfully removed DEFINER clauses from %s", dumpFile)
	}
	logging.Info("DEFINER clauses removal process completed.")
}
