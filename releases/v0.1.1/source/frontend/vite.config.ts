import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// base './':wails 资源服务按相对路径提供,绝对 / 会落在 app:// 根之外
export default defineConfig({
  plugins: [react()],
  base: './',
  build: { outDir: 'dist', sourcemap: false },
})
