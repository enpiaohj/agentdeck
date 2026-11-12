import test from 'node:test'
import assert from 'node:assert/strict'
import { fmtBytes, fmtElapsed, fmtPercent } from '../src/format.ts'

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

test('已用时间按秒 / 分 / 小时换单位，起始那一秒不显示', () => {
  assert.equal(fmtElapsed(0), '')
  assert.equal(fmtElapsed(-3), '')
  assert.equal(fmtElapsed(Number.NaN), '')
  assert.equal(fmtElapsed(1), '1 秒')
  assert.equal(fmtElapsed(59.9), '59 秒')
  assert.equal(fmtElapsed(60), '1 分 0 秒')
  assert.equal(fmtElapsed(185), '3 分 5 秒')
  assert.equal(fmtElapsed(3600), '1 小时 0 分')
  assert.equal(fmtElapsed(7860), '2 小时 11 分')
})