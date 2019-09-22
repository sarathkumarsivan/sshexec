package ioutil

import "os"

func IsDir(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func IsFile(path string) bool {
	return !IsDir(path)
}
