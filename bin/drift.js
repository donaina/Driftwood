#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

console.log('⚡ Starting Driftwood proxy...');

const ext = process.platform === 'win32' ? '.exe' : '';
const prebuiltBinPath = path.join(__dirname, `drift-bin${ext}`);
const userArgs = process.argv.slice(2);

let child;

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
  // Fallback to local Go runtime if prebuilt binary isn't present
  const mainGoPath = path.join(__dirname, '..', 'cmd', 'drift', 'main.go');
  const args = ['run', mainGoPath, ...userArgs];

  child = spawn('go', args, {
    stdio: 'inherit',
    cwd: path.join(__dirname, '..')
  });
}

child.on('error', (err) => {
  console.error('Failed to start Driftwood:', err.message);
  console.error('If prebuilt binary download failed, please ensure Go 1.25+ is installed on your system.');
  process.exit(1);
});

child.on('exit', (code) => {
  process.exit(code || 0);
});
