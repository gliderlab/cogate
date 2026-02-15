//go:build binddb
// +build binddb

package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BindExecutable binds the SQLite db to the current executable by storing exe hash + instance key.
// If bound to another executable, returns error unless rebind flag is provided via env OPENCLAW_BIND_REBIND=true.
func BindExecutable(s *Storage, dbPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable failed: %v", err)
	}
	exePath, _ = filepath.Abs(exePath)
	exeHash, err := fileHash(exePath)
	if err != nil {
		return fmt.Errorf("hash executable failed: %v", err)
	}

	storedHash, err := s.GetConfig("binding", "exe_hash")
	if err != nil {
		return fmt.Errorf("read binding hash failed: %v", err)
	}

	if storedHash == "" {
		key := randomKey(32)
		if err := s.SetConfig("binding", "exe_hash", exeHash); err != nil {
			return err
		}
		if err := s.SetConfig("binding", "exe_path", exePath); err != nil {
			return err
		}
		if err := s.SetConfig("binding", "instance_key", key); err != nil {
			return err
		}
		if err := s.SetConfig("binding", "bound_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
		return nil
	}

	if storedHash == exeHash {
		return nil
	}

	// mismatch
	rebind := os.Getenv("OPENCLAW_BIND_REBIND")
	if rebind != "true" {
		storedPath, _ := s.GetConfig("binding", "exe_path")
		return fmt.Errorf("database bound to another executable: %s (hash=%s), current=%s (hash=%s). Set OPENCLAW_BIND_REBIND=true to override",
			storedPath, storedHash, exePath, exeHash)
	}

	if err := s.SetConfig("binding", "exe_hash", exeHash); err != nil {
		return err
	}
	if err := s.SetConfig("binding", "exe_path", exePath); err != nil {
		return err
	}
	if err := s.SetConfig("binding", "rebound_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return nil
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func randomKey(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
