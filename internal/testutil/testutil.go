// Package testutil locates checked-in fixtures from tests in any package.
package testutil

import (
	"path/filepath"
	"runtime"
)

func DataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return filepath.Join(root, "testdata", name)
}
