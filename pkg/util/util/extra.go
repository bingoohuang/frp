package util

import (
	"os"
	"path/filepath"
	"strings"
)

var Home string

func init() {
	Home, _ = os.UserHomeDir()
}

func ExpandFile(path string) string {
	if path == "" {
		return path
	}

	if strings.HasPrefix(path, "~") {
		path = filepath.Join(Home, path[1:])
	}

	if f, err := os.Lstat(path); err == nil && f.Mode()&os.ModeSymlink != 0 {
		if l, err := os.Readlink(path); err == nil {
			path = l
		}
	}
	return path
}
