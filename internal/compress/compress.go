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
	"io"
	"os"
	"path/filepath"

	"mymagicdump/internal/logging"

	"github.com/dsnet/compress/bzip2"
)

func ApplyCompression(outputPrefix string, compressionType string, files []string) {
	if len(files) == 0 {
		logging.Info("No dump files produced; skipping compression.")
		return
	}
	switch compressionType {
	case "tgz":
		compressTgz(outputPrefix, files)
	case "tbz2":
		compressTbz2(outputPrefix, files)
	case "zip":
		compressZip(outputPrefix, files)
	case "none":
		return
	default:
		logging.Error("Unsupported compression type: %s. Skipping compression.", compressionType)
	}
}

func compressTgz(outputPrefix string, files []string) {
	logging.Info("Starting tar.gz compression for: %s", outputPrefix)
	out, err := os.Create(outputPrefix + ".tar.gz")
	if err != nil { logging.Error("Failed to create tar.gz file: %v", err); return }
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	tarw := tar.NewWriter(gz)
	defer tarw.Close()
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil { logging.Error("Cannot open %s for compression: %v", f, err); continue }
		fi, err := fh.Stat(); if err != nil { logging.Error("Stat %s failed: %v", f, err); fh.Close(); continue }
		hdr, err := tar.FileInfoHeader(fi, fi.Name()); if err != nil { logging.Error("Header for %s failed: %v", f, err); fh.Close(); continue }
		if err := tarw.WriteHeader(hdr); err != nil { logging.Error("Write header for %s failed: %v", f, err); fh.Close(); continue }
		if _, err := io.Copy(tarw, fh); err != nil { logging.Error("Copy %s failed: %v", f, err); fh.Close(); continue }
		fh.Close()
		if err := os.Remove(f); err != nil { logging.Warn("Failed to delete original %s after compression: %v", f, err) }
	}
	logging.Info("tar.gz compression completed successfully.")
}

func compressZip(outputPrefix string, files []string) {
	logging.Info("Starting zip compression for: %s", outputPrefix)
	out, err := os.Create(outputPrefix + ".zip")
	if err != nil { logging.Error("Failed to create zip file: %v", err); return }
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
	for _, f := range files {
		base := filepath.Base(f)
		fh, err := os.Open(f); if err != nil { logging.Error("Cannot open %s for zip: %v", f, err); continue }
		w, err := zw.Create(base); if err != nil { logging.Error("Create zip entry %s failed: %v", f, err); fh.Close(); continue }
		if _, err := io.Copy(w, fh); err != nil { logging.Error("Copy %s failed: %v", f, err); fh.Close(); continue }
		fh.Close()
		if err := os.Remove(f); err != nil { logging.Warn("Failed to delete original %s after compression: %v", f, err) }
	}
	logging.Info("Zip compression completed successfully.")
}

func compressTbz2(outputPrefix string, files []string) {
	logging.Info("Starting tar.bz2 compression for: %s", outputPrefix)
	out, err := os.Create(outputPrefix + ".tar.bz2")
	if err != nil { logging.Error("Failed to create tar.bz2 file: %v", err); return }
	defer out.Close()
	bz, err := bzip2.NewWriter(out, &bzip2.WriterConfig{Level: bzip2.BestCompression})
	if err != nil { logging.Error("Failed to init bzip2 writer: %v", err); return }
	defer bz.Close()
	tarw := tar.NewWriter(bz)
	defer tarw.Close()
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil { logging.Error("Cannot open %s for compression: %v", f, err); continue }
		fi, err := fh.Stat(); if err != nil { logging.Error("Stat %s failed: %v", f, err); fh.Close(); continue }
		hdr, err := tar.FileInfoHeader(fi, fi.Name()); if err != nil { logging.Error("Header for %s failed: %v", f, err); fh.Close(); continue }
		if err := tarw.WriteHeader(hdr); err != nil { logging.Error("Write header for %s failed: %v", f, err); fh.Close(); continue }
		if _, err := io.Copy(tarw, fh); err != nil { logging.Error("Copy %s failed: %v", f, err); fh.Close(); continue }
		fh.Close()
		if err := os.Remove(f); err != nil { logging.Warn("Failed to delete original %s after compression: %v", f, err) }
	}
	logging.Info("tar.bz2 compression completed successfully.")
}
