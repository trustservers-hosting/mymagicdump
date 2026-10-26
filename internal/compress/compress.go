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

package compress

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/trustservers-hosting/mymagicdump/internal/logging"

	"github.com/dsnet/compress/bzip2"
)

// ApplyCompression removes source dumps only after the complete archive is closed.
func ApplyCompression(outputPrefix string, compressionType string, files []string) error {
	if len(files) == 0 {
		logging.Info("No dump files produced; skipping compression.")
		return nil
	}

	var extension string
	switch compressionType {
	case "tgz":
		extension = ".tar.gz"
	case "tbz2":
		extension = ".tar.bz2"
	case "zip":
		extension = ".zip"
	case "none":
		return nil
	default:
		return fmt.Errorf("unsupported compression type: %s", compressionType)
	}

	archivePath := outputPrefix + extension
	logging.Info("Starting %s compression for: %s", extension[1:], outputPrefix)
	if err := writeArchive(archivePath, compressionType, files); err != nil {
		return fmt.Errorf("compression failed for %s: %w", archivePath, err)
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			logging.Warn("Failed to delete original %s after compression: %v", f, err)
		}
	}
	logging.Info("%s compression completed successfully.", extension[1:])
	return nil
}

func writeArchive(path, compressionType string, files []string) (err error) {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	// Close from the archive writer outward, checking errors from buffered data
	// and archive trailers as well as from copying the source files.
	var writers []io.Closer
	defer func() {
		for i := len(writers) - 1; i >= 0; i-- {
			if closeErr := writers[i].Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("finalize archive: %w", closeErr))
			}
		}
		if err == nil {
			if syncErr := out.Sync(); syncErr != nil {
				err = fmt.Errorf("sync archive: %w", syncErr)
			}
		}
		if closeErr := out.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close archive: %w", closeErr))
		}
		if err != nil {
			if removeErr := os.Remove(path); removeErr != nil {
				err = errors.Join(err, fmt.Errorf("remove incomplete archive: %w", removeErr))
			}
		}
	}()

	var addFile func(string) error
	if compressionType == "zip" {
		zw := zip.NewWriter(out)
		writers = append(writers, zw)
		addFile = func(path string) error {
			return copyArchiveFile(path, func(_ os.FileInfo) (io.Writer, error) {
				return zw.Create(filepath.Base(path))
			})
		}
	} else {
		var compressed io.WriteCloser
		if compressionType == "tgz" {
			compressed = gzip.NewWriter(out)
		} else {
			compressed, err = bzip2.NewWriter(out, &bzip2.WriterConfig{Level: bzip2.BestCompression})
			if err != nil {
				return fmt.Errorf("initialize bzip2 writer: %w", err)
			}
		}
		writers = append(writers, compressed)
		tarw := tar.NewWriter(compressed)
		writers = append(writers, tarw)
		addFile = func(path string) error {
			return copyArchiveFile(path, func(fi os.FileInfo) (io.Writer, error) {
				hdr, err := tar.FileInfoHeader(fi, fi.Name())
				if err != nil {
					return nil, err
				}
				if err := tarw.WriteHeader(hdr); err != nil {
					return nil, err
				}
				return tarw, nil
			})
		}
	}
	for _, f := range files {
		if err := addFile(f); err != nil {
			return fmt.Errorf("archive %s: %w", f, err)
		}
	}
	return nil
}

func copyArchiveFile(path string, entry func(os.FileInfo) (io.Writer, error)) (err error) {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, fh.Close()) }()
	fi, err := fh.Stat()
	if err != nil {
		return err
	}
	w, err := entry(fi)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, fh)
	return err
}
