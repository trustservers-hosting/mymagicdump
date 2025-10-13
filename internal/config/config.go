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

package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jessevdk/go-flags"
)

type CommaSeparatedList []string

func (d *CommaSeparatedList) UnmarshalFlag(value string) error {
	*d = strings.Split(value, ",")
	return nil
}

type Options struct {
	User                string             `short:"u" long:"user" description:"MySQL username" value-name:"USER"`
	Password            string             `short:"p" long:"password" description:"MySQL password" value-name:"PASSWORD"`
	Host                string             `short:"h" long:"host" description:"MySQL host address" value-name:"HOST"`
	Port                string             `short:"P" long:"port" description:"MySQL port" value-name:"PORT"`
	Socket              string             `short:"s" long:"socket" description:"Path to MySQL socket" value-name:"SOCKET"`
	DefaultsFile        string             `long:"defaults-file" default:"~/.my.cnf" description:"Path to MySQL defaults file" value-name:"FILE"`
	DefaultsGroupSuffix string             `long:"defaults-group-suffix" description:"Suffix to append to the default group name in the MySQL configuration file"`
	AllDatabases        bool               `long:"all-databases" description:"Dump all databases"`
	Databases           CommaSeparatedList `long:"databases" description:"Comma-separated list of databases to dump. Supports glob patterns (* and ?) per entry." value-name:"DATABASE1,DATABASE2"`
	SeparateDumps       bool               `long:"separate-dumps" description:"Create separate dump files for each database provided with --databases"`
	ExcludeTables       CommaSeparatedList `long:"exclude" description:"Comma-separated list of tables to exclude. Supports glob patterns (* and ?)." value-name:"DB1.TABLE1,DB2.TABLE2"`
	ExcludeTablesData   CommaSeparatedList `long:"exclude-data" description:"Comma-separated list of tables to exclude data from (but keep the schema). Supports glob patterns (* and ?)." value-name:"DB1.TABLE1,DB2.TABLE2"`
	OutputPath          string             `long:"output" default:"./" description:"Output file path" value-name:"PATH"`
	Compression         string             `long:"compression" default:"none" description:"Compression type (tgz, tbz2, zip, none)" choice:"tgz" choice:"tbz2" choice:"zip" choice:"none"`
	DryRun              bool               `long:"dry-run" description:"Simulate the dump process"`
	RemoveDefiners      bool               `long:"remove-definers" description:"Remove definer statements"`
	Retries             int                `long:"retries" default:"3" description:"Number of retries on failure" value-name:"NUM_RETRIES"`
	RetryInterval       int                `long:"retry-interval" default:"30" description:"Seconds between retries" value-name:"SECONDS"`
	NotifyEmail         string             `long:"notify" description:"Email to send notifications" value-name:"EMAIL_ADDRESS"`
	Silent              bool               `short:"q" long:"silent" description:"Only print errors to stderr"`
	Verbose             bool               `short:"v" long:"verbose" description:"Enable verbose (debug) logging"`
	ShowVersion         bool               `long:"version" description:"Show version and exit"`
	// Passthrough holds any flags/args not recognized by our parser that should be forwarded to mysqldump
	Passthrough []string `no-flag:"true"`
}

func ParseArgs() (*Options, error) {
	var opts Options
	// Ignore unknown flags so we can forward them to mysqldump
	parser := flags.NewParser(&opts, flags.Default|flags.IgnoreUnknown)
	parser.Name = "mymagicdump"
	parser.ShortDescription = "TrustServers MySQL backup tool using mysqldump with exclusions, retries and compression."
	parser.LongDescription = "A fast, scriptable MySQL backup tool built on mysqldump. Supports multiple databases, table/data exclusions, compression (tgz/zip), retries, and optional DEFINER removal."
	// Parse and capture leftover args (unknown flags/positional)
	rest, err := parser.Parse()
	if err != nil {
		return nil, err
	}
	// Collect passthrough flags: include args that look like flags and their immediate value if separated
	// e.g., [--no-tablespaces, --where, "user='bob'"] -> pass both tokens for --where
	var pass []string
	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		if strings.HasPrefix(tok, "-") { // looks like a flag
			pass = append(pass, tok)
			if i+1 < len(rest) && !strings.HasPrefix(rest[i+1], "-") {
				pass = append(pass, rest[i+1])
				i++
			}
		}
	}
	opts.Passthrough = pass
	return &opts, nil
}

// Expands a leading ~ to the user's home directory
func ExpandTilde(p string) string {
	if p == "~" {
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
		return p
	}
	return p
}
