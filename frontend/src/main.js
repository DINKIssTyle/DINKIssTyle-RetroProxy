import './base.css';
import systemCss from '@sakun/system.css/dist/system.css?inline';
import classicLayoutCss from 'classic-stylesheets/layout.css?inline';
import win31ThemeCss from 'classic-stylesheets/themes/win3x/theme.css?inline';
import win31SkinCss from 'classic-stylesheets/themes/win3x/skins/3.1.css?inline';
import macintoshAppCss from './style.css?inline';
import windows31AppCss from './windows31.css?inline';
import { Browser, Window } from '@wailsio/runtime';
import { App } from '../bindings/oldwebproxy';

const {
    StartProxy, StopProxy, GetStatus, GetLogs, ClearLogs,
    GetEncodings, SetEncoding, GetCurrentEncoding,
    GetImageFormats, SetImageFormat, GetCurrentImageFormat,
    SetDebugMode, IsDebugMode,
    GetHTMLVersions, SetHTMLVersion, GetCurrentHTMLVersion,
    GetLaunchAtStartup, SetLaunchAtStartup,
} = App;

const themes = {
    macintosh: {
        label: 'Macintosh',
        css: `${systemCss}\n${macintoshAppCss}`,
    },
    win31: {
        label: 'Windows 3.1',
        css: `${classicLayoutCss}\n${win31ThemeCss}\n${win31SkinCss}\n${windows31AppCss}`,
    },
};

const themeModes = ['random', 'macintosh', 'win31'];
const themeModeLabels = {
    random: 'Random',
    macintosh: 'Macintosh',
    win31: 'Windows',
};
const savedThemeMode = localStorage.getItem('retroproxy-theme-mode');
let currentThemeMode = themeModes.includes(savedThemeMode) ? savedThemeMode : 'random';
let currentTheme = resolveTheme(currentThemeMode);
const activeThemeStyle = document.createElement('style');
activeThemeStyle.id = 'active-theme-styles';
document.head.appendChild(activeThemeStyle);
applyTheme(currentTheme);

document.querySelector('#app').innerHTML = `
    <main class="app-desktop desktop">
        <section class="window app-window active" aria-label="DKST RetroProxy" tabindex="-1">
            <div class="title-bar app-titlebar" id="windowTitlebar">
                <div class="title-bar-buttons win-title-controls">
                    <button class="window-control" id="winCloseWindowBtn" data-close aria-label="Close window"></button>
                </div>
                <button class="close window-control mac-title-control" id="closeWindowBtn" aria-label="Close window"><span>Close</span></button>
                <h1 class="title title-bar-text">DKST RetroProxy</h1>
                <button class="resize window-control mac-title-control" id="zoomWindowBtn" aria-label="Zoom window"><span>Zoom</span></button>
                <div class="title-bar-buttons win-title-controls">
                    <button class="window-control" id="winZoomWindowBtn" data-maximize aria-label="Maximize window"></button>
                </div>
            </div>

            <div class="details-bar app-details-bar">
                <span class="proxy-state">
                    <span class="status-dot stopped" id="statusDot" aria-hidden="true"></span>
                    <span class="status-text" id="statusText">Stopped</span>
                </span>
            </div>

            <div class="window-pane window-body app-pane">
                <fieldset class="control-panel">
                    <legend>Proxy Control</legend>
                    <div class="settings-grid">
                        <label class="settings-row" for="portNumber">
                            <span>Port:</span>
                            <input type="number" id="portNumber" value="8080" min="1" max="65535" />
                        </label>
                        <label class="settings-row" for="encodingSelect">
                            <span>Encoding:</span>
                            <span class="dropdown theme-dropdown">
                                <select id="encodingSelect"></select>
                                <span class="dropdown-button" aria-hidden="true"></span>
                            </span>
                        </label>
                        <label class="settings-row" for="htmlVersionSelect">
                            <span>HTML:</span>
                            <span class="dropdown theme-dropdown">
                                <select id="htmlVersionSelect"></select>
                                <span class="dropdown-button" aria-hidden="true"></span>
                            </span>
                        </label>
                        <label class="settings-row" for="imageSelect">
                            <span>Image:</span>
                            <span class="dropdown theme-dropdown">
                                <select id="imageSelect"></select>
                                <span class="dropdown-button" aria-hidden="true"></span>
                            </span>
                        </label>
                    </div>

                    <div class="debug-row">
                        <span>Debug:</span>
                        <button class="btn" id="debugBtn">Off</button>
                        <div class="debug-url" id="debugUrlContainer">
                            <a id="debugLink" href="#"></a>
                            <button class="copy-button" id="copyDebugBtn" title="Copy debug URL" aria-label="Copy debug URL">Copy</button>
                        </div>
                    </div>

                    <div class="button-row">
                        <button class="btn btn-default" id="startBtn">Start</button>
                        <button class="btn" id="stopBtn" disabled>Stop</button>
                    </div>
                </fieldset>

                <section class="log-panel" aria-labelledby="logTitle">
                    <div class="log-header">
                        <h2 id="logTitle">Activity Log</h2>
                        <button class="btn compact-btn" id="clearBtn">Clear</button>
                    </div>
                    <div class="log-content" id="logContent" role="log" aria-live="polite">
                        <div class="log-empty">No activity.</div>
                    </div>
                </section>

                <footer class="footer-bar">
                    <span class="startup-option">
                        <input type="checkbox" id="launchAtStartupInput" />
                        <label for="launchAtStartupInput">Launch at startup</label>
                    </span>
                    <button class="theme-toggle" id="themeToggleBtn" type="button"></button>
                    <span>© 2026 DINKI'ssTyle</span>
                </footer>
            </div>
        </section>
    </main>
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
const closeWindowBtn = document.getElementById('closeWindowBtn');
const zoomWindowBtn = document.getElementById('zoomWindowBtn');
const winCloseWindowBtn = document.getElementById('winCloseWindowBtn');
const winZoomWindowBtn = document.getElementById('winZoomWindowBtn');
const windowTitlebar = document.getElementById('windowTitlebar');
const themeToggleBtn = document.getElementById('themeToggleBtn');
const launchAtStartupInput = document.getElementById('launchAtStartupInput');
const configuredSelects = [encodingSelect, imageSelect, htmlVersionSelect];

applyTheme(currentTheme);

// Event listeners
startBtn.addEventListener('click', startProxy);
stopBtn.addEventListener('click', stopProxy);
clearBtn.addEventListener('click', clearLogs);
debugBtn.addEventListener('click', toggleDebug);
encodingSelect.addEventListener('change', () => {
    updateSelectTitle(encodingSelect);
    collapseSelectedOption(encodingSelect);
    changeEncoding();
});
imageSelect.addEventListener('change', () => {
    updateSelectTitle(imageSelect);
    collapseSelectedOption(imageSelect);
    changeImageFormat();
});
htmlVersionSelect.addEventListener('change', () => {
    updateSelectTitle(htmlVersionSelect);
    collapseSelectedOption(htmlVersionSelect);
    changeHTMLVersion();
});

configuredSelects.forEach((select) => {
    select.addEventListener('pointerdown', () => restoreOptionLabels(select));
    select.addEventListener('blur', () => collapseSelectedOption(select));
});

debugLink.addEventListener('click', (e) => {
    e.preventDefault();
    Browser.OpenURL(debugLink.href);
});

copyDebugBtn.addEventListener('click', () => {
    const url = debugLink.href;
    navigator.clipboard.writeText(url).then(() => {
        const original = copyDebugBtn.textContent;
        copyDebugBtn.textContent = 'Done';
        setTimeout(() => copyDebugBtn.textContent = original, 1000);
    });
});

closeWindowBtn.addEventListener('click', () => Window.Close());
zoomWindowBtn.addEventListener('click', () => Window.ToggleMaximise());
winCloseWindowBtn.addEventListener('click', () => Window.Close());
winZoomWindowBtn.addEventListener('click', () => Window.ToggleMaximise());
themeToggleBtn.addEventListener('click', toggleTheme);
launchAtStartupInput.addEventListener('change', updateLaunchAtStartup);
windowTitlebar.addEventListener('dblclick', (event) => {
    if (!event.target.closest('button')) Window.ToggleMaximise();
});

function applyTheme(themeName) {
    currentTheme = Object.hasOwn(themes, themeName) ? themeName : 'macintosh';
    activeThemeStyle.textContent = themes[currentTheme].css;
    document.documentElement.dataset.theme = currentTheme;
    document.documentElement.style.colorScheme = 'light';

    const toggleButton = document.getElementById('themeToggleBtn');
    if (toggleButton) {
        toggleButton.textContent = `Theme: ${themeModeLabels[currentThemeMode]}`;
        toggleButton.title = currentThemeMode === 'random'
            ? `Random mode selected; this launch uses ${themes[currentTheme].label}`
            : `${themes[currentTheme].label} selected`;
    }
}

function toggleTheme() {
    const currentIndex = themeModes.indexOf(currentThemeMode);
    currentThemeMode = themeModes[(currentIndex + 1) % themeModes.length];
    localStorage.setItem('retroproxy-theme-mode', currentThemeMode);
    localStorage.removeItem('retroproxy-theme');
    applyTheme(resolveTheme(currentThemeMode));
}

function resolveTheme(themeMode) {
    if (themeMode === 'random') {
        return Math.random() < 0.5 ? 'macintosh' : 'win31';
    }
    return themeMode;
}

// Initialize
init();

async function init() {
    await loadEncodings();
    await loadImageFormats();
    await loadHTMLVersions();
    await updateDebugStatus();
    await loadLaunchAtStartup();
    await updateStatus();
    updateLogs();
}

async function loadLaunchAtStartup() {
    try {
        launchAtStartupInput.checked = await GetLaunchAtStartup();
    } catch (err) {
        launchAtStartupInput.checked = false;
        addLog('[ERROR] Failed to read launch at startup: ' + err);
    }
}

async function updateLaunchAtStartup() {
    const enabled = launchAtStartupInput.checked;
    launchAtStartupInput.disabled = true;
    try {
        await SetLaunchAtStartup(enabled);
        addLog(`[CONFIG] Launch at startup ${enabled ? 'enabled' : 'disabled'}`);
    } catch (err) {
        launchAtStartupInput.checked = !enabled;
        addLog('[ERROR] Failed to update launch at startup: ' + err);
    } finally {
        launchAtStartupInput.disabled = false;
    }
}

async function loadEncodings() {
    try {
        const encodings = await GetEncodings();
        const current = await GetCurrentEncoding();
        encodingSelect.innerHTML = encodings.map(e =>
            `<option value="${e.value}" data-full-label="${e.label}" ${e.value === current ? 'selected' : ''}>${e.label}</option>`
        ).join('');
        updateSelectTitle(encodingSelect);
        collapseSelectedOption(encodingSelect);
    } catch (err) {
        console.error('Failed to load encodings:', err);
    }
}

async function loadImageFormats() {
    try {
        const formats = await GetImageFormats();
        const current = await GetCurrentImageFormat();
        imageSelect.innerHTML = formats.map(f =>
            `<option value="${f.value}" data-full-label="${f.label}" ${f.value === current ? 'selected' : ''}>${f.label}</option>`
        ).join('');
        updateSelectTitle(imageSelect);
        collapseSelectedOption(imageSelect);
    } catch (err) {
        console.error('Failed to load image formats:', err);
    }
}

async function loadHTMLVersions() {
    try {
        const versions = await GetHTMLVersions();
        const current = await GetCurrentHTMLVersion();
        htmlVersionSelect.innerHTML = versions.map(v =>
            `<option value="${v.value}" data-full-label="${v.label}" ${v.value === current ? 'selected' : ''}>${v.label}</option>`
        ).join('');
        updateSelectTitle(htmlVersionSelect);
        collapseSelectedOption(htmlVersionSelect);
    } catch (err) {
        console.error('Failed to load HTML versions:', err);
    }
}

function shortenSelectLabel(label, maxCharacters = 14) {
    const characters = Array.from(label);
    return characters.length > maxCharacters
        ? `${characters.slice(0, maxCharacters - 1).join('')}…`
        : label;
}

function updateSelectTitle(select) {
    const selected = select.options[select.selectedIndex];
    if (!selected) return;
    select.title = selected.dataset.fullLabel || selected.textContent;
}

function restoreOptionLabels(select) {
    Array.from(select.options).forEach((option) => {
        option.textContent = option.dataset.fullLabel || option.textContent;
    });
}

function collapseSelectedOption(select) {
    restoreOptionLabels(select);
    const selected = select.options[select.selectedIndex];
    if (!selected) return;
    selected.textContent = shortenSelectLabel(selected.dataset.fullLabel || selected.textContent);
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
        addLog(current ? '[DEBUG] Disabled' : '[DEBUG] Enabled');
    } catch (err) {
        addLog('[ERROR] ' + err);
    }
}

async function changeEncoding() {
    try {
        await SetEncoding(encodingSelect.value);
        addLog('[CONFIG] Encoding: ' + encodingSelect.value);
    } catch (err) {
        addLog('[ERROR] ' + err);
    }
}

async function changeImageFormat() {
    try {
        await SetImageFormat(imageSelect.value);
        addLog('[CONFIG] Image: ' + imageSelect.value);
    } catch (err) {
        addLog('[ERROR] ' + err);
    }
}

async function changeHTMLVersion() {
    try {
        await SetHTMLVersion(htmlVersionSelect.value);
        addLog('[CONFIG] HTML: ' + htmlVersionSelect.value);
    } catch (err) {
        addLog('[ERROR] ' + err);
    }
}

async function startProxy() {
    const port = parseInt(portInput.value);
    if (isNaN(port) || port < 1 || port > 65535) {
        addLog('[ERROR] Invalid port');
        return;
    }
    startBtn.disabled = true;
    addLog('[PROXY] Starting...');
    try {
        const result = await StartProxy(port);
        addLog('[OK] ' + result);
        await updateStatus();
    } catch (err) {
        addLog('[ERROR] ' + err);
        startBtn.disabled = false;
    }
}

async function stopProxy() {
    stopBtn.disabled = true;
    addLog('[PROXY] Stopping...');
    try {
        const result = await StopProxy();
        addLog('[OK] ' + result);
        await updateStatus();
    } catch (err) {
        addLog('[ERROR] ' + err);
        stopBtn.disabled = false;
    }
}

async function clearLogs() {
    try {
        await ClearLogs();
        logContent.innerHTML = '<div class="log-empty">No activity.</div>';
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
