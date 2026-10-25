const attendanceForm = document.querySelector('[data-lesson-attendance-form]');

if (attendanceForm) {
  const statuses = Array.from(attendanceForm.querySelectorAll('[data-attendance-status]'));
  const count = attendanceForm.querySelector('[data-attendance-selected-count]');
  const save = attendanceForm.querySelector('[data-attendance-save]');
  const markAll = attendanceForm.querySelector('[data-attendance-mark-all]');
  let dirty = statuses.some((input) => input.value !== '');

  const update = () => {
    const selected = statuses.filter((input) => input.value !== '').length;
    if (count) count.textContent = String(selected);
    if (save) save.disabled = selected === 0;
  };

  statuses.forEach((input) => input.addEventListener('change', () => {
    dirty = true;
    update();
  }));

  markAll?.addEventListener('click', () => {
    statuses.forEach((input) => { input.value = 'hadir'; });
    dirty = true;
    update();
  });

  attendanceForm.addEventListener('submit', () => { dirty = false; });
  window.addEventListener('beforeunload', (event) => {
    if (dirty) {
      event.preventDefault();
      event.returnValue = '';
    }
  });
  update();
}
