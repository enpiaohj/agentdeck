import test from 'node:test'
import assert from 'node:assert/strict'
import { executeLocation, performResume } from '../src/projectLocation.ts'

test('目录有效时继续和复制使用相同的预览校验', async () => {
  for (const action of ['resume', 'copy']) {
    const calls = []
    const result = await performResume('claude:one', action, {
      prepare: async key => { calls.push(['prepare', key]); return { original: 'old', directory: 'target', needsLocation: false } },
      choose: async () => { throw new Error('不应打开对话框') },
      open: async key => { calls.push(['open', key]) },
      copy: async key => { calls.push(['copy', key]) },
    })
    assert.equal(result, true)
    assert.deepEqual(calls, [['prepare', 'claude:one'], [action === 'resume' ? 'open' : 'copy', 'claude:one']])
  }
})

test('关联失效时传递原目录和失效目录，取消后不执行动作', async () => {
  const preview = { original: 'old', directory: 'stale', needsLocation: true }
  const result = await performResume('one', 'resume', {
    prepare: async () => preview,
    choose: async value => { assert.deepEqual(value, preview); return false },
    open: async () => { throw new Error('取消后启动') },
    copy: async () => { throw new Error('取消后复制') },
  })
  assert.equal(result, false)
})

test('没有原目录时允许直接复制按 ID 恢复命令，启动仍需选目录', async () => {
  for (const action of ['copy', 'resume']) {
    const calls = []
    await performResume('one', action, {
      prepare: async () => ({ original: '', directory: '', needsLocation: true, command: 'codex resume one' }),
      choose: async () => { calls.push('choose'); return false },
      open: async () => { calls.push('open') },
      copy: async () => { calls.push('copy') },
    })
    assert.deepEqual(calls, [action === 'copy' ? 'copy' : 'choose'])
  }
})

test('保存成功而启动失败时先记录保存状态；仅本次重试不覆盖已存关联', async () => {
  const calls = []
  const services = {
    save: async (original, directory) => { calls.push(['save', original, directory]) },
    saved: directory => { calls.push(['saved', directory]) },
    open: async directory => { calls.push(['open', directory]); throw new Error('终端启动失败') },
    copy: async () => {},
  }
  await assert.rejects(executeLocation('resume', 'old', 'saved', true, services), /终端启动失败/)
  assert.deepEqual(calls, [['save', 'old', 'saved'], ['saved', 'saved'], ['open', 'saved']])
  calls.length = 0
  services.open = async directory => { calls.push(['open', directory]); return { warning: '终端已打开，名称同步失败' } }
  assert.equal(await executeLocation('resume', 'old', 'once', false, services), '终端已打开，名称同步失败')
  assert.deepEqual(calls, [['open', 'once']])
})

test('保存失败时不启动；缺少原目录时不保存关联', async () => {
  const calls = []
  const services = {
    save: async () => { throw new Error('保存失败') },
    saved: () => { calls.push('saved') },
    open: async () => { calls.push('open'); return { warning: '' } },
    copy: async () => {},
  }
  await assert.rejects(executeLocation('resume', 'old', 'target', true, services), /保存失败/)
  assert.deepEqual(calls, [])
  await executeLocation('resume', '', 'target', true, services)
  assert.deepEqual(calls, ['open'])
})

test('预览失败向上传递，不启动或复制', async () => {
  await assert.rejects(performResume('one', 'copy', {
    prepare: async () => { throw new Error('会话不可恢复') },
    choose: async () => true,
    open: async () => { throw new Error('意外启动') },
    copy: async () => { throw new Error('意外复制') },
  }), /会话不可恢复/)
})
