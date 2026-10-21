htmx.config.allowEval = false;
htmx.config.includeIndicatorStyles = false;
htmx.config.selfRequestsOnly = true;
htmx.config.historyCacheSize = 0;

const notificationTimers = new WeakMap();
const notificationTimeout = 10_000;

function dismissNotification(notification) {
  const timer = notificationTimers.get(notification);
  if (timer) window.clearTimeout(timer);
  notificationTimers.delete(notification);
  notification.classList.add('is-dismissing');
  window.setTimeout(() => {
    notification.classList.remove('is-dismissing');
    if (notification.hasAttribute('data-notification-reusable')) {
      notification.hidden = true;
      const progress = notification.querySelector('.app-notification-progress');
      if (progress) progress.style.animation = 'none';
      return;
    }
    notification.remove();
  }, 180);
}

function initializeNotification(notification) {
  if (notification.hidden) return;
  if (!notification.dataset.notificationInitialized) {
    notification.querySelector('[data-notification-close]')?.addEventListener('click', () => {
      dismissNotification(notification);
    });
    notification.dataset.notificationInitialized = 'true';
  }
  const progress = notification.querySelector('.app-notification-progress');
  if (progress) {
    progress.style.animation = 'none';
    void progress.offsetWidth;
    progress.style.animation = '';
  }
  const previousTimer = notificationTimers.get(notification);
  if (previousTimer) window.clearTimeout(previousTimer);
  notification.classList.remove('is-dismissing');
  notificationTimers.set(notification, window.setTimeout(() => dismissNotification(notification), notificationTimeout));
}

function initializeNotifications(root = document) {
  root.querySelectorAll('[data-notification]').forEach(initializeNotification);
}

function showNotification(notification) {
  if (!notification) return;
  notification.hidden = false;
  initializeNotification(notification);
}

initializeNotifications();
document.body.addEventListener('htmx:afterSwap', (event) => initializeNotifications(event.target));

for (const event of ['htmx:responseError', 'htmx:sendError', 'htmx:timeout']) {
  document.body.addEventListener(event, () => {
    showNotification(document.getElementById('request-error'));
  });
}

document.querySelectorAll('[data-form-modal]').forEach((modal) => {
  const returnTo = modal.dataset.modalReturn;
  let opener = document.querySelector(`[href="${CSS.escape(location.pathname)}"]`);
  modal.querySelectorAll('[data-modal-close]').forEach((button) => {
    button.addEventListener('click', () => modal.close());
  });
  modal.addEventListener('click', (event) => {
    if (event.target === modal) modal.close();
  });
  modal.addEventListener('close', () => {
    if (returnTo && location.pathname !== returnTo) history.replaceState(null, '', returnTo);
    opener?.focus();
  });
  document.querySelectorAll(`[data-modal-open="${CSS.escape(modal.id)}"]`).forEach((button) => {
    button.addEventListener('click', () => {
      opener = button;
      modal.showModal();
    });
  });
  if (modal.hasAttribute('data-modal-open-on-load')) {
    modal.showModal();
    modal.querySelector('input:not([type="hidden"]),select,textarea,button')?.focus({ preventScroll: true });
  }
});
document.body.addEventListener('htmx:beforeRequest', () => {
  const message = document.getElementById('request-error');
  if (message) {
    const timer = notificationTimers.get(message);
    if (timer) window.clearTimeout(timer);
    notificationTimers.delete(message);
    message.hidden = true;
    message.classList.remove('is-dismissing');
  }
});

const sidebar = document.getElementById('app-sidebar');
const sidebarToggle = document.querySelector('[data-sidebar-toggle]');
const sidebarClose = document.querySelector('[data-sidebar-close]');
const sidebarBackdrop = document.getElementById('sidebar-backdrop');
const desktopSidebar = window.matchMedia('(min-width: 1024px)');

if (sidebar && sidebarToggle && sidebarBackdrop) {
  const setSidebarOpen = (open) => {
    const desktop = desktopSidebar.matches;
    const visible = open;
    sidebar.classList.toggle('is-open', !desktop && open);
    sidebar.classList.toggle('is-collapsed', desktop && !open);
    sidebar.setAttribute('aria-hidden', String(!visible));
    sidebar.toggleAttribute('inert', !visible);
    sidebarBackdrop.hidden = desktop || !open;
    sidebarToggle.setAttribute('aria-expanded', String(!desktop && open));
  };

  sidebarToggle.addEventListener('click', () => {
    const isOpen = desktopSidebar.matches
      ? !sidebar.classList.contains('is-collapsed')
      : sidebar.classList.contains('is-open');
    setSidebarOpen(!isOpen);
  });
  sidebarClose?.addEventListener('click', () => setSidebarOpen(false));
  sidebarBackdrop.addEventListener('click', () => setSidebarOpen(false));
  sidebar.addEventListener('click', (event) => {
    if (event.target.closest('a')) setSidebarOpen(false);
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape' && !desktopSidebar.matches) setSidebarOpen(false);
  });
  desktopSidebar.addEventListener('change', () => setSidebarOpen(desktopSidebar.matches));
  setSidebarOpen(desktopSidebar.matches);
}

const liveSearchTimers = new WeakMap();
document.querySelectorAll('[data-instant-search]').forEach((input) => {
  input.addEventListener('input', () => {
    clearTimeout(liveSearchTimers.get(input));
    liveSearchTimers.set(input, window.setTimeout(() => {
      const form = input.form;
      if (!form) return;
      const page = form.elements.namedItem('page');
      if (page) page.value = '1';
      form.requestSubmit();
    }, 350));
  });
});

document.querySelectorAll('[data-instant-filter]').forEach((control) => {
  control.addEventListener('change', () => {
    const form = control.form;
    if (!form) return;
    const page = form.elements.namedItem('page');
    if (page) page.value = '1';
    form.requestSubmit();
  });
});

document.querySelectorAll('[data-filter-toggle]').forEach((button) => {
  const panel = document.getElementById(button.getAttribute('aria-controls'));
  if (!panel) return;
  button.addEventListener('click', () => {
    const open = button.getAttribute('aria-expanded') !== 'true';
    button.setAttribute('aria-expanded', String(open));
    panel.hidden = !open;
  });
});

const localTables = new WeakMap();
function setupLocalTable(table) {
  if (localTables.has(table)) return localTables.get(table);
  const body = table.tBodies[0];
  if (!body) return null;
  const rows = Array.from(body.rows).filter((row) => !row.querySelector('td[colspan]'));
  const placeholder = Array.from(body.rows).find((row) => row.querySelector('td[colspan]'));
  const surface = table.closest('.table-surface');
  const nav = document.createElement('nav');
  nav.className = 'table-pagination table-pagination--attached';
  nav.setAttribute('aria-label', 'Pagination tabel');
  if (surface) surface.insertAdjacentElement('afterend', nav);
  const state = { rows, placeholder, nav, query: '', page: 1, size: 10 };
  const render = () => {
    const matching = rows.filter((row) => row.textContent.toLocaleLowerCase().includes(state.query));
    const totalPages = Math.max(1, Math.ceil(matching.length / state.size));
    state.page = Math.min(state.page, totalPages);
    const start = (state.page - 1) * state.size;
    const visible = new Set(matching.slice(start, start + state.size));
    matching.forEach((row, index) => {
      const number = row.querySelector('[data-row-number]');
      if (number) number.textContent = String(index + 1);
    });
    rows.forEach((row) => { row.hidden = !visible.has(row); });
    if (placeholder) placeholder.hidden = rows.length > 0 || state.query !== '';
    let noResults = body.querySelector('[data-no-search-results]');
    if (state.query && matching.length === 0) {
      if (!noResults) {
        noResults = document.createElement('tr');
        noResults.dataset.noSearchResults = '';
        const cell = document.createElement('td');
        cell.colSpan = table.querySelectorAll('thead th').length || 1;
        cell.textContent = 'Tidak ada hasil pencarian.';
        noResults.append(cell);
        body.append(noResults);
      }
      noResults.hidden = false;
    } else if (noResults) noResults.hidden = true;

    nav.replaceChildren();
    const previous = document.createElement('button');
    previous.type = 'button'; previous.className = 'table-page-arrow'; previous.disabled = state.page <= 1;
    previous.innerHTML = '<svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="m15 18-6-6 6-6M9 12h12" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg><span>Previous</span>';
    previous.addEventListener('click', () => { state.page--; render(); });
    const links = document.createElement('div'); links.className = 'table-page-links';
    const numbers = totalPages <= 7 ? Array.from({length: totalPages}, (_, i) => i + 1) : [1, ...Array.from({length: 3}, (_, i) => state.page - 1 + i).filter((n) => n > 1 && n < totalPages), totalPages];
    const uniqueNumbers = [...new Set(numbers)].sort((a, b) => a - b);
    let prior = 0;
    uniqueNumbers.forEach((number) => {
      if (number - prior > 1) { const dots = document.createElement('span'); dots.textContent = '…'; dots.setAttribute('aria-hidden', 'true'); links.append(dots); }
      const button = document.createElement('button'); button.type = 'button'; button.textContent = String(number);
      button.className = number === state.page ? 'is-current' : '';
      if (number === state.page) button.setAttribute('aria-current', 'page');
      button.addEventListener('click', () => { state.page = number; render(); });
      links.append(button); prior = number;
    });
    const next = document.createElement('button');
    next.type = 'button'; next.className = 'table-page-arrow'; next.disabled = state.page >= totalPages;
    next.innerHTML = '<span>Next</span><svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="m9 18 6-6-6-6m6 6H3" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>';
    next.addEventListener('click', () => { state.page++; render(); });
    nav.append(previous, links, next);
  };
  state.setSearch = (value) => { state.query = value.trim().toLocaleLowerCase(); state.page = 1; render(); };
  localTables.set(table, state);
  render();
  return state;
}

document.querySelectorAll('[data-table-paginate]').forEach(setupLocalTable);
document.querySelectorAll('[data-table-search]').forEach((input) => {
  const table = document.querySelector(input.dataset.tableSearch);
  if (!table) return;
  if (!table.hasAttribute('data-table-paginate')) {
    input.addEventListener('input', () => {
      const query = input.value.trim().toLocaleLowerCase();
      Array.from(table.tBodies[0]?.rows || []).forEach((row) => {
        if (!row.querySelector('td[colspan]')) row.hidden = !row.textContent.toLocaleLowerCase().includes(query);
      });
    });
    return;
  }
  const state = setupLocalTable(table);
  input.addEventListener('input', () => state?.setSearch(input.value));
});
