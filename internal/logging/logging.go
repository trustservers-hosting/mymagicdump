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

package logging

import "log"

var silent bool
var verbose bool

func SetVerbosity(silentMode bool, verboseMode bool) {
	silent = silentMode
	verbose = verboseMode
}

func Info(format string, args ...any) {
	if !silent {
		log.Printf("[INFO] "+format, args...)
	}
}

func Warn(format string, args ...any) {
	if !silent {
		log.Printf("[WARNING] "+format, args...)
	}
}

func Debug(format string, args ...any) {
	if verbose && !silent {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func Error(format string, args ...any) {
	log.Printf("[ERROR] "+format, args...)
}
