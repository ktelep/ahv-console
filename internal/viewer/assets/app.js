import RFB from './novnc/core/rfb.js';
import { initLogging } from './novnc/core/util/logging.js';

// Never emit raw upstream protocol data or authorization to browser diagnostics.
initLogging('none');
const element = id => document.getElementById(id);
const status = element('status');
const cad = element('cad');
const screen = element('screen');
const panel = element('media-panel');
const driveSelect = element('drive-select');
const isoSelect = element('iso-select');
const actionButtons = [...document.querySelectorAll('[data-action]')];
let rfb;
let generation = 0;
let stateGeneration = 0;
let inventoryGeneration = 0;
let state;
let drives = [];
let images = [];
let imagesAvailable = false;
let inventoryBusy = false;
let operation;
let submitting = false;
let pollingOperation = false;
let routineTimer;
let ended = false;

async function api(path, options = {}) {
  const abort = new AbortController();
  const timer = setTimeout(() => abort.abort(), 10000);
  try {
    const response = await fetch(path, { credentials: 'same-origin', cache: 'no-store', ...options, signal: abort.signal });
    const json = response.headers.get('content-type')?.includes('application/json') ? await response.json() : null;
    if (!response.ok) throw new Error(json?.error || `Local request failed (HTTP ${response.status}); refresh or rerun the command.`);
    return json;
  } finally { clearTimeout(timer); }
}

const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const pending = () => submitting || operation?.status === 'pending';

function controls() {
  const fresh = state && !state.stale && Date.now() - Date.parse(state.observedAt) < 15000;
  for (const button of actionButtons) {
    button.disabled = pending() || !fresh || (button.dataset.action === 'power-on' ? state.power !== 'OFF' : state.power !== 'ON');
  }
  const drive = drives.find(item => item.extId === driveSelect.value);
  const image = images.find(item => item.extId === isoSelect.value);
  element('mount').disabled = pending() || inventoryBusy || !drive || !imagesAvailable || !image || image.availability === 'unavailable';
  element('eject').disabled = pending() || inventoryBusy || !drive || !drive.backingInfo;
  element('media-refresh').disabled = inventoryBusy;
}

function disconnectOff() {
  ++generation;
  rfb?.disconnect();
  rfb = undefined;
  cad.disabled = true;
  screen.replaceChildren();
  status.textContent = 'VM is powered off. Mount an ISO if needed, then use Power on.';
}

async function refreshState() {
  const current = ++stateGeneration;
  try {
    const next = await api('/api/state');
    if (current !== stateGeneration || ended) return state;
    const previous = state;
    state = next;
    element('vm-identity').textContent = `${state.name} (${state.id})`;
    document.title = `${state.name} — AHV Console`;
    const label = state.power === 'ON' ? 'On' : state.power === 'OFF' ? 'Off' : state.power === 'PAUSED' ? 'Paused' : 'Unknown';
    element('power-state').textContent = `Power: ${label}${state.stale ? ' (stale)' : ''}`;
    element('management-status').textContent = state.managementError || '';
    if (!state.stale && state.power === 'OFF' && (previous?.power !== 'OFF' || rfb)) disconnectOff();
    if (state.operation && (!operation || operation.id === state.operation.id || operation.status !== 'pending')) {
      operation = state.operation;
      element('operation-status').textContent = operation.message;
      if (operation.status === 'pending') pollOperation(operation);
    }
    controls();
    return state;
  } catch (error) {
    if (current !== stateGeneration || ended) return state;
    if (state) state.stale = true;
    element('power-state').textContent = 'Power: Unknown (stale)';
    element('management-status').textContent = error.message;
    controls();
    return state;
  }
}

async function connect() {
  const current = ++generation;
  rfb?.disconnect();
  cad.disabled = true;
  status.textContent = 'Connecting…';
  await refreshState();
  if (current !== generation || ended) return;
  if (!state || state.stale || state.power !== 'ON') {
    status.textContent = 'Console unavailable until the VM is observed powered on. Power/media controls remain available.';
    return;
  }
  screen.replaceChildren();
  const connection = new RFB(screen, `ws://${location.host}/ws`);
  rfb = connection;
  connection.scaleViewport = true;
  connection.resizeSession = false;
  let securityError = '';
  connection.addEventListener('connect', () => {
    if (current !== generation) return;
    status.textContent = 'Connected. Stop the command with Ctrl+C to end this session.';
    cad.disabled = false;
    connection.focus();
  });
  connection.addEventListener('disconnect', async () => {
    if (current !== generation) return;
    cad.disabled = true;
    status.textContent = securityError || 'Console not ready or disconnected. Use Reconnect while authorization is valid, or rerun the command.';
    try {
      const diagnostic = await api('/session');
      if (current === generation && diagnostic.error) status.textContent = diagnostic.error;
    } catch { /* The foreground process may have stopped. */ }
  });
  connection.addEventListener('credentialsrequired', () => {
    if (current !== generation) return;
    securityError = 'Unexpected RFB authentication request. Rerun the command and check PC compatibility.';
    status.textContent = securityError;
    connection.disconnect();
  });
  connection.addEventListener('securityfailure', () => {
    if (current !== generation) return;
    securityError = 'Console authorization rejected. Rerun the command.';
    status.textContent = securityError;
  });
}

function mediaLabel(drive) {
  if (!drive.backingInfo) return 'Empty';
  const reference = drive.backingInfo.dataSource?.reference;
  const image = images.find(item => item.extId === reference?.imageExtId);
  return image?.name || 'Mounted media';
}

function showDrive() {
  const drive = drives.find(item => item.extId === driveSelect.value);
  element('current-media').textContent = drive ? `Current media: ${mediaLabel(drive)}` : 'No CD-ROM devices';
  controls();
}

function showImages() {
  const selected = isoSelect.value;
  const query = element('iso-search').value.toLocaleLowerCase();
  const matching = images.filter(image => image.name.toLocaleLowerCase().includes(query));
  const nameCounts = new Map();
  const nameIndexes = new Map();
  const labels = new Map();
  for (const image of images) nameCounts.set(image.name, (nameCounts.get(image.name) || 0) + 1);
  // Number duplicate names in full inventory order so searching keeps labels stable.
  for (const image of images) {
    const index = (nameIndexes.get(image.name) || 0) + 1;
    nameIndexes.set(image.name, index);
    labels.set(image.extId, nameCounts.get(image.name) > 1 ? `${image.name} (${index})` : image.name);
  }
  isoSelect.replaceChildren();
  for (const image of matching) {
    const availability = image.availability === 'ready' ? 'Located on VM cluster' : image.availability === 'unavailable' ? 'Unavailable on VM cluster' : 'Availability unknown (PC decides)';
    const size = image.sizeBytes > 0 ? `${(image.sizeBytes / (1024 * 1024)).toFixed(1)} MiB` : 'Size unknown';
    const option = new Option(`${labels.get(image.extId)} · ${size} · ${availability}`, image.extId);
    option.disabled = image.availability === 'unavailable';
    isoSelect.add(option);
  }
  if (matching.some(image => image.extId === selected)) isoSelect.value = selected;
  else isoSelect.selectedIndex = -1;
  element('iso-status').textContent = !imagesAvailable ? 'ISO inventory unavailable; permitted power and eject actions remain independent.' : images.length === 0 ? 'No ISO images' : matching.length === 0 ? 'No matching ISO images' : `${matching.length} ISO image(s). PC determines insertion success.`;
  showDrive();
}

async function refreshMedia() {
  const current = ++inventoryGeneration;
  inventoryBusy = true;
  imagesAvailable = false;
  element('inventory-status').textContent = 'Loading CD-ROM and ISO inventory…';
  controls();
  const results = await Promise.allSettled([api('/api/drives'), api('/api/images')]);
  if (current !== inventoryGeneration || ended) return;
  const selectedDrive = driveSelect.value;
  drives = results[0].status === 'fulfilled' ? results[0].value : [];
  images = results[1].status === 'fulfilled' ? results[1].value : [];
  imagesAvailable = results[1].status === 'fulfilled';
  driveSelect.replaceChildren();
  for (const drive of drives) driveSelect.add(new Option(`${drive.diskAddress?.busType || 'Unknown bus'} ${drive.diskAddress?.index ?? '?'}`, drive.extId));
  if (drives.some(drive => drive.extId === selectedDrive)) driveSelect.value = selectedDrive;
  driveSelect.disabled = drives.length === 0;
  inventoryBusy = false;
  const errors = results.filter(result => result.status === 'rejected').map(result => result.reason.message);
  element('inventory-status').textContent = errors.join(' ') || (drives.length === 0 ? 'No CD-ROM devices' : 'Inventory refreshed. Mount ISO and Power on are separate actions.');
  showImages();
}

async function startConsoleAfterPowerOn() {
  const deadline = Date.now() + 30000;
  while (!ended && Date.now() < deadline) {
    await refreshState();
    if (state && !state.stale && state.power === 'ON') { await connect(); return; }
    await delay(1000);
  }
  if (!ended) status.textContent = 'Power-on task finished, but console readiness is unconfirmed. Refresh state or use Reconnect when on.';
}

async function pollOperation(initial) {
  if (pollingOperation || ended) return;
  pollingOperation = true;
  const deadline = Date.now() + 150000;
  let terminal;
  try {
    while (!ended && Date.now() < deadline) {
      const next = await api(`/api/operations/${initial.id}`);
      if (operation?.id !== initial.id) return;
      operation = next;
      element('operation-status').textContent = next.message;
      controls();
      if (next.status !== 'pending') { terminal = next; break; }
      await delay(1000);
    }
    if (!terminal && !ended) element('operation-status').textContent = 'Operation outcome is unconfirmed; refresh state and inspect Prism before retrying. No action was replayed.';
  } catch (error) {
    element('operation-status').textContent = `Operation outcome is unconfirmed. ${error.message} No action was replayed.`;
  } finally {
    pollingOperation = false;
  }
  if (ended) return;
  await refreshState();
  if (!panel.hidden) await refreshMedia();
  if (terminal?.status === 'succeeded' && initial.action === 'power-on') await startConsoleAfterPowerOn();
}

async function submit(action, driveId, imageId) {
  if (pending()) return;
  submitting = true;
  ++stateGeneration;
  controls();
  element('operation-status').textContent = 'Submitting…';
  try {
    operation = await api('/api/actions', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ action, ...(driveId ? { driveId } : {}), ...(imageId ? { imageId } : {}) }) });
    element('operation-status').textContent = operation.message;
  } catch (error) {
    element('operation-status').textContent = `Submission not confirmed. ${error.message} Refresh state before retrying; no automatic replay.`;
    await refreshState();
    if (!panel.hidden) await refreshMedia();
  } finally { submitting = false; controls(); }
  if (operation?.status === 'pending') pollOperation(operation);
}

for (const button of actionButtons) button.addEventListener('click', () => { element('power-menu').open = false; submit(button.dataset.action); });
element('mount').addEventListener('click', () => submit('insert', driveSelect.value, isoSelect.value));
element('eject').addEventListener('click', () => submit('eject', driveSelect.value));
element('media-refresh').addEventListener('click', refreshMedia);
driveSelect.addEventListener('change', showDrive);
isoSelect.addEventListener('change', controls);
element('iso-search').addEventListener('input', showImages);
element('media-toggle').addEventListener('click', () => {
  panel.hidden = !panel.hidden;
  element('media-toggle').setAttribute('aria-expanded', String(!panel.hidden));
  if (!panel.hidden) { refreshMedia(); driveSelect.focus(); }
});
element('media-close').addEventListener('click', () => { panel.hidden = true; element('media-toggle').setAttribute('aria-expanded', 'false'); element('media-toggle').focus(); });
for (const area of [document.querySelector('header'), panel]) area.addEventListener('focusin', () => rfb?.blur());
element('reconnect').addEventListener('click', connect);
cad.addEventListener('click', () => rfb?.sendCtrlAltDel());

function scheduleRoutine() {
  clearTimeout(routineTimer);
  if (document.hidden || ended) return;
  routineTimer = setTimeout(async () => { await refreshState(); scheduleRoutine(); }, 5000);
}
document.addEventListener('visibilitychange', () => { clearTimeout(routineTimer); if (!document.hidden && !ended) refreshState().finally(scheduleRoutine); });
window.addEventListener('pagehide', () => { ended = true; ++generation; ++stateGeneration; ++inventoryGeneration; clearTimeout(routineTimer); rfb?.disconnect(); });

try {
  const bootstrap = location.hash.slice(1);
  history.replaceState(null, '', location.pathname);
  if (bootstrap) await api('/bootstrap', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ secret: bootstrap }) });
  await refreshState();
  if (state && !state.stale && state.power === 'ON') await connect();
  scheduleRoutine();
} catch {
  status.textContent = 'This launch URL was already used or is invalid. Refresh the authorized tab, or rerun the command.';
}
