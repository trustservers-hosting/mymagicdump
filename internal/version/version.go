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

package version

import (
	"runtime/debug"
)

var (
	// Version is the semantic version, set via -ldflags at build time
	Version = "dev"
	// Commit is the git commit SHA, set via -ldflags
	Commit = ""
	// Date is the build date/time in ISO 8601, set via -ldflags
	Date = ""
)

// init provides a best-effort fallback for embedding version information when
// building without -ldflags (e.g., `go install` or `go build` outside of Goreleaser).
//
// It reads Go's build info to populate Version, Commit, and Date if they were
// not set by -ldflags. When building from a module version (e.g., `@v1.2.3`),
// info.Main.Version will contain the semver. When building from a local checkout,
// it will usually be "(devel)" and we keep the default values.
func init() {
	// If ldflags already provided a concrete version (not dev/devel), do nothing.
	if Version != "" && Version != "dev" && Version != "(devel)" {
		return
	}

	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return
	}

	// Module semantic version, if available (e.g., v1.0.0 or a pseudo-version)
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = info.Main.Version
	}
}

func String() string {
	s := Version
	if Commit != "" {
		s += " (" + Commit + ")"
	}
	if Date != "" {
		s += " built " + Date
	}
	return s
}
