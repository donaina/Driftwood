import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { resolve } from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    // Built into web/, not frontend-react/dist, so that the Go package that
    // serves these files can also embed them: //go:embed cannot reach outside
    // its own package directory, and frontend-react/ is a sibling of web/.
    // Keeping the output next to web.go means one package owns the dashboard —
    // the HTML and the bundles it loads — and the binary carries both.
    outDir: resolve(__dirname, '..', 'web', 'dist'),
    // Not emptied, deliberately. web/dist/PLACEHOLDER is committed, because
    // //go:embed is a compile error when the directory is missing and a fresh
    // clone has to be able to build before it can run any Node. Emptying the
    // directory deletes that file, which would leave git reporting it as
    // removed after every build and re-break the clone as soon as that was
    // committed. `make build` clears the directory itself and keeps the
    // placeholder; the cost of a bare `npm run build` is only orphaned
    // content-hashed chunks, which nothing references.
    emptyOutDir: false,
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
