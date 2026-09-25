#!/usr/bin/env node

/* The release build: every Vite app, into the dist directory that the matching
   //go:embed reads.

   This is a script rather than two npm-script lines because of how it failed as
   two npm-script lines. `prepack` used to build frontend-react and not site, and
   nothing caught it: the tarball still packed, still exited 0, and shipped
   `site/dist/PLACEHOLDER` — a marketing site that is a four-line text file —
   because site/dist is gitignored apart from that placeholder, so a clean
   checkout has nothing else to ship. The list of apps is the thing that was
   wrong, so the list of apps is what this file holds.

   The dist directories are cleared the way `make dashboard` and `make site`
   clear them, keeping the committed PLACEHOLDER: both Vite configs are set not
   to empty their output directory precisely so that file survives, and a stale
   hashed bundle left behind by an earlier build would otherwise be published
   under a name nothing references.

   `make verify` remains the gate — it runs gofmt, vet and the race tests as well
   — and its two Vite targets are the developer's path. This runs on the publish
   path, which has no make and no Go: it must work wherever `npm pack` does. */

const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');

/* Each app names the files that prove its build ran. A Vite build that produces
   nothing still exits 0 when a config stops emitting an entry, and the failure
   is otherwise only visible in production. */
const APPS = [
  {
    name: 'the dashboard',
    dir: 'frontend-react',
    dist: 'web/dist',
    outputs: ['assets/driftwood.css'],
  },
  {
    name: 'the site',
    dir: 'site',
    dist: 'site/dist',
    outputs: ['index.html', 'try.html'],
  },
];

const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm';

function run(args, cwd) {
  /* --no-dry-run is not decoration. npm exports its config into the environment
     of every lifecycle script, so `npm pack --dry-run` sets npm_config_dry_run
     for this process, and the nested `npm ci` inherited it and installed
     nothing. On a clean checkout that surfaced as `sh: tsc: command not found`
     and exit 127; on a dirty one it silently reused whatever node_modules
     happened to be lying around. A release build is never a rehearsal. */
  return spawnSync(npm, args, { cwd, stdio: 'inherit', env: process.env });
}

function clearDist(dist) {
  const dir = path.join(ROOT, dist);
  if (!fs.existsSync(dir)) return;
  for (const entry of fs.readdirSync(dir)) {
    if (entry === 'PLACEHOLDER') continue;
    fs.rmSync(path.join(dir, entry), { recursive: true, force: true });
  }
}

function buildApp(app) {
  const cwd = path.join(ROOT, app.dir);
  console.log(`[driftwood] Building ${app.name}...`);

  clearDist(app.dist);

  for (const step of [
    { label: 'npm ci', args: ['ci', '--no-dry-run'] },
    { label: 'npm run build', args: ['run', 'build'] },
  ]) {
    const result = run(step.args, cwd);
    if (result.error || result.status !== 0) {
      console.error(`[driftwood] ${step.label} failed for ${app.name} (${app.dir}).`);
      process.exit(result.status || 1);
    }
  }

  const missing = app.outputs.filter((f) => !fs.existsSync(path.join(ROOT, app.dist, f)));
  if (missing.length > 0) {
    console.error(`[driftwood] ${app.name} built, but ${app.dist} is missing: ${missing.join(', ')}`);
    console.error('[driftwood] Packing now would publish a package whose embedded assets are absent.');
    process.exit(1);
  }
}

for (const app of APPS) {
  buildApp(app);
}

/* Done here rather than as a `chmod +x` in the prepack script, where it was:
   that is a Unix command, so `npm pack` failed outright on Windows even though
   every other step of this build works there. The mode is lost by the tarball
   anyway — npm sets the executable bit from the `bin` field on install — but a
   binary built from a checkout should be runnable in that checkout. */
if (process.platform !== 'win32') {
  for (const f of ['drift.js', 'install.js', 'prepack.js']) {
    try {
      fs.chmodSync(path.join(__dirname, f), 0o755);
    } catch (err) {
      console.warn(`[driftwood] could not mark bin/${f} executable: ${err.message}`);
    }
  }
}

console.log('[driftwood] Release assets built for every app.');
