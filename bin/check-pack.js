#!/usr/bin/env node

/* What npm would actually publish, read back and checked.

   Nothing else in this repository can see this. The Go tests cannot: the file
   list is npm's, and the defect it caught was invisible to a compiler, a linter
   and a Vite build. `make dashboard` and `make site` cannot: they assert that
   the dist directories were written, not that the tarball carries them. And a
   successful `npm pack` cannot: it exited 0 throughout the two months the
   tarball was 2130 files and 26 MB, because `files` named the `site` directory
   and npm's `files` array overrides .gitignore, so 2022 vendored
   `site/node_modules` files went to the registry and nothing said a word.

   The exclusions are a short list of prefixes, not a filter. A stray file in
   any directory this package ships is published whatever its name, so the way
   to keep the tarball honest is to keep reading it back. That is this file.

   How much each exclusion is actually carrying was measured, not assumed, and
   it is not uniform. npm honours a `.gitignore` found *inside* a shipped
   directory even though it overrides the one at the root, so
   frontend-react/node_modules — 3248 files — has been kept out of the tarball
   by the `.gitignore` that Vite's scaffold happened to create, not by anything
   in package.json, and `!frontend-react/dist` is redundant for the same reason.
   Every other shipped directory has no nested `.gitignore`, so `site/`, `ai/`
   and `bin/` are protected by the exclusions alone. That is a thin thread for a
   release to hang on, which is the argument for this file rather than for more
   ignore files: one place that reads the result back beats four that guess.

   `--ignore-scripts` is load-bearing, and it is why this is a script rather
   than a line of shell. `npm pack --json` is unparseable while `prepack` runs,
   because prepack writes its build log to stdout and npm appends the JSON to
   it; and `--dry-run` is inherited by lifecycle scripts as
   npm_config_dry_run, which is the bug bin/prepack.js exists to work around
   (see the comment on `run()` there). With --ignore-scripts, neither applies:
   the file list is computed from `files` exactly as it would be on publish, and
   nothing is built. The dist directories are already built by the Makefile
   prerequisites that run before this. */

const { spawnSync } = require('child_process');
const path = require('path');

const ROOT = path.join(__dirname, '..');

/* The files a consumer needs for the package to do anything at all: the two npm
   entry points, the module export and its types, and the Go source that the
   install step falls back to building when no prebuilt binary arrives. */
const REQUIRED_FILES = [
  'package.json',
  'index.js',
  'index.d.ts',
  'bin/drift.js',
  'bin/install.js',
  'bin/prepack.js',
  'go.mod',
  'cmd/drift/main.go',
  'web/web.go',
  'site/site.go',
];

/* The files each //go:embed reads. They are gitignored apart from PLACEHOLDER,
   so a tarball packed from a clean checkout carries the placeholder and nothing
   else — which compiles, starts, and serves a dashboard and a site with no
   stylesheet and no scripts. */
const REQUIRED_BUILT = [
  'web/dist/assets/driftwood.css',
  'site/dist/index.html',
  'site/dist/try.html',
];

/* Ceilings rather than exact counts, so adding a component or an image does not
   fail the gate. Each sits far below what the regression that motivated this
   file produced: 2130 files, 26.1 MB packed, 87.6 MB unpacked, of which 2022
   were site/node_modules. */
const LIMITS = {
  files: 400,
  packed: 8 * 1024 * 1024,
  unpacked: 16 * 1024 * 1024,
};

/* Anything here reaching the registry is a bug: node_modules for the size,
   the two stale build outputs because their sources ship and the build runs on
   pack, and drift-bin because a checkout that has built a binary would publish
   12 MB of it. */
const FORBIDDEN = [
  { test: (p) => p.includes('node_modules'), why: 'vendored dependencies' },
  { test: (p) => p.startsWith('frontend-react/dist/'), why: "a stale duplicate of web/dist, which is the copy that is served" },
  { test: (p) => p.startsWith('ai/dist/'), why: "tsc output for the sidecar, which has its own build" },
  { test: (p) => /^bin\/drift-bin/.test(p), why: 'a locally built platform binary' },
  { test: (p) => p.startsWith('.git/'), why: 'repository metadata' },
];

const failures = [];

function fail(message) {
  failures.push(message);
}

function mb(bytes) {
  return `${(bytes / 1048576).toFixed(1)} MB`;
}

const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm';
const result = spawnSync(npm, ['pack', '--dry-run', '--json', '--ignore-scripts'], {
  cwd: ROOT,
  encoding: 'utf8',
});

if (result.error) {
  console.error(`[pack-check] could not run npm: ${result.error.message}`);
  process.exit(1);
}
if (result.status !== 0) {
  console.error('[pack-check] npm pack failed:');
  console.error(result.stderr || `exit ${result.status}`);
  process.exit(1);
}

let report;
try {
  report = JSON.parse(result.stdout)[0];
} catch {
  console.error('[pack-check] npm pack --json did not produce parseable JSON.');
  console.error('[pack-check] Its stdout was:');
  console.error(result.stdout);
  process.exit(1);
}

const paths = report.files.map((f) => f.path);
const present = new Set(paths);

for (const file of REQUIRED_FILES) {
  if (!present.has(file)) {
    fail(`the package is missing ${file}, which a consumer needs`);
  }
}
for (const file of REQUIRED_BUILT) {
  if (!present.has(file)) {
    fail(`${file} is not in the tarball, so the embedded assets are the placeholder`);
  }
}
for (const rule of FORBIDDEN) {
  const hits = paths.filter(rule.test);
  if (hits.length > 0) {
    fail(`${hits.length} entr${hits.length === 1 ? 'y' : 'ies'} under ${rule.why}: ${hits.slice(0, 3).join(', ')}${hits.length > 3 ? ', …' : ''}`);
  }
}
if (paths.length > LIMITS.files) {
  fail(`${paths.length} files, over the ceiling of ${LIMITS.files}`);
}
if (report.size > LIMITS.packed) {
  fail(`${mb(report.size)} packed, over the ceiling of ${mb(LIMITS.packed)}`);
}
if (report.unpackedSize > LIMITS.unpacked) {
  fail(`${mb(report.unpackedSize)} unpacked, over the ceiling of ${mb(LIMITS.unpacked)}`);
}

if (failures.length > 0) {
  console.error(`[pack-check] ${failures.length} problem(s) with what npm would publish:`);
  for (const message of failures) {
    console.error(`[pack-check]   - ${message}`);
  }
  process.exit(1);
}

console.log(`[pack-check] OK: ${paths.length} files, ${mb(report.size)} packed, ${mb(report.unpackedSize)} unpacked, shasum ${report.shasum.slice(0, 12)}…`);
