const fs = require('fs');
const path = require('path');
const https = require('https');
const { execSync } = require('child_process');

const VERSION = require('../package.json').version;
const REPO = 'callmidavid/apidiff';

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
    remoteName: `apidiff-${platform}-${arch}${ext}`,
    localName: `apidiff-bin${ext}`
  };
}

async function download(url, destPath) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(destPath);
    https.get(url, (response) => {
      if (response.statusCode === 302 || response.statusCode === 301) {
        return download(response.headers.location, destPath).then(resolve).catch(reject);
      }
      if (response.statusCode !== 200) {
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
  });
}

async function install() {
  const binary = getBinaryName();
  if (!binary) {
    console.log('[apidiff] Unsupported OS/architecture for prebuilt binary. Falling back to local Go toolchain if available.');
    return tryBuildFromSource();
  }

  const binDir = path.join(__dirname);
  const localBinaryPath = path.join(binDir, binary.localName);

  if (fs.existsSync(localBinaryPath)) {
    console.log('[apidiff] Prebuilt binary already exists at:', localBinaryPath);
    return;
  }

  const downloadUrl = `https://github.com/${REPO}/releases/download/v${VERSION}/${binary.remoteName}`;
  console.log(`[apidiff] Downloading prebuilt binary for ${process.platform}-${process.arch}...`);

  try {
    await download(downloadUrl, localBinaryPath);
    if (process.platform !== 'win32') {
      fs.chmodSync(localBinaryPath, 0o755);
    }
    console.log('[apidiff] Prebuilt binary installed successfully!');
  } catch (err) {
    console.warn(`[apidiff] Could not download prebuilt binary (${err.message}). Attempting fallback to local Go build...`);
    tryBuildFromSource();
  }
}

function tryBuildFromSource() {
  try {
    const ext = process.platform === 'win32' ? '.exe' : '';
    const targetPath = path.join(__dirname, `apidiff-bin${ext}`);
    const mainGoPath = path.join(__dirname, '..', 'cmd', 'apidiff', 'main.go');
    
    execSync(`go build -o "${targetPath}" "${mainGoPath}"`, { stdio: 'inherit' });
    if (process.platform !== 'win32') {
      try { fs.chmodSync(targetPath, 0o755); } catch (e) {}
    }
    console.log('[apidiff] Built binary locally using system Go toolchain.');
  } catch (e) {
    console.log('[apidiff] Go is not installed on this system. APIDiff will download prebuilt binaries on release or run via Go if installed.');
  }
}

install();
