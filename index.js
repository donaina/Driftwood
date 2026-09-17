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
        const mainGoPath = path.join(__dirname, 'cmd', 'drift', 'main.go');
        this.process = spawn('go', ['run', mainGoPath, ...args], { stdio: 'pipe' });
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
    if (this.process) {
      this.process.kill();
      this.process = null;
    }
  }
}

module.exports = Driftwood;
