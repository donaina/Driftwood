import react from '@vitejs/plugin-react'
import { defineConfig, type Connect, type Plugin } from 'vite'
import { resolve } from 'path'

/* import.meta.dirname rather than __dirname: this file is ESM, and __dirname
   is not defined in it. Vite papers over the reference by injecting a shim, so
   it works and warns on every build — which is how the dashboard's config has
   been building. */

/* The marketing site: a landing page and a try-it-out page, built to
   site/dist and embedded into the Go binary by site/site.go.

   It builds into site/ rather than a dist/ of its own because //go:embed cannot
   reach outside its own package directory — the same wall frontend-react/ hits
   with web/. Putting the output here lets one package own the whole site, the
   pages and the bundles they load.

   The binary serves it rather than a static host because of what the try page
   does: it drives the live control API same-origin. Two origins would mean
   widening the control plane's CORS allowlist, which is deliberately a hardcoded
   localhost list, purely to serve static files.

   What the two surfaces share is ./src/styles.css, which imports the dashboard's
   own token file. That is the whole reason they read as one product. */
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
   nothing would show /try reporting "no instance running" on a machine where the
   instance is running, which is the opposite of what a production rehearsal is
   for. */
const controlPlane = {
  '/_driftwood': {
    target: 'http://127.0.0.1:8787',
    changeOrigin: false,
    ws: true,
  },
}

/* "/try" is the URL both pages link to and the one site/site.go serves; the file
   on disk is try.html. Without this, dev and preview would disagree with
   production about it: Vite's default appType is "spa", so an unknown path is
   answered with index.html and clicking "Try it out" would land back on the
   landing page. Both servers get the rewrite, so the three agree.

   This is the one place the site is not purely static, and it exists only
   because the static file and the nice URL differ — the same trade site.go makes
   by serving try.html at both paths, so that links written either way work. */
function tryPage(): Plugin {
  const rewrite: Connect.NextHandleFunction = (req, _res, next) => {
    const [path, query] = (req.url ?? '').split('?')
    if (path === '/try') {
      req.url = '/try.html' + (query ? `?${query}` : '')
    }
    next()
  }
  return {
    name: 'driftwood-try-page',
    configureServer: (server) => void server.middlewares.use(rewrite),
    configurePreviewServer: (server) => void server.middlewares.use(rewrite),
  }
}

export default defineConfig({
  plugins: [react(), tryPage()],

  // Not the default "spa". These are two real documents with no client-side
  // routing, and production answers a path it does not have with a 404 — so a
  // dev server that invents index.html for every unknown path hides exactly the
  // failure this project has shipped before, where every asset answers 200 with
  // the page itself. "mpa" makes the local servers as unforgiving as the binary.
  appType: 'mpa',

  server: { proxy: controlPlane },
  preview: { proxy: controlPlane },

  build: {
    outDir: 'dist',
    // Off, unlike a default Vite project, because site/dist/PLACEHOLDER is
    // committed: //go:embed is a compile error when the directory it names is
    // absent, so a clean clone could not build at all without it. `make site`
    // clears the directory itself and keeps the placeholder — the same
    // arrangement, and the same reason, as web/dist.
    emptyOutDir: false,
    // Two pages, two entries. Vite treats each HTML file as an entry and emits
    // it with its own hashed assets, so each page is a real document rather than
    // a client-side route the server has to be taught to rewrite. site/site.go
    // therefore only has to map two paths onto two files, and never has to fall
    // back to index.html — which is what lets a missing asset be an honest 404.
    rollupOptions: {
      input: {
        main: resolve(import.meta.dirname, 'index.html'),
        try: resolve(import.meta.dirname, 'try.html'),
      },
    },
  },
})
