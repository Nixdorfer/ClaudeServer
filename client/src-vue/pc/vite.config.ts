import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  build: {
    // 生产环境移除 console 和 debugger
    minify: 'terser',
    terserOptions: {
      compress: {
        drop_console: true,
        drop_debugger: true,
        pure_funcs: ['console.log', 'console.info', 'console.debug']
      }
    },
    // 启用 CSS 代码分割
    cssCodeSplit: true,
    // 设置 chunk 大小警告限制
    chunkSizeWarningLimit: 500,
    rollupOptions: {
      output: {
        // 代码分割策略
        manualChunks: {
          'vue-vendor': ['vue'],
          'marked-vendor': ['marked']
        },
        // 压缩资源文件名
        chunkFileNames: 'assets/[hash].js',
        entryFileNames: 'assets/[hash].js',
        assetFileNames: 'assets/[hash].[ext]'
      },
    },
    // 启用源码压缩
    sourcemap: false,
    // 设置目标浏览器
    target: 'esnext'
  },
  // 生产环境优化
  esbuild: {
    // 生产环境下移除 console
    drop: process.env.NODE_ENV === 'production' ? ['console', 'debugger'] : []
  }
})
