package repair

import "path/filepath"

func isAbs(p string) bool { return filepath.IsAbs(p) }
func joinPath(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}
