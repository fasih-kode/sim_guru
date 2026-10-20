htmx.config.allowEval = false;
htmx.config.includeIndicatorStyles = false;
htmx.config.selfRequestsOnly = true;
htmx.config.historyCacheSize = 0;
for (const event of ['htmx:responseError', 'htmx:sendError', 'htmx:timeout']) {
  document.body.addEventListener(event, () => {
    document.getElementById('request-error').hidden = false;
  });
}
document.body.addEventListener('htmx:beforeRequest', () => {
  document.getElementById('request-error').hidden = true;
});
