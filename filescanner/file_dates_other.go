//go:build !darwin

package filescanner

import (
	"os"
	"time"
)

func lookupFilesystemBirthTime(_ string, _ os.FileInfo) (*time.Time, bool) {
	return nil, false
}
