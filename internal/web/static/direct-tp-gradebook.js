(() => {
  const formSelector = '[data-gradebook-form]';
  let dirty = Boolean(document.querySelector(`${formSelector}[data-draft]`));
  let saving = false;

  function status(message) {
    const element = document.querySelector('[data-gradebook-status]');
    if (element) element.textContent = message;
  }

  function filterStudents() {
    const filters = document.querySelector('[data-gradebook-filters]');
    if (!filters) return;
    filters.hidden = false;
    const search = filters.querySelector('[data-gradebook-search]').value.trim().toLocaleLowerCase('id');
    const emptyOnly = filters.querySelector('[data-gradebook-empty]').checked;
    const rows = [...document.querySelectorAll('[data-gradebook-student]')];
    let visible = 0;
    for (const row of rows) {
      const matches = row.dataset.gradebookStudent.toLocaleLowerCase('id').includes(search);
      const hasEmpty = [...row.querySelectorAll('input[name^="score_"]:not([type="hidden"])')].some((input) => !input.value.trim());
      row.hidden = !matches || (emptyOnly && !hasEmpty);
      if (!row.hidden) visible++;
    }
    filters.querySelector('[data-gradebook-count]').textContent = `${visible} dari ${rows.length} siswa · Simpan mencakup seluruh tabel`;
  }

  filterStudents();
  document.addEventListener('input', (event) => {
    if (event.target.closest('[data-gradebook-filters]')) {
      filterStudents();
      return;
    }
    if (event.target.closest(formSelector)) {
      dirty = true;
      status('Draft belum disimpan.');
    }
  });

  document.addEventListener('formdata', (event) => {
    const form = event.target;
    if (!dirty || !form.matches('[data-gradebook-context]')) return;
    // Tujuan sudah tercatat dalam data GET. Pulihkan pemilih bila halaman ini tetap terbuka.
    form.reset();
  });

  document.addEventListener('submit', async (event) => {
    const form = event.target;
    if (!form.matches(formSelector)) return;
    event.preventDefault();
    if (saving) return;
    const data = new FormData(form);
    if (event.submitter && event.submitter.name) data.append(event.submitter.name, event.submitter.value);
    const exam = form.hasAttribute('data-gradebook-exam');
    if (exam && ![...data].some(([name, value]) => name.startsWith('score_') && value.trim() !== '')) {
      status('Isi sedikitnya satu nilai sebelum menyimpan.');
      return;
    }
    const review = data.get('action') === 'review_latest';
    if (!exam && !review && ![...data].some(([name, value]) => name.startsWith('score_') && value !== data.get(`original_${name.slice(6)}`))) {
      dirty = false;
      status('Tidak ada perubahan untuk disimpan.');
      return;
    }
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
      if (exam && !confirmed && replacement && (response.status === 409 || response.status === 422)) {
        const error = replacement.querySelector('[data-exam-save-error]');
        status(error ? error.textContent : 'Penyimpanan gagal. Draft belum disimpan.');
        return;
      }
      if (!existing || (!confirmed && !draftResponse)) throw new Error('Respons penyimpanan tidak dapat diverifikasi');
      existing.replaceWith(replacement);
      if (window.htmx) window.htmx.process(replacement);
      dirty = !confirmed;
      filterStudents();
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
      filterStudents();
    }
  });
  window.addEventListener('beforeunload', (event) => {
    if (!dirty) return;
    event.preventDefault();
    event.returnValue = '';
  });
})();
