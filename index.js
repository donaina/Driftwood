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

      this.process.stdout.on('data', (data) => {
        const str = data.toString();
        if (str.includes('Web Dashboard & Proxy running')) {
          resolve(this);
        }
      });

      this.process.stderr.on('data', (data) => {
        console.error('[Driftwood]', data.toString());
      });

      this.process.on('error', (err) => {
        reject(err);
      });
    });
  }

  stop() {
    if (this.process) {
      this.process.kill();
      this.process = null;
    }
  }

  /**
   * Express / Connect middleware proxy helper for Node.js
   */
  middleware() {
    return (req, res, next) => {
      req.headers['x-driftwood-sniffed'] = 'true';
      next();
    };
  }
}

module.exports = Driftwood;
