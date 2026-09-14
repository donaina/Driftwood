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
        scenario: resolve(__dirname, 'src/main-scenario.tsx'),
        history: resolve(__dirname, 'src/main-history.tsx'),
        integrations: resolve(__dirname, 'src/main-integrations.tsx'),
        exportReporting: resolve(__dirname, 'src/main-export-reporting.tsx'),
        webhook: resolve(__dirname, 'src/main-webhook.tsx'),
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
