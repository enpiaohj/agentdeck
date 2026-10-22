import test from 'node:test'
import assert from 'node:assert/strict'
import { railTicks, currentPrompt, tickLabel } from '../src/promptRail.ts'

const prompts = [{ index: 0, text: '第一问' }, { index: 50, text: '第二问' }, { index: 52, text: '第三问' }, { index: 100, text: '最后一问' }]

test('刻度位置按段序号成比例，首问在顶、末段在底', () => {
  const ticks = railTicks(prompts, 101, 1000)
  assert.deepEqual(ticks.map(t => t.top), [0, 500, 520, 1000])
  assert.deepEqual(ticks.map(t => [t.first, t.last, t.index]), [[0, 0, 0], [1, 1, 50], [2, 2, 52], [3, 3, 100]])
})

test('间距小于最小值的相邻提问并成一根，点击落到其中第一问', () => {
  const ticks = railTicks(prompts, 101, 100)
  assert.deepEqual(ticks.map(t => [t.first, t.last, t.index]), [[0, 0, 0], [1, 2, 50], [3, 3, 100]])
  assert.equal(tickLabel(ticks[1], prompts.length), '第 2–3 问（共 4 问）\n第二问')
  assert.equal(tickLabel(ticks[0], prompts.length), '第 1 / 4 问\n第一问')
})

test('没有提问或高度未知时不出刻度；只有一段时不除零', () => {
  assert.deepEqual(railTicks([], 10, 100), [])
  assert.deepEqual(railTicks(prompts, 101, 0), [])
  assert.deepEqual(railTicks([{ index: 0, text: 'x' }], 1, 100).map(t => t.top), [0])
})

test('当前提问：视口顶部那段之前最近的一问', () => {
  assert.equal(currentPrompt(prompts, 0), 0)
  assert.equal(currentPrompt(prompts, 51), 1)
  assert.equal(currentPrompt(prompts, 52), 2)
  assert.equal(currentPrompt(prompts, 999), 3)
  assert.equal(currentPrompt([{ index: 5, text: 'x' }], 2), -1)
  assert.equal(currentPrompt([], 2), -1)
})
