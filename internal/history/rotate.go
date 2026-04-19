package history

import (
	"errors"
	"fmt"
	"os"
)

// RotateIfNeeded rotates history.jsonl when it exceeds maxSizeMB MB.
// Keeps up to maxRotations generations (history.jsonl.1, .2, ...).
func RotateIfNeeded(maxSizeMB int, maxRotations int) error {
	path, err := Path()
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() < int64(maxSizeMB)*1024*1024 {
		return nil
	}

	// Shift .N → .N+1 from the back; oldest is lost.
	for i := maxRotations - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", path, i)
		to := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := os.Stat(from); err == nil {
			_ = os.Rename(from, to)
		}
	}
	return os.Rename(path, path+".1")
}
