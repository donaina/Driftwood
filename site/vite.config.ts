import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import { resolve } from 'path'

/* import.meta.dirname rather than __dirname: this file is ESM, and __dirname
   is not defined in it. Vite papers over the reference by injecting a shim, so
   it works and warns on every build — which is how the dashboard's config has
   been building. */

/* The marketing site: a landing page and a try-it-out page, built to
   site/dist and deployed on its own.

   Separate from the dashboard on purpose. The dashboard is embedded in the Go
   binary and served from the proxy's reserved /_driftwood/ namespace, because
   it has to be reachable wherever the binary runs. This is a static site with
   no server behaviour, and giving it its own build means it can be deployed,
   cached and rolled back without touching the binary — and, more importantly,
   without the proxy's catch-all claim on "/" having to move.

   What the two share is ./src/styles.css, which imports the dashboard's own
   token file. That is the whole reason they read as one product. */
/* The dev server stands in for nginx, and has to stand in for it exactly:
   the try-it-out page calls the control plane same-origin, because the
   control plane refuses cross-origin calls by design. Without this block the
   page would be served from :5173, its /_driftwood/api/* calls would 404
   against Vite's own server, and it would correctly report that no instance
   was reachable — a confusing way to learn that the proxy is missing rather
   than the binary.

   ws: true because /_driftwood/events is a long-lived SSE stream, and the
   default proxy gives up on a connection that stays open.

   Shared with `preview` deliberately. Preview serves the built output, which
   is the closest thing to production this repo can run locally, and the one
   thing worth checking there is the same-origin path — a preview that proxied
   nothing would show /try.html reporting "no instance running" on a machine
   where the instance is running, which is the opposite of what a production
   rehearsal is for. */
const controlPlane = {
  '/_driftwood': {
    target: 'http://127.0.0.1:8787',
    changeOrigin: false,
    ws: true,
  },
}

export default defineConfig({
  plugins: [react()],

  server: { proxy: controlPlane },
  preview: { proxy: controlPlane },

  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Two pages, two entries. Vite treats each HTML file as an entry and emits
    // it with its own hashed assets, so /try.html is a real page rather than a
    // client-side route the server has to be taught to rewrite.
    rollupOptions: {
      input: {
        main: resolve(import.meta.dirname, 'index.html'),
        try: resolve(import.meta.dirname, 'try.html'),
      },
    },
  },
})
