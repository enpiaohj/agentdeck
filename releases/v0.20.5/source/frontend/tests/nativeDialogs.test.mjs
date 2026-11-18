import test from 'node:test'
import assert from 'node:assert/strict'
import { isDialogCancelled, pickOpenPath } from '../src/nativeDialogs.ts'

// Windows 上取消选择时 wails 抛出的原文（cfd 的 ErrorCancelled 被包了一层）。
const CANCELLED = new Error('Invalid dialog call: Dialog.OpenFile failed, error getting selection: cancelled by user')

test('用户取消归一成空串，真失败照旧抛出', async () => {
  assert.equal(isDialogCancelled(CANCELLED), true)
  assert.equal(isDialogCancelled('Dialog.OpenFile failed, error getting selection: canceled by user'), true)
  assert.equal(await pickOpenPath({}, async () => { throw CANCELLED }), '')
  assert.equal(isDialogCancelled(new Error('access denied')), false)
  await assert.rejects(() => pickOpenPath({}, async () => { throw new Error('access denied') }), /access denied/)
})

test('选到路径原样返回，空与多选结果都归成空串', async () => {
  assert.equal(await pickOpenPath({}, async () => 'D:\\Work'), 'D:\\Work')
  assert.equal(await pickOpenPath({}, async () => ''), '')
  assert.equal(await pickOpenPath({}, async () => []), '')
})