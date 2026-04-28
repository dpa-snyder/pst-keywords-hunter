//go:build darwin

package filescanner

import (
	"os"
	"syscall"
	"time"
)

func lookupFilesystemBirthTime(_ string, info os.FileInfo) (*time.Time, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, false
	}
	t := time.Unix(int64(stat.Birthtimespec.Sec), int64(stat.Birthtimespec.Nsec))
	return &t, true
}
