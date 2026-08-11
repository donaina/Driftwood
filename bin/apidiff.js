#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

console.log('⚡ Starting APIDiff proxy for JavaScript & TypeScript environment...');

const mainGoPath = path.join(__dirname, '..', 'cmd', 'apidiff', 'main.go');
const args = ['run', mainGoPath, ...process.argv.slice(2)];

const child = spawn('go', args, {
  stdio: 'inherit',
  cwd: path.join(__dirname, '..')
});

child.on('error', (err) => {
  console.error('Failed to start APIDiff:', err.message);
  console.error('Make sure Go 1.20+ is installed on your system.');
  process.exit(1);
});

child.on('exit', (code) => {
  process.exit(code || 0);
});
