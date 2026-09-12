package tools

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func DownloadFile(fileURL, destPath string) string {
	if fileURL == "" {
		return "[Error] url is required"
	}
	if destPath == "" {
		return "[Error] path is required"
	}

	destPath = filepath.Clean(destPath)
	if destPath == "." || destPath == ".." || strings.HasPrefix(destPath, ".."+string(filepath.Separator)) || strings.HasPrefix(filepath.ToSlash(destPath), "../") {
		return "[Error] relative path traversal detected in destination path"
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("[Error] failed to create directory: %v", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Sprintf("[Error] failed to create file: %v", err)
	}
	defer out.Close()

	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Get(fileURL)
	if err != nil {
		return fmt.Sprintf("[Error] download failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("[Error] HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Sprintf("[Error] failed to write file: %v", err)
	}

	return fmt.Sprintf("✔ Downloaded: %s\n   Path: %s\n   Size: %s\n   URL: %s",
		filepath.Base(destPath), destPath, formatFileSize(written), fileURL)
}

func formatFileSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
