package builder

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
)

// readHead returns the first limit bytes of a file as text, trimmed at a rune
// boundary so the JSON report never carries a broken UTF-8 sequence.
func readHead(path string, limit int) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	buffer := make([]byte, limit)
	read, err := file.Read(buffer)
	if read <= 0 || (err != nil && read == 0) {
		return ""
	}
	text := string(trimPartialRune(buffer[:read]))
	if fileSize(path) > int64(read) {
		text += "\n…"
	}
	return text
}

// trimPartialRune drops a trailing byte sequence that would decode as an
// incomplete UTF-8 rune.
func trimPartialRune(data []byte) []byte {
	for i := len(data) - 1; i >= 0 && i > len(data)-4; i-- {
		if data[i]&0xC0 != 0x80 {
			if expectedRuneLength(data[i]) > len(data)-i {
				return data[:i]
			}
			break
		}
	}
	return data
}

func expectedRuneLength(lead byte) int {
	switch {
	case lead&0x80 == 0:
		return 1
	case lead&0xE0 == 0xC0:
		return 2
	case lead&0xF0 == 0xE0:
		return 3
	case lead&0xF8 == 0xF0:
		return 4
	default:
		return 1
	}
}

func addFile(writer *zip.Writer, name, path string) error {
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer source.Close()
	entry, err := writer.Create(name)
	if err != nil {
		return err
	}
	if _, err := io.Copy(entry, source); err != nil {
		return fmt.Errorf("pack %s: %w", name, err)
	}
	return nil
}

func copyFile(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destination, io.LimitReader(source, maxTestBytes)); err != nil {
		destination.Close()
		return err
	}
	return destination.Close()
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
