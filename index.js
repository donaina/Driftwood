/**
 * APIDiff - JavaScript & TypeScript Integration Module
 */

const { spawn } = require('child_process');
const http = require('http');
const path = require('path');

class APIDiff {
  constructor(options = {}) {
    this.port = options.port || 8787;
    this.target = options.target || 'http://localhost:3000';
    this.process = null;
  }

  start() {
    return new Promise((resolve, reject) => {
      const mainGoPath = path.join(__dirname, 'cmd', 'apidiff', 'main.go');
      this.process = spawn('go', ['run', mainGoPath, '--port', String(this.port), '--target', this.target], {
        stdio: 'pipe'
      });

      this.process.stdout.on('data', (data) => {
        const str = data.toString();
        if (str.includes('Web Dashboard & Proxy running')) {
          resolve(this);
        }
      });

      this.process.stderr.on('data', (data) => {
        console.error('[APIDiff]', data.toString());
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
      // Forward headers or flag requests
      req.headers['x-apidiff-sniffed'] = 'true';
      next();
    };
  }
}

module.exports = APIDiff;
