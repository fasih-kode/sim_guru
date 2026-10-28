(() => {
  const root = document.querySelector('[data-teacher-attendance]');
  if (!root) return;
  const dialog = root.querySelector('[data-teacher-attendance-modal]');
  const form = root.querySelector('[data-attendance-form]');
  const video = root.querySelector('[data-attendance-video]');
  const canvas = root.querySelector('[data-attendance-preview]');
  const error = root.querySelector('[data-attendance-error]');
  const progress = root.querySelector('[data-attendance-progress]');
  const locationStatus = root.querySelector('[data-attendance-location-status]');
  const confirm = root.querySelector('[data-attendance-confirm]');
  const cameraButton = root.querySelector('[data-attendance-camera]');
  const locationButton = root.querySelector('[data-attendance-location]');
  const takeButton = root.querySelector('[data-attendance-take]');
  const retakeButton = root.querySelector('[data-attendance-retake]');
  let stream = null;
  let photo = null;
  let position = null;
  let action = null;
  let key = null;
  let submission = null;
  let busy = false;
  let cameraPending = false;
  let cameraAttempt = 0;
  let locationPending = false;
  let photoPending = false;
  let generation = 0;
  let requestController = null;

  function stopCamera() {
    cameraAttempt += 1;
    cameraPending = false;
    stream?.getTracks().forEach((track) => track.stop());
    stream = null;
    video.srcObject = null;
    video.hidden = true;
    takeButton.disabled = true;
  }
  function showError(message) {
    error.textContent = message;
    error.hidden = !message;
  }
  function controls() {
    const frozen = Boolean(submission);
    cameraButton.disabled = busy || frozen || cameraPending || photoPending;
    locationButton.disabled = busy || frozen || locationPending;
    retakeButton.disabled = busy || frozen || cameraPending || photoPending || !photo;
    takeButton.disabled = busy || frozen || !stream;
    confirm.disabled = busy || !photo || !position;
    root.querySelectorAll('[data-attendance-cancel]').forEach((button) => { button.disabled = busy; });
    confirm.textContent = busy ? 'Mengirim…' : submission ? 'Coba kirim lagi' : 'Konfirmasi';
  }
  function reset() {
    generation += 1;
    requestController?.abort();
    requestController = null;
    busy = false;
    stopCamera();
    photo = null;
    cameraPending = locationPending = photoPending = false;
    position = null;
    action = null;
    key = null;
    submission = null;
    canvas.width = 0;
    canvas.height = 0;
    canvas.hidden = true;
    showError('');
    locationStatus.textContent = 'Lokasi belum diambil.';
    progress.textContent = 'Selfie belum diambil.';
    controls();
  }
  function current(token) { return token === generation && dialog.open; }

  root.addEventListener('click', (event) => {
    const button = event.target.closest('[data-attendance-action]');
    if (!button || button.disabled || busy) return;
    reset();
    if (!window.isSecureContext || !window.crypto?.getRandomValues) {
      window.alert('GPS dan kamera memerlukan HTTPS atau localhost pada perangkat yang sama. Hubungi Petugas Absensi.');
      return;
    }
    action = button.dataset.attendanceAction;
    key = Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => byte.toString(16).padStart(2, '0')).join('');
    root.querySelector('[data-attendance-heading]').textContent = action === 'check_in' ? 'Catat Masuk' : 'Catat Pulang';
    controls();
    dialog.showModal();
    locationButton.focus();
  });

  locationButton.addEventListener('click', () => {
    if (busy || submission || locationPending) return;
    const token = generation;
    position = null;
    showError('');
    controls();
    if (!navigator.geolocation) { showError('Lokasi tidak tersedia. Hubungi Petugas Absensi.'); return; }
    locationPending = true;
    locationButton.disabled = true;
    locationStatus.textContent = 'Mengambil lokasi…';
    navigator.geolocation.getCurrentPosition((result) => {
      if (!current(token)) return;
      locationPending = false;
      const { latitude, longitude, accuracy } = result.coords;
      if (![latitude, longitude, accuracy].every(Number.isFinite) || accuracy < 0 || accuracy > 50) {
        locationStatus.textContent = 'Lokasi belum cukup akurat.';
        showError('Coba di tempat dengan sinyal lebih baik atau minta bantuan Petugas Absensi.');
      } else {
        position = { latitude, longitude, accuracy };
        locationStatus.textContent = `Lokasi siap · akurasi ${Math.ceil(accuracy)} meter.`;
      }
      controls();
    }, () => {
      if (!current(token)) return;
      stopCamera();
      locationPending = false;
      locationStatus.textContent = 'Lokasi gagal diambil.';
      showError('Izin lokasi ditolak atau sinyal belum tersedia. Coba kembali atau hubungi Petugas Absensi.');
      controls();
    }, { enableHighAccuracy: true, maximumAge: 0, timeout: 15000 });
  });

  async function startCamera() {
    if (busy || submission || cameraPending || photoPending) return;
    const token = generation;
    stopCamera();
    const attempt = cameraAttempt;
    photo = null;
    canvas.hidden = true;
    showError('');
    controls();
    if (!navigator.mediaDevices?.getUserMedia) { showError('Kamera tidak tersedia. Gunakan HTTPS atau localhost, atau hubungi Petugas Absensi.'); return; }
    cameraPending = true;
    cameraButton.disabled = true;
    progress.textContent = 'Membuka kamera…';
    try {
      const opened = await navigator.mediaDevices.getUserMedia({ audio: false, video: { facingMode: 'user' } });
      if (!current(token) || attempt !== cameraAttempt) { opened.getTracks().forEach((track) => track.stop()); return; }
      stream = opened;
      video.srcObject = stream;
      video.hidden = false;
      await video.play();
      if (!current(token) || attempt !== cameraAttempt) { opened.getTracks().forEach((track) => track.stop()); return; }
      cameraPending = false;
      progress.textContent = 'Kamera siap. Tekan Ambil Foto.';
      controls();
    } catch (_) {
      if (!current(token) || attempt !== cameraAttempt) return;
      stopCamera();
      cameraPending = false;
      progress.textContent = 'Kamera gagal dibuka.';
      showError('Izin kamera ditolak atau kamera sedang digunakan. Coba kembali atau hubungi Petugas Absensi.');
      controls();
    }
  }
  cameraButton.addEventListener('click', startCamera);
  retakeButton.addEventListener('click', startCamera);

  takeButton.addEventListener('click', async () => {
    if (!stream || busy || submission || !video.videoWidth) return;
    const token = generation;
    const scale = Math.min(1, 640 / Math.max(video.videoWidth, video.videoHeight));
    canvas.width = Math.round(video.videoWidth * scale);
    canvas.height = Math.round(video.videoHeight * scale);
    canvas.getContext('2d').drawImage(video, 0, 0, canvas.width, canvas.height);
    stopCamera();
    photoPending = true;
    controls();
    try {
      let captured = null;
      for (const quality of [0.85, 0.7, 0.55, 0.4, 0.25]) {
        captured = await new Promise((resolve) => canvas.toBlob(resolve, 'image/jpeg', quality));
        if (!current(token)) return;
        if (captured && captured.size <= 100 * 1024) break;
      }
      if (!captured || captured.size > 100 * 1024) throw new Error('photo-too-large');
      photo = captured;
      canvas.hidden = false;
      progress.textContent = 'Selfie siap. Periksa foto dan lokasi sebelum Konfirmasi.';
    } catch (_) { if (current(token)) showError('Foto belum dapat diproses. Buka kamera dan ambil ulang foto.'); }
    if (current(token)) { photoPending = false; controls(); }
  });

  function refreshStatus(document) {
    const context = document.querySelector('[data-teacher-attendance-context]');
    if (context) root.querySelector('[data-teacher-attendance-context]').replaceWith(context);
    const updated = document.querySelector('[data-teacher-attendance-status]');
    if (updated) root.querySelector('[data-teacher-attendance-status]').replaceWith(updated);
    return Boolean(updated);
  }
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    if (busy || !photo || !position || !key) return;
    // Bukti dan token dibekukan setelah pengiriman pertama, termasuk ketika respons hilang.
    submission ||= { photo, position: { ...position }, action, key };
    const data = new FormData();
    data.set('action', submission.action);
    data.set('idempotency_key', submission.key);
    data.set('latitude', String(submission.position.latitude));
    data.set('longitude', String(submission.position.longitude));
    data.set('accuracy_m', String(submission.position.accuracy));
    data.set('selfie', submission.photo, 'selfie.jpg');
    busy = true;
    stopCamera();
    showError('');
    progress.textContent = 'Mengirim kehadiran…';
    controls();
    const token = generation;
    const controller = new AbortController();
    requestController = controller;
    const timeout = setTimeout(() => controller.abort(), 25000);
    try {
      const response = await fetch(form.action, {
        method: 'POST', body: data, credentials: 'same-origin', signal: controller.signal,
        headers: { 'X-CSRF-Token': form.querySelector('[name="gorilla.csrf.Token"]').value },
      });
      const body = await response.text();
      if (!current(token)) return;
      const page = new DOMParser().parseFromString(body, 'text/html');
      const next = page.querySelector('[data-teacher-attendance]');
      if (!next) {
        showError('Sesi atau formulir tidak berlaku. Muat ulang halaman dan periksa status, atau hubungi Petugas Absensi.');
      } else if (response.ok && response.redirected) {
        refreshStatus(page);
        const result = root.querySelector('[data-attendance-result]');
        result.textContent = next.dataset.responseMessage || 'Kehadiran berhasil dicatat.';
        result.hidden = false;
        busy = false;
        dialog.close();
        root.querySelector('[data-teacher-attendance-status]').focus?.();
      } else {
        refreshStatus(page);
        showError(next.dataset.responseError || 'Pencatatan belum berhasil. Coba kirim ulang atau hubungi Petugas Absensi.');
        if (response.status === 409 || response.status === 422 || response.status === 413) {
          progress.textContent = 'Periksa status terbaru. Tutup dialog untuk memulai tindakan baru.';
          // Respons definitif ditolak; retry bukti ini tidak diperlukan.
          photo = null;
        }
      }
    } catch (_) {
      if (current(token)) showError('Respons belum diterima. Coba kirim lagi dengan foto dan lokasi yang sama, atau hubungi Petugas Absensi.');
    } finally {
      clearTimeout(timeout);
      if (token === generation) {
        busy = false;
        requestController = null;
        stopCamera();
        controls();
      }
    }
  });
  root.querySelectorAll('[data-attendance-cancel]').forEach((button) => button.addEventListener('click', () => { if (!busy) dialog.close(); }));
  dialog.addEventListener('cancel', (event) => { if (busy) event.preventDefault(); });
  dialog.addEventListener('close', reset);
  window.addEventListener('pagehide', () => { reset(); if (dialog.open) dialog.close(); });
  document.addEventListener('visibilitychange', () => { if (document.hidden) { stopCamera(); controls(); } });
})();
