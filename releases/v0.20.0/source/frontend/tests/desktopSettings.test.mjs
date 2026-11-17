import test from 'node:test'
import assert from 'node:assert/strict'
import { createDesktopSettings } from '../src/desktopSettings.ts'

const options = (changes = {}) => ({ start_at_login: false, close_to_tray: false, start_at_login_supported: true, tray_supported: true, ...changes })
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done }); return { promise, resolve } }

test('读取失败不伪造关闭状态，可重试读取后端值', async () => {
  let fail = true
  const settings = createDesktopSettings({ Desktop: async () => { if (fail) throw new Error('读取失败'); return options({ close_to_tray: true }) } })
  await settings.load()
  assert.equal(settings.getSnapshot().view, null)
  assert.match(settings.getSnapshot().error, /读取失败/)
  fail = false
  await settings.load()
  assert.equal(settings.getSnapshot().view.close_to_tray, true)
  assert.equal(settings.getSnapshot().error, '')
})

test('保存使用后端返回值，进行中不重复提交另一开关', async () => {
  const pending = deferred()
  let otherWrites = 0
  const settings = createDesktopSettings({ Desktop: async () => options(), SetStartAtLogin: () => pending.promise, SetCloseToTray: async () => { otherWrites++; return options() } })
  await settings.load()
  const saving = settings.save('start_at_login', true)
  assert.equal(settings.getSnapshot().operation, 'saving')
  assert.equal(settings.getSnapshot().view.start_at_login, false)
  await settings.save('close_to_tray', true)
  assert.equal(otherWrites, 0)
  pending.resolve(options({ start_at_login: true }))
  await saving
  assert.equal(settings.getSnapshot().view.start_at_login, true)
  assert.equal(settings.getSnapshot().operation, null)
})

test('保存失败回读实际状态，并保留错误提示', async () => {
  let reads = 0
  const settings = createDesktopSettings({ Desktop: async () => options({ close_to_tray: ++reads > 1 }), SetCloseToTray: async () => { throw new Error('无写入权限') } })
  await settings.load()
  await settings.save('close_to_tray', true)
  assert.equal(settings.getSnapshot().view.close_to_tray, true)
  assert.match(settings.getSnapshot().error, /无写入权限/)
})

test('保存与回读均失败时清空未知状态，不显示旧值为已生效', async () => {
  let reads = 0
  const settings = createDesktopSettings({ Desktop: async () => { if (++reads > 1) throw new Error('连接中断'); return options() }, SetCloseToTray: async () => { throw new Error('写入失败') } })
  await settings.load()
  await settings.save('close_to_tray', true)
  assert.equal(settings.getSnapshot().view, null)
  assert.match(settings.getSnapshot().error, /写入失败.*连接中断/)
})

test('平台不支持的设置不会提交', async () => {
  let writes = 0
  const settings = createDesktopSettings({ Desktop: async () => options({ start_at_login_supported: false, tray_supported: false }), SetStartAtLogin: async () => { writes++; return options() }, SetCloseToTray: async () => { writes++; return options() } })
  await settings.load()
  await settings.save('start_at_login', true)
  await settings.save('close_to_tray', true)
  assert.equal(writes, 0)
})
