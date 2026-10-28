// Pola bersama GET/HTMX: komponen daftar berganti, layout dan node input
// pencarian tetap hidup. Form mutasi tidak pernah ditingkatkan menjadi pencarian.
const listRequests = new WeakMap();
const localSearchValues = new Map();
let listRevision = 0;
let listTimer;
let lastListForm;
let lastListTrigger;
let listFocus;

function listMessage(message, retry = false) {
  const region = document.querySelector('[data-list-region]');
  if (!region) return;
  region.querySelector('[data-list-message]').textContent = message;
  region.querySelector('[data-list-retry]').hidden = !retry;
}

function listProtected(region) {
  return Boolean(region.querySelector('dialog[open]') || [...region.querySelectorAll('form[data-list-dirty="true"]')].some(form => !form.closest('dialog')));
}

function initializeLiveLists(root = document) {
  const region = root.matches?.('[data-list-region]') ? root : root.querySelector('[data-list-region]');
  if (!region) return;
  region.classList.add('live-list-ready');
  region.querySelectorAll('form').forEach((form, index) => {
    if (form.method.toLowerCase() !== 'get') return;
    form.id ||= `list-filter-${index}`;
    form.dataset.listFilter = '';
    form.setAttribute('hx-get', form.getAttribute('action') || location.pathname);
    form.setAttribute('hx-trigger', 'list-filter, submit');
    form.setAttribute('hx-target', '#list-region');
    form.setAttribute('hx-select', '#list-region');
    form.setAttribute('hx-swap', 'outerHTML');
    form.setAttribute('hx-sync', '#list-region:replace');
    form.setAttribute('hx-indicator', '#list-loading');
    // URL diganti setelah respons lolos pemeriksaan versi, tanpa entri tiap ketikan.
    form.querySelectorAll('input[type="search"][name]').forEach(input => {
      input.id ||= `${form.id}-search`;
      input.setAttribute('hx-preserve', 'true');
    });
  });
  region.querySelectorAll('nav a[href]').forEach(link => {
    const target = new URL(link.href, location.href);
    if (target.pathname !== location.pathname) return;
    link.setAttribute('hx-get', link.getAttribute('href'));
    link.setAttribute('hx-target', '#list-region');
    link.setAttribute('hx-select', '#list-region');
    link.setAttribute('hx-swap', 'outerHTML');
    link.setAttribute('hx-sync', '#list-region:replace');
    link.setAttribute('hx-indicator', '#list-loading');
  });
  region.querySelectorAll('input[type="search"]').forEach(input => {
    if (!input.closest('.table-search')) {
      const box = document.createElement('div'); box.className = 'table-search';
      input.before(box);
      const icon = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      icon.setAttribute('viewBox', '0 0 24 24'); icon.setAttribute('aria-hidden', 'true');
      icon.innerHTML = '<circle cx="11" cy="11" r="7" fill="none" stroke="currentColor" stroke-width="1.8"/><path d="m16 16 4 4" stroke="currentColor" stroke-width="1.8"/>';
      box.append(icon, input);
    }
    if (input.matches('[data-table-search],[data-quick-search]') && localSearchValues.has(input.id)) {
      input.value = localSearchValues.get(input.id);
      input.dispatchEvent(new Event('input', {bubbles:true}));
    }
  });
  htmx.process(region);
}

function triggerList(form) {
  if (!form?.isConnected) return;
  lastListForm = form;
  if (!form.checkValidity()) { listMessage('Lengkapi filter yang valid sebelum memperbarui hasil.'); return; }
  const from = form.elements.namedItem('from') || form.elements.namedItem('starts_on');
  const to = form.elements.namedItem('to') || form.elements.namedItem('ends_on');
  if ((from?.value || to?.value) && (!from?.value || !to?.value || from.value > to.value)) {
    listMessage('Isi rentang tanggal yang lengkap; tanggal mulai tidak boleh melewati tanggal selesai.'); return;
  }
  if (listProtected(form.closest('[data-list-region]'))) { listMessage('Tutup dialog dan simpan atau batalkan isian sebelum mengganti daftar.', true); return; }
  htmx.trigger(form, 'list-filter');
}

function scheduleList(event) {
  const input = event.target;
  const form = input.form;
  if (input.matches('[data-table-search],[data-quick-search]')) { localSearchValues.set(input.id,input.value); return; }
  if (!form?.hasAttribute('data-list-filter')) return;
  if (event.type === 'input' && !input.matches('input[type="search"],input[type="text"]')) return;
  if (input.type === 'search' && input.name) form.closest('[data-list-region]').querySelectorAll('form[data-list-filter] input[type="search"]').forEach(other => { if (other.name === input.name) other.value = input.value; });
  if (input.name === 'year_id') {
    const classes = form.elements.namedItem('class_id');
    if (classes && [...classes.options].some(option => option.value === '')) classes.value = '';
  }
  listRevision++;
  // Abort juga saat debounce belum selesai, bukan baru ketika request berikutnya dimulai.
  document.querySelectorAll('[data-list-filter], nav [hx-get]').forEach(el => htmx.trigger(el, 'htmx:abort'));
  clearTimeout(listTimer);
  listTimer = setTimeout(() => triggerList(form), input.matches('select') ? 0 : 350);
}
document.addEventListener('input', scheduleList);
document.addEventListener('change', scheduleList);
document.addEventListener('search', scheduleList);

for (const kind of ['input','change','click']) document.addEventListener(kind, event => {
  const input = event.target.closest('input,select,textarea,button');
  const form = input?.form;
  if (!form || form.method !== 'post' || !form.closest('[data-list-region]')) return;
  if (input.matches('[type="search"],[data-table-column-filter]') || input.closest('nav')) return;
  if (kind === 'click' && (!input.matches('button[type="button"]') || input.hasAttribute('data-modal-close'))) return;
  form.dataset.listDirty = 'true';
});
document.addEventListener('click', event => {
  if (event.target.closest('[data-list-retry]')) {
    if (lastListTrigger?.matches('a[hx-get]') && lastListTrigger.isConnected) htmx.trigger(lastListTrigger, 'click');
    else triggerList(lastListForm?.isConnected ? lastListForm : document.querySelector('[data-list-filter]'));
  }
});

document.body.addEventListener('htmx:configRequest', event => {
  if (event.detail.verb !== 'get' || !event.detail.elt.closest('[data-list-region]')) return;
  event.detail.headers['HX-List-Request'] = 'true';
  if (event.detail.elt.hasAttribute('data-list-filter')) {
    const current = new FormData(event.detail.elt);
    // Beberapa toolbar GET pada halaman yang sama saling melengkapi konteks.
    event.detail.elt.closest('[data-list-region]').querySelectorAll('form[data-list-filter]').forEach(form => {
      for (const [name,value] of new FormData(form)) if (!current.has(name)) event.detail.parameters[name] = value;
    });
    for (const page of ['page','day_page','request_page','history_page']) {
      if (page === 'page' || event.detail.parameters[page] != null) event.detail.parameters[page] = '1';
    }
  }
});
document.body.addEventListener('htmx:beforeRequest', event => {
  const region = event.detail.elt.closest('[data-list-region]');
  if (!region) return;
  if (listProtected(region)) { event.preventDefault(); listMessage('Tutup dialog dan simpan atau batalkan isian sebelum mengganti daftar.', true); return; }
  lastListTrigger = event.detail.elt;
  const input = document.activeElement;
  listRequests.set(event.detail.xhr, {revision:listRevision, region, focused:input?.id, selection:input?.selectionStart, panels:[...region.querySelectorAll('.table-filter-panel[id]')].filter(p => !p.hidden).map(p => p.id)});
  region.setAttribute('aria-busy','true');
  listMessage('');
});
document.body.addEventListener('htmx:beforeSwap', event => {
  const request = listRequests.get(event.detail.xhr);
  if (!request) return;
  const {xhr} = event.detail;
  if (request.revision !== listRevision || !request.region.isConnected || listProtected(request.region)) { event.detail.shouldSwap = false; return; }
  if (xhr.status !== 200 || !new DOMParser().parseFromString(xhr.responseText,'text/html').querySelector('#list-region')) {
    event.detail.shouldSwap = false;
    listMessage(xhr.status === 401 || xhr.status === 403 ? 'Sesi atau hak akses berubah. Muat ulang halaman untuk melanjutkan.' : 'Hasil belum dapat diperbarui. Periksa filter atau coba lagi.',true);
    return;
  }
  listFocus = request;
});
document.body.addEventListener('htmx:afterSwap', event => {
  if (event.detail.target.id !== 'list-region') return;
  const request = listFocus;
  const url = event.detail.xhr.responseURL;
  if (url && new URL(url).origin === location.origin) history.replaceState(history.state, '', url);
  listFocus = null;
  queueMicrotask(() => {
    const region = document.getElementById('list-region');
    if (!region) return;
    listMessage('Hasil diperbarui.');
    initializeLiveLists(region);
    for (const id of request?.panels || []) {
      const panel = document.getElementById(id);
      if (panel) { panel.hidden = false; document.querySelector(`[aria-controls="${CSS.escape(id)}"]`)?.setAttribute('aria-expanded','true'); }
    }
    const input = request?.focused && document.getElementById(request.focused);
    if (input) { input.focus({preventScroll:true}); if (input.type === 'search' && request.selection != null) input.setSelectionRange(request.selection,request.selection); }
  });
});
document.body.addEventListener('htmx:afterRequest', event => {
  const request = listRequests.get(event.detail.xhr);
  request?.region.removeAttribute('aria-busy');
  document.querySelector('[data-list-region]')?.removeAttribute('aria-busy');
  if (request && event.detail.failed && request.revision === listRevision) listMessage('Hasil belum dapat diperbarui. Periksa koneksi dan coba lagi.',true);
});
document.addEventListener('DOMContentLoaded', () => initializeLiveLists());
