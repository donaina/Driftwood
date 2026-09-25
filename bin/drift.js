#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

console.log('⚡ Starting Driftwood proxy...');

const ext = process.platform === 'win32' ? '.exe' : '';
const prebuiltBinPath = path.join(__dirname, `drift-bin${ext}`);
const userArgs = process.argv.slice(2);

let child;
let usedFallback = false;

if (fs.existsSync(prebuiltBinPath)) {
  if (process.platform !== 'win32') {
    try { fs.chmodSync(prebuiltBinPath, 0o755); } catch (e) {}
  }
  // Use prebuilt standalone binary (No Go installation needed)
  child = spawn(prebuiltBinPath, userArgs, {
    stdio: 'inherit',
    cwd: path.join(__dirname, '..')
  });
} else {
  /* Fallback to the local Go toolchain. The package is run by path
     (`./cmd/drift`), not by file: `go run cmd/drift/main.go` compiles that one
     file, so any sibling that package main gains is silently left out of the
     binary. install.js builds by path for the same reason, and this is the
     second copy of that decision. */
  usedFallback = true;
  child = spawn('go', ['run', './cmd/drift', ...userArgs], {
    stdio: 'inherit',
    cwd: path.join(__dirname, '..')
  });
}

/* Two different situations arrive here as the same ENOENT, and the old message
   named a remedy for one of them: "If prebuilt binary download failed, please
   ensure Go 1.25+ is installed". Someone whose prebuilt binary was simply
   missing was told to install a toolchain they did not need, and someone with
   no Go was told to check a download that had nothing to do with it. The
   fallback flag is what tells them apart. */
child.on('error', (err) => {
  if (usedFallback && err.code === 'ENOENT') {
    console.error('[driftwood] Cannot start: no prebuilt binary is installed and no Go toolchain was found.');
    console.error('[driftwood] The postinstall step downloads the prebuilt binary. To finish it by hand, either');
    console.error('[driftwood] reinstall with Go 1.25+ on PATH (https://go.dev/dl/), or download a release binary from:');
    console.error('[driftwood]   https://github.com/donaina/Driftwood/releases');
  } else {
    console.error('[driftwood] Cannot start: the installed binary failed to run.');
    console.error(`[driftwood] ${err.message}`);
    console.error('[driftwood] Reinstalling the package replaces it.');
  }
  process.exit(1);
});

child.on('exit', (code) => {
  process.exit(code || 0);
});
