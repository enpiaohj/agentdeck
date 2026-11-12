package claudecache

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/snappy"
)

// 只读的 LevelDB 文件扫描：不打开数据库、不碰 LOCK / MANIFEST，只逐个读
// .log（写前日志）与 .ldb / .sst（SSTable），把每个键的最新版本挑出来。
// Chromium 的 IndexedDB 用自定义比较器 idb_cmp1，通用 LevelDB 库会拒绝打开，
// 而且打开就会改写文件；这里不需要键序，只需要「每个键最后写入的值」。

const maxFile = 256 << 20

type record struct {
	seq uint64
	val []byte // nil = 删除
}

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// crcOK 校验 LevelDB 的 masked CRC32C。
func crcOK(want uint32, parts ...[]byte) bool {
	c := uint32(0)
	for _, p := range parts {
		c = crc32.Update(c, castagnoli, p)
	}
	return (c>>15|c<<17)+0xa282ead8 == want
}

// scanLevelDB 返回目录中每个键的最新值（已删除的键不返回）。Desktop 运行中
// 会压缩合并：列目录之后某个文件可能已被删除、内容挪进了新文件，这时整体重扫，
// 免得这一轮漏掉一部分值。
func scanLevelDB(dir string) (map[string][]byte, error) {
	for attempt := 0; ; attempt++ {
		out, err := scanOnce(dir)
		if errors.Is(err, errVanished) && attempt < 3 {
			continue
		}
		if errors.Is(err, errVanished) {
			err = nil
		}
		return out, err
	}
}

var errVanished = errors.New("LevelDB 文件在扫描期间被移除")

func scanOnce(dir string) (map[string][]byte, error) {
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	latest := map[string]record{}
	put := func(key []byte, seq uint64, val []byte) {
		if prior, ok := latest[string(key)]; ok && prior.seq >= seq {
			return
		}
		latest[string(key)] = record{seq: seq, val: val}
	}
	var files []string
	for _, n := range names {
		ext := strings.ToLower(filepath.Ext(n.Name()))
		if n.Type().IsRegular() && (ext == ".log" || ext == ".ldb" || ext == ".sst") {
			files = append(files, n.Name())
		}
	}
	sort.Strings(files)
	var firstErr error
	vanished := false
	for _, name := range files {
		data, err := readSmall(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			vanished = true
			continue
		}
		if err == nil {
			if strings.EqualFold(filepath.Ext(name), ".log") {
				err = scanLog(data, put)
			} else {
				err = scanTable(data, put)
			}
		}
		// 单个文件坏了（正在被写、被压缩合并掉）不影响其他文件
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	out := make(map[string][]byte, len(latest))
	for k, r := range latest {
		if r.val != nil {
			out[k] = r.val
		}
	}
	if vanished {
		return out, errVanished
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func readSmall(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxFile {
		return nil, errors.New("文件过大，已跳过: " + path)
	}
	return os.ReadFile(path)
}

var errCorrupt = errors.New("LevelDB 数据损坏")

func uvarint(b []byte, i int) (uint64, int, error) {
	if i < 0 || i > len(b) {
		return 0, i, errCorrupt
	}
	v, n := binary.Uvarint(b[i:])
	if n <= 0 {
		return 0, i, errCorrupt
	}
	return v, i + n, nil
}

// bytesAt 读一个「varint 长度 + 内容」，长度越界即报损坏。
func bytesAt(b []byte, i int) ([]byte, int, error) {
	n, i, err := uvarint(b, i)
	if err != nil || n > uint64(len(b)-i) {
		return nil, i, errCorrupt
	}
	return b[i : i+int(n)], i + int(n), nil
}

// scanLog 读写前日志：32 KiB 一块，块内是带类型的物理记录，拼成逻辑记录后
// 是一个 WriteBatch。末尾写了一半的记录直接丢弃。
func scanLog(data []byte, put func([]byte, uint64, []byte)) error {
	const block = 32768
	var pending []byte
	for i := 0; i+7 <= len(data); {
		if left := block - i%block; left < 7 {
			i += left
			continue
		}
		sum := binary.LittleEndian.Uint32(data[i:])
		n := int(binary.LittleEndian.Uint16(data[i+4:]))
		kind := data[i+6]
		i += 7
		if n > len(data)-i {
			return nil
		}
		payload := data[i : i+n]
		i += n
		// 校验不过的记录（正在写的半截、块尾的预分配零）丢弃，连同未拼完的分片
		if kind == 0 || !crcOK(sum, []byte{kind}, payload) {
			pending = nil
			continue
		}
		switch kind {
		case 1: // FULL
			if err := scanBatch(payload, put); err != nil {
				return err
			}
			pending = nil
		case 2: // FIRST
			pending = append([]byte(nil), payload...)
		case 3: // MIDDLE
			if pending != nil {
				pending = append(pending, payload...)
			}
		case 4: // LAST
			if pending != nil {
				if err := scanBatch(append(pending, payload...), put); err != nil {
					return err
				}
			}
			pending = nil
		}
	}
	return nil
}

func scanBatch(b []byte, put func([]byte, uint64, []byte)) error {
	if len(b) < 12 {
		return errCorrupt
	}
	seq := binary.LittleEndian.Uint64(b)
	count := binary.LittleEndian.Uint32(b[8:])
	i := 12
	for k := uint32(0); k < count; k++ {
		if i >= len(b) {
			return errCorrupt
		}
		kind := b[i]
		i++
		key, next, err := bytesAt(b, i)
		if err != nil {
			return err
		}
		i = next
		var val []byte
		switch kind {
		case 1:
			if val, i, err = bytesAt(b, i); err != nil {
				return err
			}
			if val == nil {
				val = []byte{}
			}
		case 0:
		default:
			return errCorrupt
		}
		put(key, seq+uint64(k), val)
	}
	return nil
}

// scanTable 读一个 SSTable：footer → index block → 各 data block。
func scanTable(data []byte, put func([]byte, uint64, []byte)) error {
	if len(data) < 48 {
		return errCorrupt
	}
	footer := data[len(data)-48:]
	_, i, err := handle(footer, 0) // metaindex
	if err != nil {
		return err
	}
	off, size, _, err := handleAt(footer, i)
	if err != nil {
		return err
	}
	index, err := readBlock(data, off, size)
	if err != nil {
		return err
	}
	return eachEntry(index, func(_, h []byte) error {
		off, size, _, err := handleAt(h, 0)
		if err != nil {
			return err
		}
		blk, err := readBlock(data, off, size)
		if err != nil {
			return err
		}
		return eachEntry(blk, func(k, v []byte) error {
			if len(k) < 8 {
				return errCorrupt
			}
			tag := binary.LittleEndian.Uint64(k[len(k)-8:])
			var val []byte
			if tag&0xff == 1 {
				val = append([]byte{}, v...)
			}
			put(append([]byte(nil), k[:len(k)-8]...), tag>>8, val)
			return nil
		})
	})
}

func handle(b []byte, i int) (uint64, int, error) {
	_, i, err := uvarint(b, i)
	if err != nil {
		return 0, i, err
	}
	return uvarint(b, i)
}

func handleAt(b []byte, i int) (off, size uint64, next int, err error) {
	if off, i, err = uvarint(b, i); err != nil {
		return
	}
	size, next, err = uvarint(b, i)
	return
}

func readBlock(data []byte, off, size uint64) ([]byte, error) {
	if off > uint64(len(data)) || size+5 > uint64(len(data))-off {
		return nil, errCorrupt
	}
	raw := data[off : off+size]
	if !crcOK(binary.LittleEndian.Uint32(data[off+size+1:]), raw, data[off+size:off+size+1]) {
		return nil, errCorrupt
	}
	switch data[off+size] {
	case 0:
		return raw, nil
	case 1:
		return snappy.Decode(nil, raw)
	default:
		return nil, errors.New("不支持的 LevelDB 块压缩类型")
	}
}

// eachEntry 遍历一个块：前缀压缩的键值对，末尾是重启点数组与其个数。
func eachEntry(blk []byte, fn func(k, v []byte) error) error {
	if len(blk) < 4 {
		return errCorrupt
	}
	restarts := int(binary.LittleEndian.Uint32(blk[len(blk)-4:]))
	end := len(blk) - 4 - 4*restarts
	if restarts < 0 || end < 0 {
		return errCorrupt
	}
	var last []byte
	for i := 0; i < end; {
		shared, j, err := uvarint(blk, i)
		if err != nil {
			return err
		}
		unshared, j, err := uvarint(blk, j)
		if err != nil {
			return err
		}
		vlen, j, err := uvarint(blk, j)
		if err != nil {
			return err
		}
		if shared > uint64(len(last)) || unshared > uint64(end-j) || vlen > uint64(end-j)-unshared {
			return errCorrupt
		}
		key := append(append([]byte(nil), last[:shared]...), blk[j:j+int(unshared)]...)
		j += int(unshared)
		if err := fn(key, blk[j:j+int(vlen)]); err != nil {
			return err
		}
		i = j + int(vlen)
		last = key
	}
	return nil
}
