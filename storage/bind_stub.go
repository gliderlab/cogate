//go:build !binddb
// +build !binddb

package storage

// BindExecutable is a no-op when built without the binddb tag.
func BindExecutable(s *Storage, dbPath string) error {
	return nil
}
