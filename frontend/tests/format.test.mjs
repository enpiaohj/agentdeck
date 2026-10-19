import test from 'node:test'
import assert from 'node:assert/strict'
import { fmtBytes, fmtPercent } from '../src/format.ts'

test('字节数按量级换单位，0 与异常值给 0 B', () => {
  assert.equal(fmtBytes(0), '0 B')
  assert.equal(fmtBytes(-1), '0 B')
  assert.equal(fmtBytes(512), '512 B')
  assert.equal(fmtBytes(1024), '1.0 KB')
  assert.equal(fmtBytes(1536), '1.5 KB')
  assert.equal(fmtBytes(20 * 1024), '20 KB')
  assert.equal(fmtBytes(2.5 * 1024 ** 3), '2.5 GB')
})

test('进度百分比只给有效值的整数百分比，总量未知时留空', () => {
  assert.equal(fmtPercent(0, 0), '')
  assert.equal(fmtPercent(0, 100), '0%')
  assert.equal(fmtPercent(50, 100), '50%')
  assert.equal(fmtPercent(120, 100), '100%')
})