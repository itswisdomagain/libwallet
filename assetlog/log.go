package assetlog

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jrick/logrotate/rotator"
)

const maxLogRolls = 8

// NewRotator initializes a rotating file logger.
func NewRotator(logDir, logFileName string) (*rotator.Rotator, error) {
	if err := os.MkdirAll(logDir, 0744); err != nil {
		return nil, fmt.Errorf("error creating log directory: %w", err)
	}

	logFilename := filepath.Join(logDir, logFileName)
	return rotator.New(logFilename, 32*1024, false, maxLogRolls)
}
