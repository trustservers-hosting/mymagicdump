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

var (
    // Version is the semantic version, set via -ldflags at build time
    Version = "dev"
    // Commit is the git commit SHA, set via -ldflags
    Commit = ""
    // Date is the build date/time in ISO 8601, set via -ldflags
    Date = ""
)

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
