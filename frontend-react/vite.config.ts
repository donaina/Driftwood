import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { resolve } from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // One CSS file for the whole build. This is load-bearing, not cosmetic:
    // only one entry imports CSS today, but once shared primitives exist,
    // several entries could each pull in a stylesheet. With a stable output
    // name (below) multiple CSS assets would silently overwrite each other.
    cssCodeSplit: false,
    rollupOptions: {
      // One entry per React view that web/index.html actually loads. The
      // agency and exportReporting entries are gone with their components:
      // both were facades that alerted "would be shown here in a full
      // implementation", so there was nothing behind them to build.
      input: {
        scenario: resolve(__dirname, 'src/main-scenario.tsx'),
        history: resolve(__dirname, 'src/main-history.tsx'),
        integrations: resolve(__dirname, 'src/main-integrations.tsx'),
        webhook: resolve(__dirname, 'src/main-webhook.tsx'),
        thresholds: resolve(__dirname, 'src/main-thresholds.tsx')
      },
      output: {
        entryFileNames: '[name].js',
        chunkFileNames: 'assets/chunks/[name].[hash].js',
        // The stylesheet gets a STABLE, unhashed path so web/index.html can
        // link it with a plain <link> (that file is hand-edited, not a Vite
        // input, so it can't reference a build-generated hash).
        assetFileNames: (info) => {
          const names = (info as { names?: string[] }).names ?? []
          return names.some((n) => n?.endsWith('.css'))
            ? 'assets/driftwood.css'
            : 'assets/[name].[hash].[ext]'
        }
      }
    }
  }
})
