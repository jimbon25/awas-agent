const fs = require('fs');
const path = require('path');
const https = require('https');
const { execSync } = require('child_process');

const OWNER = 'jimbon25';
const REPO = 'awas-agent';
const PKG_VERSION = require('../package.json').version;

const PLATFORM_MAP = {
  darwin: 'darwin',
  linux: 'linux',
  win32: 'windows'
};

const ARCH_MAP = {
  x64: 'amd64',
  arm64: 'arm64'
};

const os = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];

if (!os || !arch) {
  console.error(`error: Unsupported platform: ${process.platform} (${process.arch})`);
  process.exit(1);
}

const fileExt = os === 'windows' ? 'zip' : 'tar.gz';
const fileName = `awas_${PKG_VERSION}_${os}_${arch}.${fileExt}`;
const downloadUrl = `https://github.com/${OWNER}/${REPO}/releases/download/v${PKG_VERSION}/${fileName}`;

const binDir = __dirname;
const tempFilePath = path.join(binDir, fileName);

console.log(`info: Downloading AWAS binary from ${downloadUrl}...`);

function downloadFile(url, dest, callback) {
  https.get(url, (res) => {
    if (res.statusCode === 302 || res.statusCode === 301) {
      downloadFile(res.headers.location, dest, callback);
      return;
    }
    if (res.statusCode !== 200) {
      console.error(`error: Download failed. Status code: ${res.statusCode}`);
      process.exit(1);
    }
    const file = fs.createWriteStream(dest);
    res.pipe(file);
    file.on('finish', () => {
      file.close(callback);
    });
  }).on('error', (err) => {
    fs.unlink(dest, () => {});
    console.error(`error: Download error: ${err.message}`);
    process.exit(1);
  });
}

downloadFile(downloadUrl, tempFilePath, () => {
  console.log('info: Extracting binary...');
  try {
    if (os === 'windows') {
      execSync(`powershell -Command "Expand-Archive -Path '${tempFilePath}' -DestinationPath '${binDir}' -Force"`);
    } else {
      execSync(`tar -xzf "${tempFilePath}" -C "${binDir}"`);
      fs.chmodSync(path.join(binDir, 'awas'), 0o755);
      if (fs.existsSync(path.join(binDir, 'index.js'))) {
        fs.chmodSync(path.join(binDir, 'index.js'), 0o755);
      }
    }
    fs.unlinkSync(tempFilePath);
    console.log('success: AWAS CLI binary installed successfully!');
  } catch (err) {
    console.error(`error: Extraction failed: ${err.message}`);
    process.exit(1);
  }
});
