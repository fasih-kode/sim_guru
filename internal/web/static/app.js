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

// Keep action menus in tables mutually exclusive and dismiss them on outside
// clicks so navigating through a long table never leaves stale menus open.
document.addEventListener('click', (event) => {
  const currentMenu = event.target.closest('.table-actions');
  document.querySelectorAll('details.table-actions[open]').forEach((menu) => {
    if (menu !== currentMenu) menu.open = false;
  });
});

document.addEventListener('keydown', (event) => {
  if (event.key !== 'Escape') return;
  document.querySelectorAll('details.table-actions[open]').forEach((menu) => { menu.open = false; });
});

document.body.addEventListener('htmx:beforeSwap', (event) => {
  const target = event.detail.target;
  const status = event.detail.xhr?.status;
  if (target?.id === 'student-guidance-detail-region' && (status === 409 || status === 422)) {
    event.detail.shouldSwap = true;
    event.detail.isError = false;
  }
});

for (const event of ['htmx:responseError', 'htmx:sendError', 'htmx:timeout']) {
  document.body.addEventListener(event, (detail) => {
    if (detail.detail.elt?.closest('[data-list-region]')) return;
    showNotification(document.getElementById('request-error'));
  });
}

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

// Deep link pada ponsel harus langsung menampilkan tab aktif di dalam strip geser.
const navigationTabsViewport = document.querySelector('[data-navigation-tabs]');
const activeNavigationTab = navigationTabsViewport?.querySelector('[aria-current="page"]');
if (navigationTabsViewport && activeNavigationTab && navigationTabsViewport.scrollWidth > navigationTabsViewport.clientWidth) {
  const tabRect = activeNavigationTab.getBoundingClientRect();
  const viewportRect = navigationTabsViewport.getBoundingClientRect();
  const centerOffset = tabRect.left + tabRect.width / 2 - (viewportRect.left + navigationTabsViewport.clientWidth / 2);
  const maxScroll = navigationTabsViewport.scrollWidth - navigationTabsViewport.clientWidth;
  navigationTabsViewport.scrollLeft = Math.max(0, Math.min(maxScroll, navigationTabsViewport.scrollLeft + centerOffset));
}

// Inisialisasi per elemen mencegah listener ganda setelah hasil HTMX diganti.
const contentCleanup = new WeakMap();
function registerPageCleanup(element, cleanup) { contentCleanup.set(element, cleanup); }
document.body.addEventListener('htmx:beforeCleanupElement', event => {
  contentCleanup.get(event.target)?.(); contentCleanup.delete(event.target);
});
const initializedContent = new WeakMap();
function selectOnce(root, selector) {
  return [...root.querySelectorAll(selector)].filter(element => {
    let selectors = initializedContent.get(element);
    if (!selectors) { selectors = new Set(); initializedContent.set(element, selectors); }
    if (selectors.has(selector)) return false;
    selectors.add(selector); return true;
  });
}
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
  const state = { rows, placeholder, nav, query: '', filters: {}, page: 1, size: 10 };
  const render = () => {
    const matching = rows.filter((row) => (row.dataset.tableSearchText ?? row.textContent).toLocaleLowerCase().includes(state.query)
      && Object.entries(state.filters).every(([column, value]) => !value || (row.dataset.filterGlobal === 'true' && column !== 'day') || row.dataset[`filter${column[0].toUpperCase()}${column.slice(1)}`] === value));
    const totalPages = Math.max(1, Math.ceil(matching.length / state.size));
    state.page = Math.min(state.page, totalPages);
    const start = (state.page - 1) * state.size;
    const visible = new Set(matching.slice(start, start + state.size));
    matching.forEach((row, index) => {
      const number = row.querySelector('[data-row-number]');
      if (number) number.textContent = String(index + 1);
    });
    rows.forEach((row) => { row.hidden = !visible.has(row); });
    if (placeholder) placeholder.hidden = rows.length > 0 || state.query !== '' || Object.values(state.filters).some(Boolean);
    let noResults = body.querySelector('[data-no-search-results]');
    if ((state.query || Object.values(state.filters).some(Boolean)) && matching.length === 0) {
      if (!noResults) {
        noResults = document.createElement('tr');
        noResults.dataset.noSearchResults = '';
        const cell = document.createElement('td');
        cell.colSpan = table.querySelectorAll('thead th').length || 1;
        cell.textContent = 'Tidak ada data yang cocok dengan pencarian.';
        noResults.append(cell);
        body.append(noResults);
      }
      noResults.hidden = false;
    } else if (noResults) noResults.hidden = true;

    if (state.query || Object.values(state.filters).some(Boolean)) listMessage(matching.length ? `${matching.length} data cocok.` : 'Tidak ada data yang cocok dengan pencarian.');
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
  state.setFilter = (column, value) => { state.filters[column] = value; state.page = 1; render(); };
  localTables.set(table, state);
  render();
  return state;
}


function initializePage(root = document) {
  selectOnce(root, '[data-form-modal]').forEach((modal) => {
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
  selectOnce(root, '[data-filter-toggle]').forEach((button) => {
    const panel = document.getElementById(button.getAttribute('aria-controls'));
    if (!panel) return;
    button.addEventListener('click', () => {
      const open = button.getAttribute('aria-expanded') !== 'true';
      button.setAttribute('aria-expanded', String(open));
      panel.hidden = !open;
    });
  });

  selectOnce(root, '[data-table-paginate]').forEach(setupLocalTable);
  selectOnce(root, '[data-table-search]').forEach((input) => {
    const table = document.querySelector(input.dataset.tableSearch);
    if (!table) return;
    const state = setupLocalTable(table);
    const param = input.dataset.tableSearchParam;
    input.addEventListener('input', () => {
      state?.setSearch(input.value);
      if (!param) return;
      const search = input.value.trim();
      const updateURL = (href) => {
        const url = new URL(href);
        if (search) url.searchParams.set(param, search);
        else url.searchParams.delete(param);
        return url.href;
      };
      history.replaceState(history.state, '', updateURL(location.href));
      table.querySelectorAll('[data-table-search-link]').forEach((link) => {
        link.href = updateURL(link.href);
      });
    });
    if (input.value) state?.setSearch(input.value);
  });
  const savedTableFilters = new Map();
  selectOnce(root, '[data-table-column-filter]').forEach((select) => {
    const table = document.querySelector(select.dataset.table);
    if (!table) return;
    const state = setupLocalTable(table);
    const storageKey = `sim-guru:table-filters:${location.pathname}:${table.id || 'table'}`;
    let saved = savedTableFilters.get(storageKey);
    if (!saved) {
      try {
        saved = JSON.parse(sessionStorage.getItem(storageKey) || '{}');
      } catch {
        saved = {};
      }
      savedTableFilters.set(storageKey, saved);
    }
    const column = select.dataset.tableColumnFilter;
    const savedValueExists = typeof saved[column] === 'string' && [...select.options].some((option) => option.value === saved[column]);
    if (savedValueExists) {
      select.value = saved[column];
    } else {
      const hadSavedValue = Object.prototype.hasOwnProperty.call(saved, column);
      delete saved[column];
      if (hadSavedValue) {
        try {
          sessionStorage.setItem(storageKey, JSON.stringify(saved));
        } catch {
          // Filtering still works when browser storage is unavailable.
        }
      }
    }
    state?.setFilter(column, select.value);
    const panel = select.closest('.table-filter-panel');
    const toggle = panel && document.querySelector(`[aria-controls="${panel.id}"]`);
    if (toggle && Object.values(saved).some(Boolean)) {
      panel.hidden = false;
      toggle.setAttribute('aria-expanded', 'true');
    }
    select.addEventListener('change', () => {
      state?.setFilter(column, select.value);
      saved[column] = select.value;
      try {
        sessionStorage.setItem(storageKey, JSON.stringify(saved));
      } catch {
        // Filtering still works when browser storage is unavailable.
      }
      if (toggle && select.value) {
        panel.hidden = false;
        toggle.setAttribute('aria-expanded', 'true');
      }
    });
  });

  // The teacher is determined by the assignment, and rechecked by the server.
  selectOnce(root, '[data-schedule-form]').forEach((form) => {
    const assignments = [...form.querySelector('[data-schedule-assignments]').content.querySelectorAll('option')];
    const periods = [...form.querySelector('[data-schedule-periods]').content.querySelectorAll('option')];
    const classes = form.querySelector('[data-schedule-class]');
    const subjects = form.querySelector('[data-schedule-subject]');
    const teacher = form.querySelector('[data-schedule-teacher]');
    const days = form.querySelector('[data-schedule-day]');
    const start = form.querySelector('[data-schedule-start]');
    const end = form.querySelector('[data-schedule-end]');
    const submit = form.querySelector('[data-schedule-submit]');
    const names = ['', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu', 'Minggu'];
    const choose = (select, options, selected, empty) => {
      select.replaceChildren(...options);
      if (!options.length) select.append(new Option(empty, ''));
      if (options.some((o) => o.value === selected)) select.value = selected;
      select.disabled = !options.length;
    };
    const updateTeacher = () => {
      teacher.value = subjects.selectedOptions[0]?.dataset.teacherName || '';
      submit.disabled = !teacher.value || !start.value || !end.value;
    };
    const updateSubjects = (selected = '') => {
      choose(subjects, assignments.filter((a) => a.dataset.classId === classes.value).map((a) => a.cloneNode(true)), selected, 'Belum ada penugasan');
      updateTeacher();
    };
    const updateEnd = (selected = '') => {
      const available = periods.filter((p) => p.dataset.weekday === days.value);
      let next = Number(start.value);
      const contiguous = [];
      for (const p of available) {
        if (Number(p.value) < next) continue;
        if (Number(p.value) !== next) break;
        const option = p.cloneNode(true);
        option.textContent = `Jam ${p.value} · ${p.dataset.end.slice(0, 5)}`;
        contiguous.push(option); next++;
      }
      choose(end, contiguous, selected, 'Jam belum tersedia');
      updateTeacher();
    };
    const updatePeriods = (selectedStart = '', selectedEnd = '') => {
      choose(start, periods.filter((p) => p.dataset.weekday === days.value).map((p) => {
        const option = p.cloneNode(true);
        option.textContent = `Jam ${p.value} · ${p.dataset.start.slice(0, 5)}`;
        return option;
      }), selectedStart, 'Isi jam pelajaran dahulu');
      updateEnd(selectedEnd);
    };
    const classMap = new Map(assignments.map((a) => [a.dataset.classId, a.dataset.className]));
    choose(classes, [...classMap].map(([id, name]) => new Option(name, id)), form.dataset.selectedClass, 'Belum ada kelas dengan pengampu');
    const dayValues = [1, 2, 3, 4, 5, 6];
    if (periods.some((p) => p.dataset.weekday === '7')) dayValues.push(7);
    choose(days, dayValues.map((d) => new Option(names[d], String(d))), form.dataset.selectedDay, 'Hari belum tersedia');
    updateSubjects(form.dataset.selectedSubject);
    updatePeriods(form.dataset.selectedStart, form.dataset.selectedEnd);
    if (!assignments.length) form.querySelector('[data-schedule-help]').textContent = 'Belum ada pengampu yang memenuhi syarat pada tanggal berlaku draf. Atur Guru Pengampu terlebih dahulu.';
    classes.addEventListener('change', () => updateSubjects());
    subjects.addEventListener('change', updateTeacher);
    days.addEventListener('change', () => updatePeriods());
    start.addEventListener('change', () => updateEnd());
    end.addEventListener('change', updateTeacher);
  });

  // Show generation progress without weakening CSP or repeating submissions.
  selectOnce(root, '[data-generate-form]').forEach((form) => {
    form.addEventListener('submit', () => {
      form.querySelector('[data-generate-progress]').hidden = false;
      form.querySelector('button[type="submit"]').disabled = true;
    });
  });

  // Target selection and a three-state day/period grid, using ordinary clicks
  // and native keyboard controls. Disabled school activities are never painted.
  selectOnce(root, '[data-grid-target-form]').forEach((form) => {
    const kind = form.elements.target_kind;
    const target = form.elements.target_id;
    const update = () => {
      Array.from(target.options).forEach((option) => {
        const disabled = Boolean(option.dataset.targetKind && option.dataset.targetKind !== kind.value);
        option.hidden = disabled;
        option.disabled = disabled;
        if (disabled && option.selected) target.value = '';
      });
    };
    kind.addEventListener('change', () => { target.value = ''; update(); });
    update();
  });
  selectOnce(root, '[data-constraint-grid]').forEach((form) => {
    const cells = [...form.querySelectorAll('[data-grid-cell]')];
    const labels = { available: 'Tersedia', soft: 'Kondisional', hard: 'Tidak tersedia' };
    const symbols = { available: '✓', soft: '?', hard: '×' };
    const currentPaint = () => form.querySelector('input[name="paint_state"]:checked').value;
    const refreshSummary = () => {
      const counts = { available: 0, soft: 0, hard: 0 };
      cells.forEach((cell) => { counts[form.elements[cell.dataset.gridCell].value]++; });
      form.querySelector('[data-grid-summary]').textContent = `${counts.available} Tersedia · ${counts.soft} Kondisional · ${counts.hard} Tidak tersedia`;
    };
    const paint = (cell, state) => {
      form.elements[cell.dataset.gridCell].value = state;
      cell.classList.remove('grid-cell--available', 'grid-cell--soft', 'grid-cell--hard');
      cell.classList.add(`grid-cell--${state}`);
      cell.querySelector('[data-grid-symbol]').textContent = symbols[state];
      cell.querySelector('[data-grid-state-label]').textContent = labels[state];
      const label = `${cell.dataset.cellLabel} · ${labels[state]}`;
      cell.setAttribute('aria-label', label);
      cell.title = label;
    };
    cells.forEach((cell) => cell.addEventListener('click', () => { paint(cell, currentPaint()); refreshSummary(); }));
    form.querySelectorAll('[data-grid-day]').forEach((day) => day.addEventListener('click', () => {
      cells.filter((cell) => cell.dataset.gridWeekday === day.dataset.gridDay).forEach((cell) => paint(cell, currentPaint()));
      refreshSummary();
    }));
    form.querySelector('[data-grid-reset]').addEventListener('click', () => {
      cells.forEach((cell) => paint(cell, 'available'));
      refreshSummary();
    });
    refreshSummary();
  });

  // Preview always uses the last server-validated mapping. Editing a choice must
  // disable confirmation until another preview has been submitted.
  selectOnce(root, '[data-asc-mapping]').forEach((form) => {
    const dirty = () => {
      form.querySelector('[data-asc-dirty]').hidden = false;
      const confirm = document.querySelector('[data-asc-confirm-open]');
      if (confirm) confirm.disabled = true;
    };
    form.addEventListener('change', (event) => { if (event.target.matches('select[name^="map_"]')) dirty(); });
    form.querySelectorAll('[data-asc-period]').forEach((period) => {
      const day = form.elements[period.dataset.dayField];
      const update = (changed) => {
        let selected = changed ? period.dataset.sourceNumber : period.value;
        period.value = '';
        Array.from(period.options).forEach((option) => {
          if (!option.dataset.day) return;
          const allowed = option.dataset.day === day.value;
          option.hidden = !allowed;
          option.disabled = !allowed;
          if (allowed && option.value === selected) option.selected = true;
        });
      };
      day.addEventListener('change', () => { update(true); dirty(); });
      update(false);
    });
  });

  // A destination exists only for promotion or repeating the current grade.
  selectOnce(root, '[data-transition-row]').forEach((row) => {
    const outcome = row.querySelector('[data-transition-outcome]');
    const destination = row.querySelector('[data-transition-class]');
    if (!outcome || !destination) return;
    const update = () => {
      const grade = Number(row.dataset.grade);
      const targetGrade = outcome.value === 'repeat' ? grade :
        (outcome.value === 'promote' && grade < 9 ? grade + 1 : 0);
      Array.from(destination.options).forEach((option) => {
        if (!option.dataset.grade) return;
        const allowed = Number(option.dataset.grade) === targetGrade;
        option.hidden = !allowed;
        option.disabled = !allowed;
        if (!allowed && option.selected) destination.value = '';
      });
      if (!targetGrade) destination.value = '';
      destination.disabled = !targetGrade;
      destination.required = Boolean(targetGrade);
      row.querySelector('[data-transition-destination]').hidden = !targetGrade;
      row.querySelector('[data-transition-no-class]').hidden = Boolean(targetGrade);
    };
    outcome.addEventListener('change', update);
    update();
  });

  // The chosen candidate belongs to the class already selected on the placement page.
  selectOnce(root, '[data-placement-student-id]').forEach((button) => {
    button.addEventListener('click', () => {
      const modal = document.getElementById('new-placement-modal');
      if (!modal) return;
      modal.querySelector('[name="student_id"]').value = button.dataset.placementStudentId;
      modal.querySelector('[data-placement-student-label]').textContent = button.dataset.placementStudentName;
    });
  });

  // Selection spans every page; searching only changes visibility.
  selectOnce(root, '[data-placement-batch]').forEach(form => {
    const boxes = [...form.querySelectorAll('[name="placement_ids"]')];
    const count = form.querySelector('[data-batch-count]');
    const update = () => { count.textContent = boxes.filter(box => box.checked).length; };
    boxes.forEach(box => box.addEventListener('change', update));
    form.querySelectorAll('[data-batch-select]').forEach(button => {
      button.addEventListener('click', () => {
        boxes.forEach(box => { box.checked = button.dataset.batchSelect === 'all'; });
        update();
      });
    });
    update();
  });

  selectOnce(root, '[data-print-student-card]').forEach(button => {
    button.addEventListener('click', () => window.print());
  });

  // Use an explicit 24-hour field instead of the device's AM/PM time picker.
  selectOnce(root, '[data-time-24]').forEach(input => {
    const update = () => {
      if (/^[0-9]{3,4}$/.test(input.value)) {
        input.value = `${input.value.slice(0, 2)}:${input.value.slice(2)}`;
      }
      const valid = !input.value || /^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(input.value);
      input.setCustomValidity(valid ? '' : 'Gunakan format 24 jam HH:MM, misalnya 06:11 atau 18:11.');
    };
    input.addEventListener('input', update);
    update();
  });

  selectOnce(root, '[data-arrival-rule-form]').forEach(form => {
    const mode = form.querySelector('[data-arrival-rule-mode]');
    const update = () => {
      const dated = mode.value === 'date';
      for (const [selector, visible] of [['[data-arrival-weekly]', !dated], ['[data-arrival-date]', dated]]) {
        const panel = form.querySelector(selector);
        panel.hidden = !visible;
        panel.querySelectorAll('input,select').forEach(input => { input.disabled = !visible || input.hasAttribute('data-rule-scope'); });
      }
    };
    mode.addEventListener('change', update);
    update();
  });

  selectOnce(root, '[data-daily-required-arrival]').forEach(input => {
    const status = input.form.querySelector('[data-daily-arrival-status]');
    const update = () => { input.required = status.value === 'hadir'; input.disabled = status.value !== 'hadir'; };
    status.addEventListener('change', update);
    update();
  });

  selectOnce(root, '[data-daily-quick]').forEach(form => {
    const rows = [...form.querySelectorAll('[data-quick-row]')];
    const editable = rows.filter(row => !row.hasAttribute('data-quick-locked'));
    const summary = form.querySelector('[data-quick-summary]');
    let dirty = false;
    const statusOf = row => row.querySelector('[data-quick-status]:checked')?.value || '';
    const update = () => {
      const counts = {hadir: 0, sakit: 0, izin: 0, alpa: 0, empty: 0};
      let missing = 0;
      editable.forEach(row => {
        const status = statusOf(row);
        counts[status || 'empty']++;
        const arrival = row.querySelector('[data-quick-arrival]');
        arrival.required = status === 'hadir';
        arrival.disabled = status !== 'hadir';
        arrival.placeholder = status === 'hadir' ? '06:50' : 'Tidak diperlukan';
        if (arrival.required && !arrival.value) missing++;
      });
      summary.textContent = `${counts.hadir} Hadir · ${counts.sakit} Sakit · ${counts.izin} Izin · ${counts.alpa} Alpa · ${counts.empty} belum dipilih · ${rows.length - editable.length} sudah tersimpan${missing ? ` · ${missing} Hadir perlu jam datang` : ''}`;
    };
    form.addEventListener('input', event => { if (!event.target.matches('[data-quick-search]')) dirty = true; update(); });
    form.addEventListener('change', () => { dirty = true; update(); });
    form.querySelector('[data-quick-present]').addEventListener('click', () => {
      editable.filter(row => !statusOf(row)).forEach(row => { row.querySelector('[data-quick-status][value="hadir"]').checked = true; });
      dirty = true; update();
    });
    const common = form.querySelector('[data-quick-common-time]');
    form.querySelector('[data-quick-apply-time]').addEventListener('click', () => {
      if (!common.value || !common.reportValidity()) { common.focus(); return; }
      editable.filter(row => statusOf(row) === 'hadir').forEach(row => {
        const input = row.querySelector('[data-quick-arrival]');
        if (!input.value) { input.value = common.value; input.dispatchEvent(new Event('input', {bubbles: true})); }
      });
      dirty = true; update();
    });
    editable.forEach(row => row.querySelector('[data-quick-clear]').addEventListener('click', () => {
      row.querySelectorAll('[data-quick-status]').forEach(input => { input.checked = false; });
      const arrival = row.querySelector('[data-quick-arrival]'); arrival.value = ''; arrival.dispatchEvent(new Event('input', {bubbles: true}));
      dirty = true; update();
    }));
    const search = form.querySelector('[data-quick-search]');
    search.addEventListener('input', () => {
      const query = search.value.trim().toLocaleLowerCase();
      rows.forEach(row => { row.hidden = !row.textContent.toLocaleLowerCase().includes(query); });
    });
    const reason = form.querySelector('[name="reason_kind"]');
    const note = form.querySelector('[name="reason_note"]');
    const reasonState = () => { note.required = reason.value === 'other'; };
    reason.addEventListener('change', reasonState); reasonState();
    const warnUnsaved = event => { if (dirty && form.isConnected) { event.preventDefault(); event.returnValue = ''; } };
    window.addEventListener('beforeunload', warnUnsaved);
    registerPageCleanup(form, () => window.removeEventListener('beforeunload', warnUnsaved));
    form.addEventListener('submit', () => { dirty = false; });
    document.querySelectorAll('[data-quick-navigation]').forEach(nav => nav.addEventListener('submit', event => {
      if (dirty && !window.confirm('Ada isian absensi yang belum disimpan. Tinggalkan isian ini?')) { event.preventDefault(); nav.reset(); return; }
      dirty = false;
    }));
    dirty = editable.some(row => statusOf(row)) || Boolean(reason.value || note.value);
    if (dirty) form.dataset.listDirty = "true";
    update();
  });

}
initializePage();
document.body.addEventListener("htmx:afterSwap", event => initializePage(event.target));
