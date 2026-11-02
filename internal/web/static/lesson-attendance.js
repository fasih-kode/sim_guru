function initializeLessonAttendance(root = document) {
  const attendanceForm = root.querySelector('[data-lesson-attendance-form]');

  if (attendanceForm && !attendanceForm.dataset.lessonInitialized) {
    attendanceForm.dataset.lessonInitialized = "true";
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
    const warnUnsaved = (event) => {
      if (dirty && attendanceForm.isConnected) {
        event.preventDefault();
        event.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', warnUnsaved);
    registerPageCleanup(attendanceForm, () => window.removeEventListener('beforeunload', warnUnsaved));
    if (dirty) attendanceForm.dataset.listDirty = "true";
    update();
  }

}
initializeLessonAttendance();
document.body.addEventListener("htmx:afterSwap", event => initializeLessonAttendance(event.target));
