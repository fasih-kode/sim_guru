const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const source = fs.readFileSync('internal/web/static/direct-tp-gradebook.js', 'utf8');

function browser(fetch) {
  const listeners = {}, windowListeners = {};
  const status = { textContent: '' };
  const button = { disabled: false };
  const table = { inert: false };
  const form = { action: '/guru/buku-nilai', matches: () => true, querySelector: () => table, querySelectorAll: () => [button], elements: [
    { name: 'score_1_2', value: '90' }, { name: 'original_1_2', value: '80' }, { name: 'version_1_2', value: '0' },
    { name: 'score_3_2', value: '70' }, { name: 'original_3_2', value: '70' }, { name: 'version_3_2', value: '0' },
    { name: 'score_disabled', value: '100', disabled: true }
  ] };
  const content = { replaceWith: () => {}, contains: () => true };
  const document = {
    addEventListener: (name, fn) => { listeners[name] = fn; },
    querySelector: (selector) => selector === '[data-gradebook-status]' ? status : selector === '#content' ? content : selector.includes('[data-draft]') ? null : form
  };
  const window = { addEventListener: (name, fn) => { windowListeners[name] = fn; } };
  class FormDataStub { constructor(form) { this.entries = form.elements.filter(e => e.name && !e.disabled).map(e => [e.name, e.value]); } append(k, v) { this.entries.push([k,v]); } get(k) { return this.entries.find(x => x[0] === k)?.[1]; } [Symbol.iterator]() { return this.entries[Symbol.iterator](); } }
  const replacement = { querySelector: (selector) => selector === '[data-gradebook-confirmed]' ? {} : null };
  class DOMParserStub { parseFromString() { return { querySelector: () => replacement }; } }
  vm.runInNewContext(source, { document, window, fetch, FormData: FormDataStub, URLSearchParams, DOMParser: DOMParserStub, history: { replaceState() {} } });
  const input = () => listeners.input({ target: { closest: () => form } });
  const unload = () => { let blocked = false; windowListeners.beforeunload({ preventDefault() { blocked = true; } }); return blocked; };
  const submit = () => listeners.submit({ target: form, preventDefault() {} });
  return { input, unload, submit, status, listeners, form };
}

test('draft melindungi navigasi biasa dan swap HTMX', () => {
  const b = browser(() => {}); b.input();
  assert.match(b.status.textContent, /belum disimpan/i); assert.equal(b.unload(), true);
  let blocked = false; b.listeners['htmx:beforeSwap']({ detail: { target: { id: 'content' } }, preventDefault() { blocked = true; } });
  assert.equal(blocked, true);
});
test('submit menserialisasi nilai, nilai awal dan versi semua kontrol yang aktif', async () => {
  let sent;
  const b = browser(async (_url, options) => { sent = options.body; throw new Error('offline'); });
  b.input(); await b.submit();
  assert.equal(sent.get('score_1_2'), '90'); assert.equal(sent.get('original_1_2'), '80');
  assert.equal(sent.get('score_3_2'), '70'); assert.equal(sent.get('original_3_2'), '70');
  assert.equal(sent.has('score_disabled'), false);
});
test('nilai yang dikembalikan ke nilai awal tidak mengirim permintaan', async () => {
  let called = false;
  const b = browser(async () => { called = true; });
  b.form.elements[0].value = '80'; b.input(); await b.submit();
  assert.equal(called, false); assert.equal(b.unload(), false);
  assert.match(b.status.textContent, /tidak ada perubahan/i);
});
test('gagal jaringan tidak menghapus draft atau perlindungan navigasi', async () => {
  const b = browser(async () => { throw new Error('offline'); }); b.input(); await b.submit();
  assert.match(b.status.textContent, /gagal|terputus/i); assert.equal(b.unload(), true);
});
test('respons tanpa bukti penyimpanan tidak dianggap berhasil', async () => {
  const b = browser(async () => ({ ok: false, redirected: false, status: 500, text: async () => 'gagal' }));
  b.input(); await b.submit();
  assert.match(b.status.textContent, /gagal/i); assert.equal(b.unload(), true);
});
test('submit menunggu bukti respons sebelum menandai berhasil', async () => {
  let finish; const b = browser(() => new Promise(resolve => { finish = resolve; })); b.input();
  const pending = b.submit(); assert.match(b.status.textContent, /menyimpan/i); assert.equal(b.unload(), true);
  finish({ ok: true, redirected: true, status: 200, url: '/guru/buku-nilai', text: async () => 'ok' }); await pending;
  assert.match(b.status.textContent, /berhasil/i); assert.equal(b.unload(), false);
});
