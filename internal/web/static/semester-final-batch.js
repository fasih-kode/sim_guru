document.addEventListener('DOMContentLoaded', () => {
  const form = document.querySelector('[data-final-batch-selection]');
  if (!form) return;
  const button = form.querySelector('[data-final-batch-preview]');
  const count = form.querySelector('[data-final-batch-count]');
  const update = () => {
    const selected = form.querySelectorAll('input[name="student_id"]:checked').length;
    count.textContent = String(selected);
    button.disabled = selected === 0;
  };
  form.addEventListener('change', update);
  update();
});
