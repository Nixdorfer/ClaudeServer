import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'
import {readFileSync} from 'fs'
import {resolve} from 'path'

const infoJson = JSON.parse(readFileSync(resolve(__dirname, '../../../info.json'), 'utf-8'))
const appVersion = infoJson[0].version

export default defineConfig({
  plugins: [vue()],
  define: {
    __APP_VERSION__: JSON.stringify(appVersion),
    __VUE_OPTIONS_API__: true,
    __VUE_PROD_DEVTOOLS__: false,
    __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: false
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
