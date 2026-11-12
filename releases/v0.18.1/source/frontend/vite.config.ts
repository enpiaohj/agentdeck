import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// base './':wails 资源服务按相对路径提供,绝对 / 会落在 app:// 根之外
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: 'dist',
    sourcemap: false,
    // Wails embed 同步整个 dist；固定依赖分包减小主包，无额外网络或运行时依赖。
    rollupOptions: { output: { manualChunks(id) {
      const path = id.replaceAll('\\', '/')
      if (/\/node_modules\/(react|react-dom|scheduler)\//.test(path)) return 'react'
      if (/\/node_modules\/(?:@fluentui|@griffel|@floating-ui)\//.test(path)) return 'fluent'
      if (/\/node_modules\/(react-markdown|remark-gfm)\//.test(path)) return 'markdown'
    } } },
  },
})
