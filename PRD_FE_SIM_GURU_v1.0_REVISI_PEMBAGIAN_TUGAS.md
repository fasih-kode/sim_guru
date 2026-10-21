# PRD Frontend SIM-GURU v1.0

**Status:** Panduan implementasi UI, selaras PRD utama (revisi kewenangan sederhana).\
**Rujukan utama:** `PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md`.

## 0. Stack dan aturan UI

- Ubuntu 26.04 LTS untuk pengembangan; Go `html/template` merender layout dan fragmen; htmx menangani pembaruan sebagian halaman.
- TailAdmin Free HTML + Tailwind CSS sebagai basis visual: https://github.com/TailAdmin/tailadmin-free-tailwind-dashboard-template
- Adaptasi sidebar, header, card, tabel, formulir, modal, dan state dari TailAdmin; Alpine.js boleh untuk interaksi visual lokal. Jangan membangun SPA React/Next/Vue.
- Build aset lokal. Periksa lisensi, README, dan versi template sebelum integrasi; jangan membawa semua demo yang tidak digunakan.
- Validasi, CSRF, sesi, dan otorisasi tetap di backend Go. Sembunyi menu bukan pengganti pembatasan route.
- **Tidak ada menu Mandat, Delegasi, Kotak Persetujuan, atau workflow approval rutin.**

## 1. Prinsip pengalaman pengguna

1. Guru membuka aplikasi dan langsung melihat tugas sesuai role yang diatur Admin dan penugasan yang diatur Kurikulum.
2. Tindakan rutin diselesaikan dengan satu formulir dan konfirmasi yang wajar; hindari alur pengajuan/approval berulang.
3. Tampilkan state **memuat, kosong, berhasil, gagal, tidak berhak, data belum lengkap**; tampilkan alasan validasi dekat field.
4. Pada operasi sensitif gunakan konfirmasi tindakan, alasan koreksi, dan riwayat, **bukan persetujuan digital Kepala Sekolah**.
5. UI responsif untuk laptop/desktop dan ponsel, akses keyboard, label input jelas, kontras terbaca, serta feedback htmx.

## 2. Struktur sidebar menurut role

| Menu utama | Submenu | Siapa melihat |
|---|---|---|
| Dashboard | Ringkasan, tugas hari ini, perhatian | Semua pengguna sesuai role |
| Administrasi | Data Siswa, Data Guru, Kelas, Penempatan Awal, Pengguna, Role, Riwayat Perubahan | Admin |
| Data Induk | Siswa, Guru (baca sesuai cakupan); Mata Pelajaran (kelola Kurikulum) | Kurikulum dan role lain sesuai kebutuhan |
| Akademik | Tahun Ajaran, Semester, Kelas/Penempatan (baca-saja), Wali Kelas, Penugasan Mengajar, Transisi | Kurikulum |
| Jadwal | Jam Pelajaran, Jadwal, Riwayat Perubahan | Kurikulum; Guru melihat Jadwal Saya |
| Absensi | Guru QR, Siswa Harian, Absensi Pelajaran, Koreksi | Sesuai role/penugasan |
| Jurnal | Jurnal Saya, Riwayat Koreksi | Guru Mapel; pengawasan sesuai role |
| Penilaian | TP, Komponen, Nilai, Remedial, Nilai Akhir, Koreksi Final | Guru Mapel dan Kurikulum sesuai hak |
| Laporan | Akademik, Kehadiran, Kelengkapan | Kepala Sekolah, Kurikulum, Guru/Wali sesuai lingkup |
| Pengaturan | Profil, Kata Sandi, Konfigurasi teknis | Sesuai role |

**Tidak ada menu** Persetujuan dan Delegasi, Inbox Approval, Permohonan Mandat, atau Tugas Menunggu Kepala Sekolah.

## 2A. Pemisahan layar data induk dan operasional — FINAL

- **Admin:** CRUD identitas siswa dan guru, kelengkapan data, pencarian/duplikat, akun, role, pembuatan/koreksi kelas dan penempatan awal siswa. Hanya Admin melihat tombol Tambah/Edit Identitas siswa/guru dan Tambah/Edit Kelas.
- **Kurikulum:** siswa/guru tampil **baca-saja** bersama kelas/penempatan awal sebagai sumber pilihan ketika mengatur wali kelas, guru pengampu, dan jadwal. Kurikulum menyiapkan tahun ajaran beserta dua semester dan mengaktifkannya; mengelola mapel, transisi kenaikan/kelulusan serta pemeriksaan koreksi nilai Final. Tombol Simpan Penugasan di halaman Kurikulum langsung berlaku setelah validasi.
- **Guru Mapel/Wali/BK/Kesiswaan/Kepala Sekolah:** tampilan data siswa/guru sesuai cakupan dan kebutuhan; tidak ada tombol edit identitas.
- Jika data induk keliru, tampilkan petunjuk **Hubungi Admin** tanpa membuat alur mandat/delegasi/persetujuan digital.
- Validasi hak edit di backend; menghilangkan tombol tidak cukup.

## 3. Halaman wajib dan interaksi

### 3.1 Login dan profil
- Login personel, keluar, ganti kata sandi, penanganan sesi kedaluwarsa. Tidak ada login siswa/orang tua.
- Dashboard setelah login disusun dari role dan penugasan aktif.

### 3.2 Admin: Data Siswa, Data Guru, Kelas, Penempatan Awal, Pengguna dan Role
- Daftar pengguna: cari/filter role/status, tambah, edit, aktif/nonaktif.
- Form pengguna: identitas, username/kredensial, pilihan multi-role sesuai kebutuhan.
- Form data induk siswa/guru: tambah, ubah, status aktif/nonaktif, pemeriksaan duplikat, dan riwayat koreksi identitas.
- Form role: tetapkan role tanpa menyisipkan pengaturan kelas, guru pengampu, atau wali kelas.
- Halaman Kelas terpisah: Admin membuat/mengoreksi kelas dan melihat histori; tahun dipilih dari kalender yang disiapkan Kurikulum. Jika belum ada tahun, arahkan untuk menghubungi Kurikulum.
- Penempatan awal siswa menjadi halaman Admin; jangan menampilkan kontrol mutasinya kepada Kurikulum.
- Penugasan wali/pengampu dan jadwal tetap di halaman Kurikulum, terpisah dari pengelolaan kelas/penempatan awal oleh Admin.
- Tampilkan ringkasan hak yang akan diperoleh dan histori perubahan role/penugasan.
- **Jangan** menampilkan tombol “Ajukan ke Kepala Sekolah”, “Minta Mandat”, atau “Tunggu Persetujuan”.

### 3.3 Kurikulum: Akademik
- Tahun/semester: siapkan tahun beserta dua semester, daftar, status, aktifkan, tutup dengan pemeriksaan kewajiban; cegah status aktif ganda.
- Kelas/penempatan awal: baca-saja, daftar tingkat, nama, wali kelas, kesiapan dan siswa dengan pencarian/filter; hubungi Admin untuk pembuatan/koreksi kelas dan penempatan awal. Akun multi-role Admin+Kurikulum tetap dapat mengelola kelas melalui kewenangan Admin.
- Perpindahan tengah tahun: form siswa, kelas asal/tujuan, tanggal efektif, alasan, **catatan keputusan Kepala Sekolah** (tanggal/rujukan); simpan setelah validasi, bukan approval digital.
- Transisi tahun: pratinjau daftar naik/tidak naik/lulus/keluar, konflik, konfirmasi eksekusi; tanpa inbox persetujuan digital.

### 3.4 Jadwal dan penugasan mengajar
- Grid jadwal per kelas/guru; draf, cek konflik, publikasi, histori versi.
- Perubahan jadwal: ubah hari/jam atau penugasan **pengampu resmi** sesuai kewenangan, dengan tanggal efektif dan alasan; tampilkan riwayat sebelum/sesudah. **Tidak ada** pilihan Guru Pengganti/Cadangan, penunjukan untuk satu pertemuan, atau tombol alihkan ke guru lain.
- Jika guru berhalangan, jadwal tetap tercatat tanpa membuat jurnal mengajar otomatis. Tampilkan kondisi belum ada jurnal tanpa menyimpulkan pembelajaran terlaksana.
- Guru memiliki tampilan **Jadwal Saya**, hanya penugasan aktif.

### 3.5 Absensi
- Guru: tombol/QR masuk dan pulang, status tercatat, pesan gagal/fallback sesuai aturan yang disetujui.
- Siswa: absensi harian dan pelajaran di layar terpisah; status Hadir/Sakit/Izin/Alpa/Belum Dicatat.
- Koreksi: tampilkan nilai sebelumnya, nilai baru, alasan, simpan, riwayat. Tidak perlu approval Kepala Sekolah.

### 3.6 Jurnal
- Guru pengampu resmi memilih jadwal/kelas, mengisi waktu/blok, materi, catatan, Simpan Draf/Finalkan. Identitas guru berasal dari akun/penugasan resmi; tidak ada dropdown Guru Pengganti atau Guru Cadangan.
- Setelah Final, tombol **Ajukan Koreksi** dapat diganti menjadi **Koreksi dengan Alasan** untuk guru berwenang; versi sebelumnya tetap dapat dilihat.
- Jika data tidak sesuai jadwal, tampilkan indikator anomali tanpa mengubah catatan otomatis.

### 3.7 Penilaian
- Halaman TP, komponen, input nilai per siswa, remedial, rekomendasi (hanya jika rumus tersedia), nilai akhir dan finalisasi.
- Nilai Final ditampilkan read-only; tombol **Koreksi Nilai Final** membuka alasan dan proses pemeriksaan Kurikulum yang sederhana, bukan approval Kepala Sekolah.
- Tampilkan perbedaan rekomendasi dan nilai akhir serta alasannya. Jika rumus belum disetujui, jangan tampilkan angka rekomendasi palsu.

### 3.8 Dashboard Kepala Sekolah dan laporan
- Ringkasan guru/siswa, kelengkapan jurnal, kehadiran, status penilaian, dan laporan akademik sesuai data yang tersedia.
- Tidak ada angka “permohonan menunggu persetujuan” untuk pekerjaan rutin.
- Laporan menandai Final/Sementara/Belum Lengkap dan bukan rapor resmi.

## 4. Pola teknis Go Template + htmx

- GET navigasi biasa merender layout lengkap TailAdmin; request htmx merender fragmen HTML yang relevan.
- Gunakan respons server untuk filter/pagination/pencarian/form, dengan indikator loading dan area pesan kesalahan.
- Tombol simpan memicu validasi server, perubahan data, audit, lalu update fragmen/redirect.
- Perubahan data harus dilindungi CSRF; jangan bergantung pada pengecekan role di JavaScript.
- Asset TailAdmin dibangun lokal; tetapkan versi template dan catat perubahan komponen.

## 5. Acceptance criteria FE

- Semua layar dan aksi harus bebas dari menu, badge, pilihan guru, form, dan izin khusus Guru Pengganti/Cadangan.
- Perubahan pengampu resmi hanya melalui alur penugasan Kurikulum, bukan penggantian satu kali pada halaman Jadwal.

| ID | Skenario | Hasil |
|---|---|---|
| FE-01 | Admin memberi role Kurikulum | Menu Kurikulum langsung muncul saat sesi/hak diperbarui |
| FE-02 | Kurikulum menetapkan wali kelas | Wali melihat kelasnya tanpa approval |
| FE-03 | Guru tanpa penugasan membuka URL nilai | Server menolak meskipun URL diketahui |
| FE-04 | Kurikulum menyusun jadwal | Benturan terlihat dan publikasi ditolak |
| FE-05 | Guru mengisi jurnal Final | Koreksi butuh alasan, histori terlihat, tanpa inbox approval |
| FE-06 | Koreksi nilai Final | Guru dan Kurikulum melihat status pemeriksaan sederhana; Kepala Sekolah tidak menjadi approver rutin |
| FE-07 | Perpindahan tengah tahun | Form mencatat keputusan Kepala Sekolah, tidak ada approval workflow digital |
| FE-08 | Laporan belum lengkap | Status jelas dan tidak diklaim Final |
| FE-09 | Akses mobile | Sidebar, tabel, formulir dapat digunakan |
| FE-11 | Guru berhalangan hadir | Tidak muncul tombol tunjuk pengganti; jadwal tidak otomatis membuat jurnal |
| FE-12 | Pengampu resmi berganti | Penugasan Kurikulum dan jadwal efektif baru terlihat; riwayat lama tetap tersedia |
| FE-10 | htmx gagal | Ada pesan kesalahan dan pengguna tidak kehilangan input tanpa peringatan |

## 6. Batasan keputusan UI

Nama menu dan layout detail adalah **RANCANGAN**; aturan peran/penugasan sederhana adalah **FINAL**. Jangan menambah approval digital hanya karena tersedia komponen tombol/modal pada TailAdmin. Detail rumus nilai, katalog laporan, dan fallback QR mengikuti OPEN di PRD utama.
