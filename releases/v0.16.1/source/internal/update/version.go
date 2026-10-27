// Package update 检查 GitHub Releases 上是否有比当前更新的正式版本，
// 并在用户显式操作时把分发包下载到本地。
//
// 边界：只读公开的 release 接口与资产；不自动替换正在运行的 exe、不启动
// 下载到的程序。产品未做代码签名，因此「校验」只保证下载完整、与发布方
// 公布的 SHA-256 一致，不构成发布者身份证明。
package update

import (
	"fmt"
	"strconv"
	"strings"
)

// Version 是 MAJOR.MINOR.PATCH 加可选预发布后缀的版本号。
type Version struct {
	Major, Minor, Patch int
	Pre                 string // 预发布后缀（如 beta.1），空表示正式版
}

// ParseVersion 解析版本号，接受带或不带前导 v 的写法。
func ParseVersion(text string) (Version, error) {
	raw := strings.TrimSpace(text)
	raw = strings.TrimPrefix(strings.TrimPrefix(raw, "v"), "V")
	if raw == "" {
		return Version{}, fmt.Errorf("版本号为空")
	}
	core, pre, hasPre := strings.Cut(raw, "-")
	if hasPre {
		pre = strings.TrimSpace(pre)
	}
	parts := strings.Split(core, ".")
	if len(parts) > 3 {
		return Version{}, fmt.Errorf("版本号格式无法识别：%s", text)
	}
	numbers := make([]int, 3)
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("版本号格式无法识别：%s", text)
		}
		numbers[i] = n
	}
	return Version{Major: numbers[0], Minor: numbers[1], Patch: numbers[2], Pre: pre}, nil
}

// Compare 比较两个版本：a 比 b 新返回正数，相同返回 0，更旧返回负数。
// 预发布版本低于同号的正式版（1.0.0-beta < 1.0.0）。
func Compare(a, b Version) int {
	for _, pair := range [][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if pair[0] != pair[1] {
			if pair[0] > pair[1] {
				return 1
			}
			return -1
		}
	}
	switch {
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	default:
		// 预发布后缀按字节序比较即可满足「比大小」的需要：同一发布线上
		// 的后缀（beta.1 / beta.2）字典序与数字序一致。
		if a.Pre > b.Pre {
			return 1
		}
		return -1
	}
}

// String 返回规范写法（不带前导 v）。
func (v Version) String() string {
	text := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		text += "-" + v.Pre
	}
	return text
}

// IsNewer 判断 candidate 是否比 current 新。任一无法解析时返回 false——
// 版本来源不可信时宁可少提示，也不误报「有新版本」。
func IsNewer(candidate, current string) bool {
	next, err := ParseVersion(candidate)
	if err != nil {
		return false
	}
	now, err := ParseVersion(current)
	if err != nil {
		return false
	}
	return Compare(next, now) > 0
}
