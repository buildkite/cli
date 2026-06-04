//go:build windows

package keyring

import "os"

func fileOwnerUID(os.FileInfo) (uint32, bool) {
	return 0, false
}

func lockFile(*os.File) error {
	return nil
}

func unlockFile(*os.File) error {
	return nil
}
