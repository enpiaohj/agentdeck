//go:build windows

package app

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// exeVersion 读出当前可执行文件 VERSIONINFO 里的产品版本。
//
// 版本号的唯一来源是 build/windows/info.json：它经 wails3 generate syso
// 写进 exe 资源，这里再从资源读回来。Go 代码里因此不需要再维护一份常量，
// 也就不会出现「代码说 0.9.0、exe 属性说 0.8.0」的分裂。
//
// 读不到时返回空串（例如测试二进制或 go run），由调用方按「开发版本」处理。
func exeVersion() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	var handle windows.Handle
	size, err := windows.GetFileVersionInfoSize(exe, &handle)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err = windows.GetFileVersionInfo(exe, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	var info *windows.VS_FIXEDFILEINFO
	var length uint32
	if err = windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&info), &length); err != nil || info == nil {
		return ""
	}
	if info.Signature != 0xFEEF04BD {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d",
		info.ProductVersionMS>>16, info.ProductVersionMS&0xFFFF, info.ProductVersionLS>>16)
}
