package app

// utf16Count 将 Go 原文转换成浏览器字符串所用的 UTF-16 偏移。
func utf16Count(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}
