package utils

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	gaba "github.com/UncleJunVIP/gabagool/v2/pkg/gabagool"
	"go.uber.org/atomic"
)

// progressWriter wraps a writer and updates progress as bytes are written
type progressWriter struct {
	writer         io.Writer
	totalBytes     uint64
	extractedBytes *uint64
	progress       *atomic.Float64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if n > 0 && pw.progress != nil && pw.totalBytes > 0 {
		*pw.extractedBytes += uint64(n)
		pw.progress.Store(float64(*pw.extractedBytes) / float64(pw.totalBytes))
	}
	return n, err
}

func TempDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return os.TempDir()
	}

	return filepath.Join(wd, ".tmp")
}

func DeleteFile(path string) bool {
	logger := gaba.GetLogger()

	err := os.Remove(path)
	if err != nil {
		logger.Error("Issue removing file",
			"path", path,
			"error", err)
		return false
	} else {
		logger.Debug("Removed file", "path", path)
		return true
	}
}

func Unzip(zipPath string, destDir string, progress *atomic.Float64) error {
	logger := gaba.GetLogger()

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Pre-allocate a larger buffer for file copying (128KB)
	// Reusing the same buffer across all files reduces allocations
	buffer := make([]byte, 128*1024)

	// Track created directories to avoid redundant MkdirAll calls
	createdDirs := make(map[string]bool)
	createdDirs[destDir] = true

	// Calculate total uncompressed size for progress tracking (no I/O overhead)
	var totalBytes uint64
	for _, file := range reader.File {
		if !file.FileInfo().IsDir() {
			totalBytes += file.UncompressedSize64
		}
	}

	var extractedBytes uint64

	for _, file := range reader.File {
		filePath := filepath.Join(destDir, file.Name)

		if file.FileInfo().IsDir() {
			if !createdDirs[filePath] {
				if err := os.MkdirAll(filePath, file.Mode()); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", filePath, err)
				}
				createdDirs[filePath] = true
			}
			continue
		}

		parentDir := filepath.Dir(filePath)
		if !createdDirs[parentDir] {
			if err := os.MkdirAll(parentDir, 0755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", filePath, err)
			}
			createdDirs[parentDir] = true
		}

		if err := extractFile(file, filePath, buffer, totalBytes, &extractedBytes, progress); err != nil {
			return fmt.Errorf("failed to extract file %s: %w", file.Name, err)
		}

		logger.Debug("Extracted file", "file", file.Name, "dest", filePath)
	}

	return nil
}

func extractFile(file *zip.File, destPath string, buffer []byte, totalBytes uint64, extractedBytes *uint64, progress *atomic.Float64) error {
	srcFile, err := file.Open()
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Use a buffered writer (64KB buffer) to reduce write syscalls
	bufWriter := bufio.NewWriterSize(destFile, 64*1024)
	defer bufWriter.Flush()

	// Wrap the buffered writer with progress tracking
	progressW := &progressWriter{
		writer:         bufWriter,
		totalBytes:     totalBytes,
		extractedBytes: extractedBytes,
		progress:       progress,
	}

	// Use the pre-allocated buffer for copying to reduce allocations
	_, err = io.CopyBuffer(progressW, srcFile, buffer)
	if err != nil {
		return err
	}

	// Ensure all data is flushed to disk
	return bufWriter.Flush()
}

// OrganizeMultiFileRomForMuOS reorganizes extracted multi-file ROM contents for muOS
// muOS expects: .m3u file in platform dir, disc files in _{gameName}/ subdirectory
func OrganizeMultiFileRomForMuOS(extractDir, romDirectory, gameName string) error {
	logger := gaba.GetLogger()

	// Find the .m3u file in the extracted directory
	var m3uFile string
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return fmt.Errorf("failed to read extracted directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".m3u" {
			m3uFile = filepath.Join(extractDir, entry.Name())
			break
		}
	}

	if m3uFile == "" {
		// No .m3u file found, this might not be a multi-disc game
		// Just rename the directory with underscore prefix
		underscoreDir := filepath.Join(romDirectory, "_"+gameName)
		if err := os.Rename(extractDir, underscoreDir); err != nil {
			return fmt.Errorf("failed to rename directory to %s: %w", underscoreDir, err)
		}
		logger.Debug("No .m3u file found, renamed directory", "from", extractDir, "to", underscoreDir)
		return nil
	}

	// Read the .m3u file contents
	m3uContent, err := os.ReadFile(m3uFile)
	if err != nil {
		return fmt.Errorf("failed to read .m3u file: %w", err)
	}

	// Update the .m3u file contents to reference _{gameName}/ subdirectory
	lines := strings.Split(string(m3uContent), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Prepend _{gameName}/ to the file reference
		lines[i] = filepath.Join("_"+gameName, line)
	}
	updatedM3U := strings.Join(lines, "\n")

	// Write the updated .m3u file to the platform directory
	m3uDestPath := filepath.Join(romDirectory, gameName+".m3u")
	if err := os.WriteFile(m3uDestPath, []byte(updatedM3U), 0644); err != nil {
		return fmt.Errorf("failed to write updated .m3u file: %w", err)
	}
	logger.Debug("Moved and updated .m3u file", "from", m3uFile, "to", m3uDestPath)

	// Remove the original .m3u file from the extracted directory
	if err := os.Remove(m3uFile); err != nil {
		logger.Warn("Failed to remove original .m3u file", "path", m3uFile, "error", err)
	}

	// Rename the extracted directory to have underscore prefix
	underscoreDir := filepath.Join(romDirectory, "_"+gameName)
	if err := os.Rename(extractDir, underscoreDir); err != nil {
		return fmt.Errorf("failed to rename directory to %s: %w", underscoreDir, err)
	}
	logger.Debug("Renamed directory for muOS", "from", extractDir, "to", underscoreDir)

	return nil
}
