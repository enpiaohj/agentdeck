import test from 'node:test'
import assert from 'node:assert/strict'
import { createUpdateSettings } from '../src/updateSettings.ts'

const view = (changes = {}) => ({
  current: '0.9.0', dev: false, checked: false, latest: '', available: false, notes: '', page_url: '',
  checked_at: '', published_at: '', checking: false, downloading: false, progress: 0,
  downloaded_dir: '', downloadable: false, auto_check: true, error: '', release_page_url: 'https://example.test',
  ...changes,
})
const events = () => {
  let handler = null
  return {
    subscribe: fn => { handler = fn; return () => { handler = null } },
    emit: next => handler?.(next),
    get subscribed() { return handler !== null },
  }
}
const noEvents = () => events()

test('读取状态失败时保留错误，可重试', async () => {
  let fail = true
  const settings = createUpdateSettings({
    Status: async () => { if (fail) throw new Error('后端未就绪'); return view({ checked: true, latest: '0.9.0' }) },
  }, noEvents())
  await settings.load()
  assert.equal(settings.getSnapshot().view, null)
  assert.match(settings.getSnapshot().error, /后端未就绪/)
  fail = false
  await settings.load()
  assert.equal(settings.getSnapshot().view.checked, true)
  assert.equal(settings.getSnapshot().error, '')
})

test('检查失败后回读实际状态，不只显示本地错误', async () => {
  let reads = 0
  const settings = createUpdateSettings({
    Status: async () => view({ checked: ++reads > 1, latest: reads > 1 ? '1.1.0' : '' }),
    Check: async () => { throw new Error('无法连接 GitHub') },
  }, noEvents())
  await settings.load()
  await settings.check()
  assert.match(settings.getSnapshot().error, /无法连接 GitHub/)
  assert.equal(settings.getSnapshot().view.latest, '1.1.0')
})

test('下载失败时清空进行中的标记并保留错误', async () => {
  const settings = createUpdateSettings({
    Status: async () => view({ checked: true, latest: '1.1.0', available: true, downloadable: true }),
    Download: async () => { throw new Error('SHA-256 不一致') },
  }, noEvents())
  await settings.load()
  await settings.download()
  assert.match(settings.getSnapshot().error, /SHA-256/)
  assert.equal(settings.getSnapshot().view.available, true)
  assert.equal(settings.getSnapshot().view.downloading, false)
})

test('自动检查开关使用后端返回值', async () => {
  const settings = createUpdateSettings({
    Status: async () => view(),
    SetAutoCheck: async enabled => view({ auto_check: enabled }),
  }, noEvents())
  await settings.load()
  await settings.setAutoCheck(false)
  assert.equal(settings.getSnapshot().view.auto_check, false)
})

test('后端推送的进度视图直接覆盖本地状态', async () => {
  const bus = events()
  const settings = createUpdateSettings({ Status: async () => view() }, bus)
  await settings.load()
  const stop = settings.watch()
  assert.equal(bus.subscribed, true)
  bus.emit(view({ downloading: true, progress: 0.5, latest: '1.1.0' }))
  assert.equal(settings.getSnapshot().view.progress, 0.5)
  assert.equal(settings.getSnapshot().error, '')
  stop()
  assert.equal(bus.subscribed, false)
})

test('打开下载目录失败写进错误提示', async () => {
  const settings = createUpdateSettings({
    Status: async () => view(),
    OpenDownloadFolder: async () => { throw new Error('资源管理器不可用') },
  }, noEvents())
  await settings.load()
  await settings.openFolder()
  assert.match(settings.getSnapshot().error, /资源管理器不可用/)
})
