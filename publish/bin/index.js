#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');

const binaryName = process.platform === 'win32' ? 'awas.exe' : 'awas';
const binaryPath = path.join(__dirname, binaryName);

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: 'inherit'
});

child.on('close', (code) => {
  process.exit(code);
});
