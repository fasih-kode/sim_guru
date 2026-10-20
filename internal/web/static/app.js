htmx.config.allowEval = false;
htmx.config.includeIndicatorStyles = false;
htmx.config.selfRequestsOnly = true;
htmx.config.historyCacheSize = 0;
for (const event of ['htmx:responseError', 'htmx:sendError', 'htmx:timeout']) {
  document.body.addEventListener(event, () => {
    const message = document.getElementById('request-error');
    if (message) message.hidden = false;
  });
}
document.body.addEventListener('htmx:beforeRequest', () => {
  const message = document.getElementById('request-error');
  if (message) message.hidden = true;
});
