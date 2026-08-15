const fs = require('fs');
const path = require('path');
const https = require('https');
const crypto = require('crypto');
const { execSync } = require('child_process');

const VERSION = require('../package.json').version;
const REPO = 'donaina/Driftwood';

const platformMap = {
  darwin: 'darwin',
  linux: 'linux',
  win32: 'windows'
};

const archMap = {
  x64: 'amd64',
  arm64: 'arm64'
};

function getBinaryName() {
  const platform = platformMap[process.platform];
  const arch = archMap[process.arch];
  if (!platform || !arch) {
    return null;
  }
  const ext = process.platform === 'win32' ? '.exe' : '';
  return {
    remoteName: `drift-${platform}-${arch}${ext}`,
    localName: `drift-bin${ext}`
  };
}

function sha256File(filePath) {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash('sha256');
    const stream = fs.createReadStream(filePath);
    stream.on('error', reject);
    stream.on('data', chunk => hash.update(chunk));
    stream.on('end', () => resolve(hash.digest('hex')));
  });
}

async function download(url, destPath) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(destPath);
    const req = https.get(url, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        file.close();
        return download(response.headers.location, destPath).then(resolve).catch(reject);
      }
      if (response.statusCode !== 200) {
        file.close();
        fs.unlink(destPath, () => {});
        return reject(new Error(`HTTP status ${response.statusCode}`));
      }
      response.pipe(file);
      file.on('finish', () => {
        file.close(() => resolve(true));
      });
    }).on('error', (err) => {
      fs.unlink(destPath, () => {});
      reject(err);
    });
    // Timeout
    req.setTimeout(30000, () => {
      req.destroy();
      fs.unlink(destPath, () => {});
      reject(new Error('Download timeout'));
    });
  });
}

async function downloadChecksums(version) {
  const url = `https://github.com/${REPO}/releases/download/v${version}/SHA256SUMS.txt`;
  return new Promise((resolve, reject) => {
    https.get(url, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        if (res.statusCode !== 200) {
          reject(new Error(`Checksums HTTP ${res.statusCode}`));
          return;
        }
        const sums = {};
        data.trim().split('\n').forEach(line => {
          const parts = line.trim().split(/\s+/);
          if (parts.length >= 2) {
            sums[parts[1]] = parts[0];
          }
        });
        resolve(sums);
      });
    }).on('error', reject);
  });
}

async function install() {
  const binary = getBinaryName();
  if (!binary) {
    console.log('[driftwood] Unsupported OS/architecture for prebuilt binary. Falling back to local Go toolchain if available.');
    return tryBuildFromSource();
  }

  const binDir = path.join(__dirname);
  const localBinaryPath = path.join(binDir, binary.localName);

  if (fs.existsSync(localBinaryPath)) {
    console.log('[driftwood] Prebuilt binary already exists at:', localBinaryPath);
    return;
  }

  const downloadUrl = `https://github.com/${REPO}/releases/download/v${VERSION}/${binary.remoteName}`;
  console.log(`[driftwood] Downloading prebuilt binary for ${process.platform}-${process.arch}...`);

  try {
    await download(downloadUrl, localBinaryPath);

    // Verify SHA256 checksum
    console.log('[driftwood] Verifying SHA256 checksum...');
    const checksums = await downloadChecksums(VERSION);
    const expected = checksums[binary.remoteName];
    if (expected) {
      const actual = await sha256File(localBinaryPath);
      if (actual.toLowerCase() !== expected.toLowerCase()) {
        fs.unlinkSync(localBinaryPath);
        throw new Error(`Checksum mismatch: expected ${expected}, got ${actual}`);
      }
      console.log('[driftwood] Checksum verified!');
    } else {
      console.warn('[driftwood] Checksum not found in release, skipping verification');
    }

    if (process.platform !== 'win32') {
      fs.chmodSync(localBinaryPath, 0o755);
    }
    console.log('[driftwood] Prebuilt binary installed successfully!');
  } catch (err) {
    console.warn(`[driftwood] Could not download prebuilt binary (${err.message}). Attempting fallback to local Go build...`);
    tryBuildFromSource();
  }
}

function tryBuildFromSource() {
  try {
    const ext = process.platform === 'win32' ? '.exe' : '';
    const targetPath = path.join(__dirname, `drift-bin${ext}`);
    const mainGoPath = path.join(__dirname, '..', 'cmd', 'drift', 'main.go');

    execSync(`go build -o "${targetPath}" "${mainGoPath}"`, { stdio: 'inherit' });
    if (process.platform !== 'win32') {
      try { fs.chmodSync(targetPath, 0o755); } catch (e) {}
    }
    console.log('[driftwood] Built binary locally using system Go toolchain.');
  } catch (e) {
    console.log('[driftwood] Go is not installed on this system. Driftwood will download prebuilt binaries on release or run via Go if installed.');
  }
}

install();