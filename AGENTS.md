# Instruksi permanen SIM-GURU

## Awal sesi
1. Periksa `git status --short` dan perubahan pengguna; jangan menimpa perubahan yang belum di-commit.
2. Baca dokumen ini, [HANDOVER](docs/HANDOVER.md), [indeks PRD](docs/PRD.md), dan dokumentasi relevan.
3. Periksa implementasi aktual. Dokumentasi tidak otomatis benar ketika bertentangan dengan kode; laporkan perbedaan dan minta keputusan untuk konflik kebijakan.
4. Jelaskan rencana serta dampak sebelum perubahan besar. Gunakan bahasa Indonesia untuk laporan.

## Implementasi
- Ikuti arsitektur, stack, dan konvensi yang berlaku. Fondasi aplikasi tersedia; baca ARCHITECTURE dan kode aktual sebelum menambah modul atau API.
- Admin mengelola identitas siswa/guru, akun, role; Kurikulum mengelola penugasan akademik. Jangan mengubah aturan bisnis tanpa persetujuan.
- Utamakan perubahan kecil, terfokus, mudah diuji; clean code, pemisahan tanggung jawab, dan penanganan error eksplisit.
- Jangan melakukan refactoring di luar cakupan tanpa alasan dan persetujuan. Perubahan arsitektur besar memerlukan persetujuan.
- Tambahkan/perbarui pengujian untuk perubahan perilaku; jalankan pengujian dan pemeriksaan statis relevan. Jangan mengklaim lulus tanpa menjalankannya.
- Jangan mengubah migration yang sudah diterapkan tanpa strategi aman. Gunakan transaksi untuk operasi atomik dan database pengujian terpisah.
- Jangan menghapus data, reset database, atau menjalankan tindakan destruktif tanpa persetujuan eksplisit dan strategi pemulihan.
- Jangan menyimpan password, token, key, atau rahasia ke dokumentasi/Git/log. Otorisasi, validasi, dan CSRF harus di server.
- Jangan commit atau push tanpa instruksi eksplisit. Jangan membatalkan pekerjaan pengguna.
- P0 OPEN hanya menahan fitur terdampak. Jangan menebak rumus nilai atau menambah guru pengganti, mandat, dan approval rutin.

## Akhir sesi
1. Tinjau diff dan file baru; periksa rahasia serta artefak yang tidak diperlukan.
2. Jalankan verifikasi relevan; laporkan yang berhasil, gagal, atau tidak dijalankan beserta alasannya.
3. Perbarui dokumentasi terdampak dan [HANDOVER](docs/HANDOVER.md) sebelum mengakhiri pekerjaan.
4. Catat perubahan, bukti verifikasi, risiko, pekerjaan wajib tersisa, dan langkah berikutnya.

## Tata kelola sumber
[PRD](docs/PRD.md) menunjuk dokumen kebutuhan otoritatif. [DECISIONS](docs/DECISIONS.md) mencatat keputusan; jangan menggandakan kebutuhan lengkap. Bedakan fakta terverifikasi, keputusan disetujui, rekomendasi belum disetujui, dan pekerjaan belum selesai. Jika sumber bertentangan, laporkan dan minta keputusan sebelum mengganti kebijakan; lanjutkan pekerjaan independen yang aman.
