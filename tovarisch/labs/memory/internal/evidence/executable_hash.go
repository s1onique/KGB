package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// executableSHA256 hashes the running executable. It remains a focused helper
// because final-evidence tests assert that the production path uses the actual
// process image rather than a caller-provided hash.
func executableSHA256() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
