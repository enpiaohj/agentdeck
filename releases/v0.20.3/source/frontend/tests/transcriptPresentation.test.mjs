import test from 'node:test'
import assert from 'node:assert/strict'
import { foldedContent, isLongCode, markdownCodeText, transcriptGroups } from '../src/transcriptPresentation.ts'

test('思考、工具调用、工具结果和上下文使用可折叠展示，工具名称保留', () => {
  const part = { role: 'assistant', kind: 'thinking', text: '先核对配置。\n再执行检查。' }
  assert.equal(foldedContent(part).label, '思考过程')
  assert.equal(foldedContent({ ...part, kind: 'text', role: 'thinking' }).label, '思考过程')
  assert.equal(foldedContent({ ...part, kind: 'tool_use', name: 'Bash' }).label, '工具调用 · Bash')
  assert.equal(foldedContent({ ...part, role: 'tool', kind: 'text' }).monospace, true)
  assert.equal(foldedContent({ ...part, kind: 'context' }).label, '上下文记录')
})

test('普通问答、用户粘贴 JSON 和不完整数据保持直接可读', () => {
  for (const text of ['正常回答\n下一步操作', '```js\nconsole.log(1)\n```', '{\n"unfinished": [\n1,\n2,\n3,\n']) {
    assert.equal(foldedContent({ role: 'assistant', kind: 'text', text }), null)
  }
  const text = JSON.stringify({ values: Array.from({ length: 12 }, (_, i) => i) }, null, 2)
  assert.equal(foldedContent({ role: 'user', kind: 'text', text }), null)
  assert.equal(foldedContent({ role: 'assistant', kind: 'text', text }).label, '结构化数据')
})

test('折叠摘要保留首条有效内容与行数，不修改原始输出', () => {
  const text = '\n{\r\n  "status": "failed",\r\n  "detail": "需要重试"\r\n}\n'
  const part = { role: 'tool', kind: 'tool_result', text }
  assert.equal(foldedContent(part).preview, 'status: failed')
  assert.equal(foldedContent(part).lines, 4)
  assert.equal(part.text, text)
})

test('JSON 正文的摘要取有语义的字段，而不是 `{"script":` 这样的外壳', () => {
  const script = JSON.stringify({ script: 'export const meta = {\n  name: "x",\n}\n', name: 'x' })
  assert.equal(foldedContent({ role: 'tool', kind: 'tool_result', text: script }).preview, 'script: export const meta = { name: "x", }')
  const tool = JSON.stringify({ command: 'powershell -NoProfile -Command "Get-Date"', cwd: 'D:\\Work' })
  assert.equal(foldedContent({ role: 'assistant', kind: 'tool_use', text: tool }).preview, 'command: powershell -NoProfile -Command "Get-Date"')
  const file = JSON.stringify({ file_path: 'D:/Temp/ux-main.png', size: 312583 })
  assert.equal(foldedContent({ role: 'tool', kind: 'tool_result', text: file }).preview, 'file_path: D:/Temp/ux-main.png')
})

test('没有可读标量字段时退回顶层键名，纯文本仍取首个有效行', () => {
  assert.equal(foldedContent({ role: 'tool', kind: 'tool_result', text: '{"alpha": [1, 2], "beta": {"x": 1}, "gamma": 3}' }).preview, '字段：alpha、beta、gamma')
  assert.equal(foldedContent({ role: 'tool', kind: 'tool_result', text: 'plain output line\nsecond' }).preview, 'plain output line')
  // 不完整 JSON 进不了 previewKeys，退回原文首行，不抛异常。
  assert.equal(foldedContent({ role: 'tool', kind: 'tool_result', text: '{\n  "unfinished": [\n1,\n2,\n' }).preview, '"unfinished": [')
})

test('连续过程分组保留窗口原索引，不跨普通问答配对', () => {
  const parts = ['thinking', 'tool_use', 'text', 'tool_result'].map(kind => ({ role: 'assistant', kind, text: kind }))
  const groups = transcriptGroups(parts, 40)
  assert.deepEqual(groups.map(g => [g.process, g.start, g.parts.length]), [[true, 40, 2], [false, 42, 1], [true, 43, 1]])
  assert.equal(groups[0].parts[1], parts[1])
})

test('仅长代码块默认收起，短代码保持展开', () => {
  assert.equal(isLongCode('const ok = true;'), false)
  assert.equal(isLongCode(Array(29).fill('code').join('\n')), false)
  assert.equal(isLongCode(Array(30).fill('code').join('\n')), true)
})

test('代码节点只去掉渲染器附加的换行，保留代码原有首尾空行', () => {
  assert.equal(markdownCodeText('\nconst ready = true;\n\n'), '\nconst ready = true;\n')
  assert.equal(markdownCodeText('const ready = true;\n'), 'const ready = true;')
  assert.equal(markdownCodeText('const ready = true;'), 'const ready = true;')
})
