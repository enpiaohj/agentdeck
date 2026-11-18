// 轻量语法高亮:面向会话正文里的围栏代码与工具输出,不做完整 parser,
// 只切分注释 / 字符串 / 数字 / 关键字四类 token,够用且零依赖。
import type { ReactNode } from 'react'

const KEYWORDS = new Set([
  // JS/TS
  'const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while', 'import', 'from',
  'export', 'default', 'class', 'extends', 'new', 'async', 'await', 'try', 'catch', 'finally',
  'throw', 'typeof', 'instanceof', 'this', 'super', 'null', 'undefined', 'true', 'false',
  'type', 'interface', 'enum', 'implements', 'readonly', 'public', 'private', 'static',
  // Go
  'func', 'package', 'defer', 'go', 'range', 'select', 'case', 'switch', 'break', 'continue',
  'struct', 'map', 'chan', 'nil', 'make',
  // Python / Shell
  'def', 'elif', 'lambda', 'None', 'True', 'False', 'self', 'with', 'as', 'in', 'not', 'and', 'or',
  'echo', 'then', 'fi', 'do', 'done', 'esac', 'local', 'export', 'cd', 'ls', 'cat', 'grep', 'if',
])

type Kind = 'comment' | 'string' | 'number' | 'keyword' | 'plain'

// 一次扫描:注释 | 字符串(含模板串)| 数字 | 标识符
const TOKEN_RE =
  /(\/\/[^\n]*|#[^\n]*|\/\*[\s\S]*?\*\/)|("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`(?:[^`\\]|\\.)*`)|(\b\d+(?:\.\d+)?\b)|([A-Za-z_$][\w$]*)/g

export function highlight(code: string, theme: 'light' | 'dark'): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  let m: RegExpExecArray | null
  TOKEN_RE.lastIndex = 0
  let key = 0
  while ((m = TOKEN_RE.exec(code)) !== null) {
    if (m.index > last) out.push(code.slice(last, m.index))
    let kind: Kind = 'plain'
    if (m[1]) kind = 'comment'
    else if (m[2]) kind = 'string'
    else if (m[3]) kind = 'number'
    else if (m[4] && KEYWORDS.has(m[4])) kind = 'keyword'
    out.push(
      kind === 'plain'
        ? m[0]
        : <span key={key++} style={{ color: TOKEN_COLOR[theme][kind] }}>{m[0]}</span>,
    )
    last = m.index + m[0].length
  }
  if (last < code.length) out.push(code.slice(last))
  return out
}

// 用系统色而非 Fluent token:代码配色需要跨主题稳定,且不随品牌色变化。
const TOKEN_COLOR: Record<'light' | 'dark', Record<Exclude<Kind, 'plain'>, string>> = {
  light: {
    comment: '#8A8A8A',
    string: '#0F7B3F',
    number: '#B35A00',
    keyword: '#1057C4',
  },
  dark: {
    comment: '#8C8C8C',
    string: '#57C08A',
    number: '#E0A162',
    keyword: '#6FB3F2',
  },
}
