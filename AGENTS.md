# Instruksi permanen SIM-GURU

## Awal sesi
1. Periksa `git status --short` dan perubahan pengguna; jangan menimpa perubahan yang belum di-commit.
2. Baca dokumen ini, [HANDOVER](docs/HANDOVER.md), [indeks PRD](docs/PRD.md), dan dokumentasi relevan.
3. Periksa implementasi aktual. Dokumentasi tidak otomatis benar ketika bertentangan dengan kode; laporkan perbedaan dan minta keputusan untuk konflik kebijakan.
4. Jelaskan rencana serta dampak sebelum perubahan besar. Gunakan bahasa Indonesia untuk laporan.

## Implementasi
- Ikuti arsitektur, stack, dan konvensi yang berlaku. Fondasi aplikasi tersedia; baca ARCHITECTURE dan kode aktual sebelum menambah modul atau API.
- **Standar FE SIM-GURU: TailAdmin Free adalah acuan visual default untuk seluruh halaman, semua role, dan komponen baru.** Sebelum membuat/mengubah FE, buka demo TailAdmin dan periksa halaman/komponen paling dekat serta source repository; jangan menunggu pengguna mengirim screenshot atau mengulang referensi. Cocokkan layout, lebar konten, jarak, tipografi, warna, border/radius, ikon, state, responsivitas, dan interaksi. Contoh: permintaan form di modal mengikuti komponen Form in Modal pada `/modals`; tabel mengikuti `/basic-tables`. Gunakan komponen/varian terdekat bila permintaan tidak menyebut detail.
- Adaptasikan pola TailAdmin ke stack SSR/htmx dan pola akses SIM-GURU; pertahankan data, business rules, otorisasi, aksesibilitas, dan konvensi proyek. Jangan mengganti stack atau menyalin seluruh template aplikasi tanpa kebutuhan. Verifikasi hasil FE secara visual pada desktop dan ponsel, bandingkan dengan referensi; jangan menyatakan cocok bila belum diperiksa.
- Jangan meminta klarifikasi visual yang sudah dapat dijawab dengan membuka referensi. Jika referensi tidak dapat diakses, cari source resmi. Tanyakan hanya bila ada beberapa varian yang materially berbeda dan pilihan memengaruhi alur/aturan produk; jika tidak, pilih varian TailAdmin yang paling dekat dan lanjutkan.
- Admin mengelola identitas siswa/guru, akun, role, pembuatan/koreksi kelas dan penempatan awal siswa. Kurikulum menyiapkan/mengaktifkan tahun ajaran beserta dua semester, mengelola mapel, wali kelas, pengampu, jadwal, transisi kenaikan/kelulusan dan pemeriksaan koreksi nilai Final. Kurikulum membaca kelas tanpa hak membuat/mengoreksi kelas; Admin tidak otomatis mendapat hak kalender/penugasan/nilai. Lihat PRD bab 3A dan keputusan D-015. Jangan mengubah aturan bisnis tanpa persetujuan.
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
