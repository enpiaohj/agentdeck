package app

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// diskFree 返回 path 所在卷的剩余可用字节。
//
// GetDiskFreeSpaceEx 要求一个已存在的路径，而迁移的目标目录可能还没建，
// 因此这里向上找到最近的已存在祖先再问它的卷：新目录必然落在同一个卷上。
func diskFree(path string) (int64, error) {
	probe := path
	if abs, err := filepath.Abs(path); err == nil {
		probe = abs
	}
	for {
		if info, err := os.Stat(probe); err == nil {
			if !info.IsDir() {
				probe = filepath.Dir(probe)
			}
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe || parent == "" {
			break
		}
		probe = parent
	}
	ptr, err := windows.UTF16PtrFromString(probe)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err = windows.GetDiskFreeSpaceEx(ptr, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return int64(free), nil
}
