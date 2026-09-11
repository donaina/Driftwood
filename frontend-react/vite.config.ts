import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { resolve } from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    rollupOptions: {
      input: {
        main: resolve(__dirname, 'src/main.tsx'),
        thresholds: resolve(__dirname, 'src/main-thresholds.tsx'),
        agency: resolve(__dirname, 'src/main-agency.tsx')
      },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: 'assets/chunks/[name].[hash].js',
        assetFileNames: 'assets/[name].[hash].[ext]'
      }
    }
  }
})
