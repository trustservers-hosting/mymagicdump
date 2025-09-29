#   mymagicdump
#	
#	A fast, scriptable MySQL/MariaDB backup wrapper around mysqldump 
#	with exclusions, retries, progress, optional compression and more
#
#	Copyright (C) 2025 Trustservers PC
#
#	This program is free software: you can redistribute it and/or modify
#	it under the terms of the GNU General Public License as published by
#	the Free Software Foundation, either version 3 of the License, or
#	(at your option) any later version.
#
#	This program is distributed in the hope that it will be useful,
#	but WITHOUT ANY WARRANTY; without even the implied warranty of
#	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
#	GNU General Public License for more details.
#
#	You should have received a copy of the GNU General Public License
#	along with this program.  If not, see <https://www.gnu.org/licenses/>.

.PHONY: help build version clean tag tag-push

BINARY := mymagicdump
PKG := ./cmd/mymagicdump
VERSION_PKG := mymagicdump/internal/version

# Derive values from git when available
VER ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X '$(VERSION_PKG).Version=$(VER)' -X '$(VERSION_PKG).Commit=$(COMMIT)' -X '$(VERSION_PKG).Date=$(DATE)'

help:
	@echo "Common targets:"
	@echo "  make build                                 # Build $(BINARY) with version metadata from git"
	@echo "  make version                               # Print embedded version"
	@echo "  make tag VERSION=v1.2.0 MESSAGE='Release'  # Create git tag"
	@echo "  make tag-push                              # Push tags"
	@echo "  make clean                                 # Remove binary"

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

version: build
	./$(BINARY) --version

clean:
	go clean
	rm -f $(BINARY)

# Tagging helpers
# Usage: make tag VERSION=v0.2.0 MESSAGE='Release v0.2.0'
tag:
	@if [ -z "$(VERSION)" ]; then echo "ERROR: set VERSION=vX.Y.Z"; exit 1; fi
	git tag -a $(VERSION) -m "$(or $(MESSAGE),$(VERSION))"

tag-push:
	git push --tags
