import test from 'node:test'
import assert from 'node:assert/strict'
import { foldedContent, isLongCode, markdownCodeText } from '../src/transcriptPresentation.ts'

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
  assert.equal(foldedContent(part).preview, '"status": "failed",')
  assert.equal(foldedContent(part).lines, 4)
  assert.equal(part.text, text)
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
