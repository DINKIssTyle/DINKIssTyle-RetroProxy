import './style.css';
import { StartProxy, StopProxy, GetStatus, GetLogs, ClearLogs, GetEncodings, SetEncoding, GetCurrentEncoding, GetImageFormats, SetImageFormat, GetCurrentImageFormat, SetDebugMode, IsDebugMode, GetHTMLVersions, SetHTMLVersion, GetCurrentHTMLVersion } from '../wailsjs/go/main/App';
import { BrowserOpenURL } from '../wailsjs/runtime/runtime';

// Initialize the app
document.querySelector('#app').innerHTML = `
    <div class="container">
        <header class="header">
            <h1 class="title">🌐 DKST RetroProxy</h1>
            <p class="subtitle">Modern websites for legacy browsers</p>
        </header>
        
        <div class="status-card">
            <div class="status-indicator">
                <span class="status-dot" id="statusDot"></span>
                <span class="status-text" id="statusText">Stopped</span>
            </div>
        </div>

        <div class="settings-panel">
            <div class="settings-row">
                <label>Port</label>
                <input type="number" id="portNumber" value="8080" min="1" max="65535" />
            </div>
            <div class="settings-row">
                <label>Encoding</label>
                <select id="encodingSelect"></select>
            </div>
            <div class="settings-row">
                <label>HTML</label>
                <select id="htmlVersionSelect"></select>
            </div>
            <div class="settings-row">
                <label>Image</label>
                <select id="imageSelect"></select>
            </div>
            <div class="settings-row debug-row">
                <label>Debug</label>
                <div style="display:flex;align-items:center;gap:10px;flex:1;min-width:0;">
                    <button class="btn-debug" id="debugBtn" style="flex-shrink:0;">Off</button>
                    <div id="debugUrlContainer" style="display:none;align-items:center;gap:5px;flex:1;min-width:0;background:rgba(0,0,0,0.2);padding:2px 8px;border-radius:4px;">
                        <a id="debugLink" href="#" style="color:#60a5fa;text-decoration:none;font-family:monospace;font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;"></a>
                        <button id="copyDebugBtn" style="background:none;border:none;cursor:pointer;font-size:14px;padding:0;color:#888;" title="Copy">📋</button>
                    </div>
                </div>
            </div>
        </div>

        <div class="button-row">
            <button class="btn btn-start" id="startBtn">▶ Start</button>
            <button class="btn btn-stop" id="stopBtn" disabled>■ Stop</button>
        </div>



        <div class="log-panel">
            <div class="log-header">
                <span>📜 Log</span>
                <button class="btn-clear" id="clearBtn">Clear</button>
            </div>
            <div class="log-content" id="logContent">
                <div class="log-empty">No activity</div>
            </div>
            <div style="text-align:center;margin-top:5px;font-size:11px;color:#666;padding-bottom:5px;">(C) 2025 DINKI'ssTyle</div>
        </div>
    </div>
`;

// Get elements
const portInput = document.getElementById('portNumber');
const startBtn = document.getElementById('startBtn');
const stopBtn = document.getElementById('stopBtn');
const clearBtn = document.getElementById('clearBtn');
const debugBtn = document.getElementById('debugBtn');
const statusDot = document.getElementById('statusDot');
const statusText = document.getElementById('statusText');
const logContent = document.getElementById('logContent');
const encodingSelect = document.getElementById('encodingSelect');

const imageSelect = document.getElementById('imageSelect');
const htmlVersionSelect = document.getElementById('htmlVersionSelect');
const debugLink = document.getElementById('debugLink');
const copyDebugBtn = document.getElementById('copyDebugBtn');
const debugUrlContainer = document.getElementById('debugUrlContainer');

// Event listeners
startBtn.addEventListener('click', startProxy);
stopBtn.addEventListener('click', stopProxy);
clearBtn.addEventListener('click', clearLogs);
debugBtn.addEventListener('click', toggleDebug);
encodingSelect.addEventListener('change', () => changeEncoding());
imageSelect.addEventListener('change', () => changeImageFormat());
htmlVersionSelect.addEventListener('change', () => changeHTMLVersion());

debugLink.addEventListener('click', (e) => {
    e.preventDefault();
    BrowserOpenURL(debugLink.href);
});

copyDebugBtn.addEventListener('click', () => {
    const url = debugLink.href;
    navigator.clipboard.writeText(url).then(() => {
        const original = copyDebugBtn.textContent;
        copyDebugBtn.textContent = '✅';
        setTimeout(() => copyDebugBtn.textContent = original, 1000);
    });
});

// Initialize
init();

async function init() {
    await loadEncodings();
    await loadImageFormats();
    await loadHTMLVersions();
    await updateDebugStatus();
    await updateStatus();
    updateLogs();
}

async function loadEncodings() {
    try {
        const encodings = await GetEncodings();
        const current = await GetCurrentEncoding();
        encodingSelect.innerHTML = encodings.map(e =>
            `<option value="${e.value}" ${e.value === current ? 'selected' : ''}>${e.label}</option>`
        ).join('');
    } catch (err) {
        console.error('Failed to load encodings:', err);
    }
}

async function loadImageFormats() {
    try {
        const formats = await GetImageFormats();
        const current = await GetCurrentImageFormat();
        imageSelect.innerHTML = formats.map(f =>
            `<option value="${f.value}" ${f.value === current ? 'selected' : ''}>${f.label}</option>`
        ).join('');
    } catch (err) {
        console.error('Failed to load image formats:', err);
    }
}

async function loadHTMLVersions() {
    try {
        const versions = await GetHTMLVersions();
        const current = await GetCurrentHTMLVersion();
        htmlVersionSelect.innerHTML = versions.map(v =>
            `<option value="${v.value}" ${v.value === current ? 'selected' : ''}>${v.label}</option>`
        ).join('');
    } catch (err) {
        console.error('Failed to load HTML versions:', err);
    }
}

async function updateDebugStatus() {
    try {
        const enabled = await IsDebugMode();
        if (enabled) {
            debugBtn.textContent = 'On';
            debugBtn.classList.add('active');
        } else {
            debugBtn.textContent = 'Off';
            debugBtn.classList.remove('active');
        }
    } catch (err) {
        console.error('Failed to get debug status:', err);
    }
}

async function toggleDebug() {
    try {
        const current = await IsDebugMode();
        await SetDebugMode(!current);
        await updateDebugStatus();
        addLog(current ? '🔧 Debug mode disabled' : '🔧 Debug mode enabled');
    } catch (err) {
        addLog('❌ ' + err);
    }
}

async function changeEncoding() {
    try {
        await SetEncoding(encodingSelect.value);
        addLog('⚙️ Encoding: ' + encodingSelect.value);
    } catch (err) {
        addLog('❌ ' + err);
    }
}

async function changeImageFormat() {
    try {
        await SetImageFormat(imageSelect.value);
        addLog('🖼️ Image: ' + imageSelect.value);
    } catch (err) {
        addLog('❌ ' + err);
    }
}

async function changeHTMLVersion() {
    try {
        await SetHTMLVersion(htmlVersionSelect.value);
        addLog('📝 HTML Ver: ' + htmlVersionSelect.value);
    } catch (err) {
        addLog('❌ ' + err);
    }
}

async function startProxy() {
    const port = parseInt(portInput.value);
    if (isNaN(port) || port < 1 || port > 65535) {
        addLog('❌ Invalid port');
        return;
    }
    startBtn.disabled = true;
    addLog('⏳ Starting...');
    try {
        const result = await StartProxy(port);
        addLog('✅ ' + result);
        await updateStatus();
    } catch (err) {
        addLog('❌ ' + err);
        startBtn.disabled = false;
    }
}

async function stopProxy() {
    stopBtn.disabled = true;
    addLog('⏳ Stopping...');
    try {
        const result = await StopProxy();
        addLog('✅ ' + result);
        await updateStatus();
    } catch (err) {
        addLog('❌ ' + err);
        stopBtn.disabled = false;
    }
}

async function clearLogs() {
    try {
        await ClearLogs();
        logContent.innerHTML = '<div class="log-empty">No activity</div>';
    } catch (err) {
        console.error(err);
    }
}

function addLog(msg) {
    const time = new Date().toLocaleTimeString('en-US', { hour12: false });
    const entry = document.createElement('div');
    entry.className = 'log-entry';
    entry.textContent = `${time} ${msg}`;

    const empty = logContent.querySelector('.log-empty');
    if (empty) empty.remove();

    logContent.appendChild(entry);
    logContent.scrollTop = logContent.scrollHeight;
}

async function updateLogs() {
    try {
        const logs = await GetLogs();
        if (logs && logs.length > 0) {
            const empty = logContent.querySelector('.log-empty');
            if (empty) empty.remove();

            const current = logContent.querySelectorAll('.log-entry').length;
            for (let i = current; i < logs.length; i++) {
                const time = new Date().toLocaleTimeString('en-US', { hour12: false });
                const entry = document.createElement('div');
                entry.className = 'log-entry';
                entry.textContent = `${time} ${logs[i]}`;
                logContent.appendChild(entry);
            }
            logContent.scrollTop = logContent.scrollHeight;
        }
    } catch (err) {
        console.error(err);
    }
}

function checkDebugVisibility(isVisible) {
    if (isVisible) {
        debugUrlContainer.style.display = 'flex';
    } else {
        debugUrlContainer.style.display = 'none';
    }
}

async function updateStatus() {
    try {
        const status = await GetStatus();
        const isDebug = await IsDebugMode();

        if (status.running) {
            statusDot.className = 'status-dot running';
            statusText.textContent = `Running on port ${status.port}`;
            startBtn.disabled = true;
            stopBtn.disabled = false;
            portInput.disabled = true;

            const url = `http://localhost:${status.port}/debug`;
            debugLink.href = url;
            debugLink.textContent = url;

            if (isDebug) {
                debugUrlContainer.style.display = 'flex';
            } else {
                debugUrlContainer.style.display = 'none';
            }
        } else {
            statusDot.className = 'status-dot stopped';
            statusText.textContent = 'Stopped';
            startBtn.disabled = false;
            stopBtn.disabled = true;
            portInput.disabled = false;
            debugUrlContainer.style.display = 'none';
        }
    } catch (err) {
        console.error(err);
    }
}

setInterval(() => {
    updateStatus();
    updateLogs();
    // updateDebugStatus(); // Debug status also needs polling? Or just UI sync?
    // updateDebugStatus is separate but status.running handles main UI.
    // Let's keep updateDebugStatus polled if external changes happen? 
    // Actually toggleDebug calls it. 
    // Let's rely on updateStatus for visibility.
    updateDebugStatus();
}, 2000);
