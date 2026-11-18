# -*- coding: utf-8 -*-
"""裁剪 PNG 区域(zlib 解压/重压),用于放大检查。用法: crop.py <in> <out> <l> <t> <w> <h>"""
import struct, sys, zlib


def read_png(p):
    d = open(p, 'rb').read()
    assert d[:8] == b"\x89PNG\r\n\x1a\n"
    pos, w, h, bitd, ct = 8, 0, 0, 0, 0
    idat = b''
    while pos < len(d):
        ln, tag = struct.unpack(">I4s", d[pos:pos + 8])
        data = d[pos + 8:pos + 8 + ln]
        if tag == b'IHDR':
            w, h, bitd, ct = struct.unpack(">IIBB", data[:10])
        elif tag == b'IDAT':
            idat += data
        pos += 12 + ln
    raw = zlib.decompress(idat)
    ch = 4 if ct == 6 else 3
    stride = w * ch
    rows = []
    prev = bytearray(stride)
    o = 0
    for y in range(h):
        f = raw[o]; o += 1
        line = bytearray(raw[o:o + stride]); o += stride
        if f == 1:
            for i in range(ch, stride):
                line[i] = (line[i] + line[i - ch]) & 255
        elif f == 2:
            for i in range(stride):
                line[i] = (line[i] + prev[i]) & 255
        elif f == 3:
            for i in range(stride):
                a = line[i - ch] if i >= ch else 0
                line[i] = (line[i] + ((a + prev[i]) >> 1)) & 255
        elif f == 4:
            for i in range(stride):
                a = line[i - ch] if i >= ch else 0
                b = prev[i]
                c = prev[i - ch] if i >= ch else 0
                pp = a + b - c
                pa, pb, pc = abs(pp - a), abs(pp - b), abs(pp - c)
                pr = a if (pa <= pb and pa <= pc) else (b if pb <= pc else c)
                line[i] = (line[i] + pr) & 255
        rows.append(bytes(line)); prev = line
    return w, h, ch, rows


def write_png(p, w, h, ch, rows):
    raw = b''.join(b"\x00" + r for r in rows)
    def chunk(tag, data):
        c = struct.pack(">I", len(data)) + tag + data
        return c + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
    ct = 6 if ch == 4 else 2
    png = (b"\x89PNG\r\n\x1a\n"
           + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, ct, 0, 0, 0))
           + chunk(b"IDAT", zlib.compress(raw, 6)) + chunk(b"IEND", b""))
    open(p, 'wb').write(png)


def crop(rows, ch, l, t, w, h):
    return [r[l * ch:(l + w) * ch] for r in rows[t:t + h]]


inp, outp, l, t, w, h = sys.argv[1], sys.argv[2], *map(int, sys.argv[3:7])
w0, h0, ch, rows = read_png(inp)
write_png(outp, w, h, ch, crop(rows, ch, l, t, w, h))
print(f"cropped {outp} {w}x{h}")
