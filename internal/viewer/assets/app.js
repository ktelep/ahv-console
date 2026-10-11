import RFB from './novnc/core/rfb.js';
import { initLogging } from './novnc/core/util/logging.js';

// noVNC debug output can include protocol data. Keep all diagnostics controlled
// by this page and never copy upstream error payloads into the DOM or console.
initLogging('none');
const status = document.getElementById('status');
const cad = document.getElementById('cad');
const screen = document.getElementById('screen');
let rfb;
let generation = 0;

async function connect() {
  const current = ++generation;
  if (rfb) rfb.disconnect();
  cad.disabled = true;
  status.textContent = 'Connecting…';
  try {
    const response = await fetch('/session', { credentials: 'same-origin', cache: 'no-store' });
    if (!response.ok) throw new Error('unauthorized');
    const state = await response.json();
    if (current !== generation) return;
    if (state.error) { status.textContent = state.error; return; }
    screen.replaceChildren();
    rfb = new RFB(screen, `ws://${location.host}/ws`);
    rfb.scaleViewport = true;
    rfb.resizeSession = false;
    rfb.addEventListener('connect', () => {
      if (current !== generation) return;
      status.textContent = 'Connected. Stop the command with Ctrl+C to end this session.';
      cad.disabled = false;
      rfb.focus();
    });
    rfb.addEventListener('disconnect', async () => {
      if (current !== generation) return;
      cad.disabled = true;
      status.textContent = 'Disconnected. Reconnect while authorization is valid, or rerun the command.';
      try {
        const response = await fetch('/session', { cache: 'no-store' });
        if (response.ok) {
          const state = await response.json();
          if (current === generation && state.error) status.textContent = state.error;
        }
      } catch { /* The owning command may have stopped. */ }
    });
    rfb.addEventListener('credentialsrequired', () => {
      if (current !== generation) return;
      status.textContent = 'Unexpected RFB authentication request. Rerun the command and check PC compatibility.';
      rfb.disconnect();
    });
    rfb.addEventListener('securityfailure', () => {
      if (current === generation) status.textContent = 'Console authorization rejected. Rerun the command.';
    });
  } catch {
    if (current === generation) status.textContent = 'Local session unavailable. Use the URL printed by the running command, or rerun it.';
  }
}

document.getElementById('reconnect').addEventListener('click', connect);
cad.addEventListener('click', () => rfb?.sendCtrlAltDel());
window.addEventListener('pagehide', () => rfb?.disconnect());

try {
  const bootstrap = location.hash.slice(1);
  history.replaceState(null, '', location.pathname);
  if (bootstrap) {
    const response = await fetch('/bootstrap', {
      method: 'POST', credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ secret: bootstrap }),
    });
    if (!response.ok) throw new Error('bootstrap rejected');
  }
  await connect();
} catch {
  status.textContent = 'This launch URL was already used or is invalid. Refresh the authorized tab, or rerun the command.';
}
