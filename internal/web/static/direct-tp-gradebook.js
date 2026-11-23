(() => {
  let dirty = Boolean(document.querySelector('[data-gradebook-form][data-draft]'));
  document.addEventListener('input', (event) => {
    if (event.target.closest('[data-gradebook-form]')) dirty = true;
  });
  document.addEventListener('submit', (event) => {
    if (event.target.matches('[data-gradebook-form]')) dirty = false;
  });
  document.addEventListener('htmx:afterSwap', (event) => {
    if (event.target.id === 'content') dirty = Boolean(document.querySelector('[data-gradebook-form][data-draft]'));
  });
  window.addEventListener('beforeunload', (event) => {
    if (!dirty) return;
    event.preventDefault();
    event.returnValue = '';
  });
})();
