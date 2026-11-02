import QrScanner from './qr-scanner.min.js';

const root = document.querySelector('[data-qr-reader]');
if (root) {
  const form = root.querySelector('[data-qr-form]');
  const input = form.querySelector('[name="token"]');
  const output = root.querySelector('[data-qr-result]');
  const cameraStatus = root.querySelector('[data-qr-camera-status]');
  const next = root.querySelector('[data-qr-next]');
  const video = root.querySelector('video');
  const start = root.querySelector('[data-qr-start]');
  let busy = false;
  let awaitingNext = false;
  let scanner;
  let cameraRunning = false;

  const show = (message, error = false) => {
    output.textContent = message;
    output.classList.toggle('qr-result--error', error);
  };
  const submit = async token => {
    if (busy || awaitingNext) return;
    busy = true;
    awaitingNext = true;
    scanner?.pause();
    form.querySelector('button[type="submit"]').disabled = true;
    show('Pencatatan sedang dikirim…');
    try {
      const data = new FormData(form);
      data.set('token', token);
      const response = await fetch(form.action, {method: 'POST', body: new URLSearchParams(data), credentials: 'same-origin', headers: {'Accept': 'application/json'}});
      if (!response.headers.get('Content-Type')?.includes('application/json')) {
        throw new Error('Sesi berakhir atau permintaan ditolak. Muat ulang dan login kembali bila diperlukan.');
      }
      const result = await response.json();
      if (!response.ok || !result.ok) throw new Error(result.message || 'Pencatatan gagal. Periksa absensi harian.');
      show(`${result.name} · ${result.class} · ${result.date} · ${result.arrival} WIB · ${result.punctuality || ""}. ${result.message}`);
      input.value = '';
    } catch (error) {
      show(error instanceof Error ? error.message : 'Jaringan bermasalah. Periksa absensi harian sebelum mencoba kembali.', true);
    } finally {
      busy = false;
      next.hidden = false;
    }
  };
  form.addEventListener('submit', event => {event.preventDefault(); submit(input.value.trim());});
  next.addEventListener('click', async () => {
    awaitingNext = false;
    next.hidden = true;
    form.querySelector('button[type="submit"]').disabled = false;
    show('Siap memindai siswa berikutnya.');
    if (cameraRunning) {
      try { await scanner.start(); } catch { cameraStatus.textContent = 'Kamera tidak dapat dilanjutkan. Gunakan Mulai kamera atau pencatatan manual.'; }
    }
    input.focus();
  });
  start.addEventListener('click', async () => {
    if (awaitingNext) { next.focus(); return; }
    try {
      if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) throw new Error('Kamera memerlukan HTTPS atau localhost. Gunakan pembaca USB atau absensi manual jika kamera tidak tersedia.');
      if (!scanner) scanner = new QrScanner(video, result => submit(result.data), {preferredCamera: 'environment', maxScansPerSecond: 5, returnDetailedScanResult: true});
      await scanner.start();
      cameraRunning = true;
      cameraStatus.textContent = 'Kamera aktif. Arahkan satu kartu ke kamera.';
    } catch (error) {
      cameraStatus.textContent = error instanceof Error ? error.message : 'Kamera gagal dibuka atau izinnya ditolak. Gunakan pembaca USB atau catat manual dengan alasan.';
    }
  });
  root.querySelector('[data-qr-stop]').addEventListener('click', () => {scanner?.stop(); cameraRunning = false; cameraStatus.textContent = 'Kamera dihentikan.';});
  window.addEventListener('pagehide', () => scanner?.destroy());
}
