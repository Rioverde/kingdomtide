package simulation

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OpenLogFile creates a per-run log file under {workdir}/logs/sim/
// named simulation-{seed}-{timestamp}.log. Returns *os.File so the
// caller can pass it to WithLogger(...) and Close() it after Run.
//
// Caller is responsible for Close. On error returns (nil, err); the
// caller may fall back to a no-op writer.
func OpenLogFile(workdir string, seed int64) (*os.File, error) {
	logDir := filepath.Join(workdir, "logs", "sim")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	path := filepath.Join(logDir, fmt.Sprintf("simulation-%d-%s.log", seed, stamp))
	return os.Create(path)
}
