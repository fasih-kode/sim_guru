(() => {
  const formSelector = '[data-gradebook-form]';
  let dirty = Boolean(document.querySelector(`${formSelector}[data-draft]`));
  let saving = false;

  function status(message) {
    const element = document.querySelector('[data-gradebook-status]');
    if (element) element.textContent = message;
  }

  document.addEventListener('input', (event) => {
    if (event.target.closest(formSelector)) {
      dirty = true;
      status('Draft belum disimpan.');
    }
  });

  document.addEventListener('submit', async (event) => {
    const form = event.target;
    if (!form.matches(formSelector)) return;
    event.preventDefault();
    if (saving) return;
    const data = new FormData(form);
    if (event.submitter && event.submitter.name) data.append(event.submitter.name, event.submitter.value);
    const review = data.get('action') === 'review_latest';
    const body = new URLSearchParams(data);
    const table = form.querySelector('.gradebook-scroll');
    const buttons = [...form.querySelectorAll('button[type="submit"]')];
    saving = true;
    dirty = true;
    if (table) table.inert = true;
    buttons.forEach((button) => { button.disabled = true; });
    status(review ? 'Memuat versi terbaru untuk ditinjau…' : 'Sedang menyimpan…');
    try {
      const response = await fetch(form.action, { method: 'POST', body, credentials: 'same-origin' });
      const html = await response.text();
      const parsed = new DOMParser().parseFromString(html, 'text/html');
      const replacement = parsed.querySelector('#content');
      const existing = document.querySelector('#content');
      const confirmed = response.ok && response.redirected && replacement && replacement.querySelector('[data-gradebook-confirmed]');
      const draftResponse = replacement && replacement.querySelector(`${formSelector}[data-draft]`) && (response.status === 409 || response.status === 422 || (review && response.ok && !response.redirected));
      if (!existing || (!confirmed && !draftResponse)) throw new Error('Respons penyimpanan tidak dapat diverifikasi');
      existing.replaceWith(replacement);
      if (window.htmx) window.htmx.process(replacement);
      dirty = !confirmed;
      if (confirmed) {
        history.replaceState(null, '', response.url);
        status('Berhasil disimpan.');
      } else {
        status(review ? 'Draft belum disimpan. Periksa versi terbaru sebelum menyimpan.' : 'Penyimpanan gagal. Draft belum disimpan.');
      }
    } catch (_) {
      dirty = true;
      status('Penyimpanan gagal atau koneksi terputus. Draft belum disimpan.');
    } finally {
      saving = false;
      if (table) table.inert = false;
      buttons.forEach((button) => { button.disabled = false; });
    }
  });

  document.addEventListener('htmx:beforeSwap', (event) => {
    const target = event.detail && event.detail.target;
    if (dirty && target && (target.id === 'content' || target.contains(document.querySelector(formSelector)))) {
      event.preventDefault();
      status('Draft belum disimpan. Simpan atau buang draft sebelum berpindah.');
    }
  });
  document.addEventListener('htmx:afterSwap', (event) => {
    if (!dirty && event.detail && event.detail.target && event.detail.target.id === 'content') {
      dirty = Boolean(document.querySelector(`${formSelector}[data-draft]`));
    }
  });
  window.addEventListener('beforeunload', (event) => {
    if (!dirty) return;
    event.preventDefault();
    event.returnValue = '';
  });
})();
