import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'
import {readFileSync} from 'fs'
import {resolve} from 'path'

const tauriConfig = JSON.parse(readFileSync(resolve(__dirname, '../../src-tauri/tauri.conf.json'), 'utf-8'))

export default defineConfig({
  plugins: [vue()],
  define: {
    __APP_VERSION__: JSON.stringify(tauriConfig.version)
  },
  server: {
    host: '0.0.0.0',
    port: 5174
  },
  build: {
    minify: 'terser',
    terserOptions: {
      compress: {
        drop_console: false,
        drop_debugger: true
      }
    },
    cssCodeSplit: true,
    chunkSizeWarningLimit: 500,
    rollupOptions: {
      output: {
        manualChunks: {
          'vue-vendor': ['vue'],
          'marked-vendor': ['marked']
        },
        chunkFileNames: 'assets/[hash].js',
        entryFileNames: 'assets/[hash].js',
        assetFileNames: 'assets/[hash].[ext]'
      },
    },
    sourcemap: false,
    target: 'esnext'
  },
  esbuild: {
    drop: process.env.NODE_ENV === 'production' ? ['debugger'] : []
  }
})
