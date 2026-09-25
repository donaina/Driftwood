/**
 * Driftwood - JavaScript & TypeScript Integration Module
 */

const { spawn } = require('child_process');
const path = require('path');
const fs = require('fs');

class Driftwood {
  constructor(options = {}) {
    this.port = options.port || 8787;
    this.target = options.target || 'http://localhost:3000';
    this.process = null;
    this.viaGoRun = false;
  }

  start() {
    return new Promise((resolve, reject) => {
      const ext = process.platform === 'win32' ? '.exe' : '';
      const prebuiltBinPath = path.join(__dirname, 'bin', `drift-bin${ext}`);
      const args = ['--port', String(this.port), '--target', this.target];

      if (fs.existsSync(prebuiltBinPath)) {
        if (process.platform !== 'win32') {
          try { fs.chmodSync(prebuiltBinPath, 0o755); } catch (e) {}
        }
        this.process = spawn(prebuiltBinPath, args, { stdio: 'pipe' });
      } else {
        /* By package path, from the module root — the same two things
           bin/drift.js does for the same line, and this was the third copy that
           did neither. `go run cmd/drift/main.go` compiles that one file, so a
           sibling added to package main is silently left out; and an absolute
           path here made Go resolve the imports against the *caller's* module,
           so start() rejected with "go.mod file not found in current directory
           or any parent directory" and never reached the dashboard at all. */
        this.viaGoRun = true;
        this.process = spawn('go', ['run', './cmd/drift', ...args], {
          stdio: 'pipe',
          cwd: __dirname,
          /* Its own process group, so stop() can signal the server rather than
             only the wrapper holding it. */
          detached: process.platform !== 'win32',
        });
      }

      // The server announces itself through Go's log package, which writes to
      // stderr. Watching stdout alone meant this promise never settled and the
      // README's own example hung forever. Both streams are read: which one a
      // log line lands on is the server's business, not this wrapper's.
      let settled = false;
      const checkReady = (data) => {
        if (settled) return;
        if (data.toString().includes('Web Dashboard & Proxy running')) {
          settled = true;
          resolve(this);
        }
      };

      this.process.stdout.on('data', checkReady);

      this.process.stderr.on('data', (data) => {
        console.error('[Driftwood]', data.toString());
        checkReady(data);
      });

      // 'error' fires only when the process cannot be started at all. A binary
      // that starts and then dies — `go run` failing to compile is the common
      // case — emits 'exit' instead, and nothing was listening for it, so
      // start() went on waiting on a process that had already gone.
      this.process.on('error', (err) => {
        if (settled) return;
        settled = true;
        reject(err);
      });

      this.process.on('exit', (code) => {
        if (settled) return;
        settled = true;
        reject(new Error(
          `drift exited with code ${code} before the dashboard was ready`
        ));
      });
    });
  }

  stop() {
    if (!this.process) return;

    /* `go run` compiles the server and execs it as a child, so killing the
       wrapper leaves the server running, reparented to init and still holding
       the port. That is what it did: after stop(), the dashboard answered on a
       port its caller had been told was released, and the process outlived the
       Node process that started it. Killing the group takes both. */
    if (this.viaGoRun) {
      const { pid } = this.process;
      if (process.platform === 'win32') {
        spawn('taskkill', ['/pid', String(pid), '/T', '/F'], { stdio: 'ignore' });
      } else {
        try {
          process.kill(-pid, 'SIGTERM');
        } catch (err) {
          this.process.kill();
        }
      }
    } else {
      this.process.kill();
    }

    this.process = null;
  }
}

module.exports = Driftwood;
