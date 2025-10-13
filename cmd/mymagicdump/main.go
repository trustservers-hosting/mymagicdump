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

package main

import (
	"fmt"
	"os"

	"github.com/trustservers-hosting/mymagicdump/internal/config"
	"github.com/trustservers-hosting/mymagicdump/internal/dumper"
	"github.com/trustservers-hosting/mymagicdump/internal/logging"
	"github.com/trustservers-hosting/mymagicdump/internal/version"
)

func main() {
	opts, err := config.ParseArgs()
	if err != nil {
		os.Exit(1)
	}
	if opts.ShowVersion {
		fmt.Fprintf(os.Stdout, "mymagicdump %s\n", version.String())
		return
	}
	// Startup banner (respect --silent)
	if !opts.Silent {
		fmt.Fprintf(os.Stdout, "mymagicdump Version %s\n", version.Version)
		fmt.Fprintf(os.Stdout, "Copyright (c) 2025 TrustServers PC\n\n")
	}
	logging.SetVerbosity(opts.Silent, opts.Verbose)
	r := dumper.NewRunner(opts)
	if err := r.Prepare(); err != nil {
		logging.Error("Prepare failed: %v", err)
		os.Exit(1)
	}
	if err := r.Run(); err != nil {
		os.Exit(1)
	}
}
