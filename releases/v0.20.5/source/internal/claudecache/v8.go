package claudecache

import (
	"encoding/binary"
	"errors"
	"math"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/klauspost/compress/snappy"
)

// IndexedDB 的值是 Blink 包一层、里面是 V8 结构化克隆序列化的 JS 值。这里只
// 解码 JSON 能表达的那部分（对象、数组、字符串、数字、布尔、null、Date、Map），
// 遇到 ArrayBuffer、宿主对象等直接报错：调用方按「这条不是会话」跳过，
// 不猜测着读。

const (
	maxDepth = 256
	// maxArray 限制单个数组的长度：声明的长度来自不可信输入，不能直接拿去分配
	maxArray = 1 << 20
)

var errValue = errors.New("无法解析的 IndexedDB 值")

// decodeValue 解开 IndexedDB 存的一个值：可选的 snappy 包装（ff 11 02），
// 然后是 Blink 信封（ff 版本、可选的 fe 尾部偏移）与 V8 头（ff 版本）。
func decodeValue(raw []byte) (any, error) {
	if len(raw) >= 3 && raw[0] == 0xff && raw[1] == 0x11 && raw[2] == 0x02 {
		n, err := snappy.DecodedLen(raw[3:])
		if err != nil || n > maxFile {
			return nil, errValue
		}
		if raw, err = snappy.Decode(nil, raw[3:]); err != nil {
			return nil, err
		}
	}
	d := &v8{b: raw}
	return d.value(0)
}

type v8 struct {
	b    []byte
	i    int
	refs []any
}

func (d *v8) byte() (byte, error) {
	if d.i >= len(d.b) {
		return 0, errValue
	}
	c := d.b[d.i]
	d.i++
	return c, nil
}

func (d *v8) varint() (uint64, error) {
	v, n := binary.Uvarint(d.b[d.i:])
	if n <= 0 {
		return 0, errValue
	}
	d.i += n
	return v, nil
}

func (d *v8) raw(n uint64) ([]byte, error) {
	if n > uint64(len(d.b)-d.i) {
		return nil, errValue
	}
	out := d.b[d.i : d.i+int(n)]
	d.i += int(n)
	return out, nil
}

func (d *v8) peek() byte {
	if d.i < len(d.b) {
		return d.b[d.i]
	}
	return 0
}

// skip 吞掉填充字节、版本头与 Blink 尾部偏移，停在下一个真正的值标签上。
func (d *v8) skip() error {
	for d.i < len(d.b) {
		switch d.b[d.i] {
		case 0x00:
			d.i++
		case 0xff:
			d.i++
			if _, err := d.varint(); err != nil {
				return err
			}
		case 0xfe:
			if _, err := d.raw(13); err != nil {
				return err
			}
		default:
			return nil
		}
	}
	return errValue
}

func (d *v8) value(depth int) (any, error) {
	if depth > maxDepth {
		return nil, errValue
	}
	if err := d.skip(); err != nil {
		return nil, err
	}
	tag, _ := d.byte()
	switch tag {
	case '_', '0', '-': // undefined, null, 数组空洞
		return nil, nil
	case 'T':
		return true, nil
	case 'F':
		return false, nil
	case 'I':
		v, err := d.varint()
		return float64(int64(v>>1) ^ -int64(v&1)), err
	case 'U':
		v, err := d.varint()
		return float64(v), err
	case 'N':
		b, err := d.raw(8)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil
	case '"': // Latin-1
		n, err := d.varint()
		if err != nil {
			return nil, err
		}
		b, err := d.raw(n)
		if err != nil {
			return nil, err
		}
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		return string(r), nil
	case 'c': // UTF-16LE
		n, err := d.varint()
		if err != nil || n%2 != 0 {
			return nil, errValue
		}
		b, err := d.raw(n)
		if err != nil {
			return nil, err
		}
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(b[2*i:])
		}
		return string(utf16.Decode(u)), nil
	case 'S':
		n, err := d.varint()
		if err != nil {
			return nil, err
		}
		b, err := d.raw(n)
		if err != nil || !utf8.Valid(b) {
			return nil, errValue
		}
		return string(b), nil
	case '^':
		n, err := d.varint()
		if err != nil || n >= uint64(len(d.refs)) {
			return nil, errValue
		}
		return d.refs[n], nil
	case 'D':
		b, err := d.raw(8)
		if err != nil {
			return nil, err
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(b))
		d.refs = append(d.refs, v)
		return v, nil
	case 'o':
		o := map[string]any{}
		d.refs = append(d.refs, o)
		return o, d.props(o, '{', depth)
	case ';': // Map：键值交替，按字符串键收进对象
		o := map[string]any{}
		d.refs = append(d.refs, o)
		for {
			if err := d.skip(); err != nil {
				return nil, err
			}
			if d.peek() == ':' {
				d.i++
				_, err := d.varint()
				return o, err
			}
			k, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			v, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			o[key(k)] = v
		}
	case 'A':
		n, err := d.varint()
		if err != nil || n > uint64(len(d.b)-d.i) || n > maxArray {
			return nil, errValue
		}
		a := make([]any, 0, n)
		ref := len(d.refs)
		d.refs = append(d.refs, a)
		for k := uint64(0); k < n; k++ {
			v, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		d.refs[ref] = a
		if err := d.props(map[string]any{}, '$', depth); err != nil {
			return nil, err
		}
		_, err = d.varint() // length
		return a, err
	case 'a':
		n, err := d.varint()
		if err != nil || n > uint64(len(d.b)-d.i) || n > maxArray {
			return nil, errValue
		}
		ref := len(d.refs)
		d.refs = append(d.refs, []any{})
		props := map[string]any{}
		if err := d.props(props, '@', depth); err != nil {
			return nil, err
		}
		if _, err := d.varint(); err != nil {
			return nil, err
		}
		a := make([]any, n)
		for k, v := range props {
			if i, err := strconv.ParseUint(k, 10, 64); err == nil && i < n {
				a[i] = v
			}
		}
		d.refs[ref] = a
		return a, nil
	default:
		return nil, errValue
	}
}

// props 读对象属性直到结束标签，结束标签后跟属性个数。
func (d *v8) props(o map[string]any, end byte, depth int) error {
	for {
		if err := d.skip(); err != nil {
			return err
		}
		if d.peek() == end {
			d.i++
			_, err := d.varint()
			return err
		}
		k, err := d.value(depth + 1)
		if err != nil {
			return err
		}
		v, err := d.value(depth + 1)
		if err != nil {
			return err
		}
		o[key(k)] = v
	}
}

func key(k any) string {
	switch v := k.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}
