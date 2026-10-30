# PRD Frontend SIM-GURU v1.0

**Status:** Panduan implementasi UI, selaras PRD utama (revisi kewenangan sederhana).\
**Rujukan utama:** `PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md`.

## 0. Stack dan aturan UI

**Navigasi global — D-047 (FINAL):** semua kelompok dengan beberapa halaman memakai horizontal navigation links bergaya tabs TailAdmin Free, dengan ikon, teks/underline biru aktif, satu tab aktif, link native dan `aria-current="page"`. Sidebar hanya menampilkan kelompok utama; parent menuju halaman awal yang diizinkan dan tetap aktif pada halaman anak. Role/permission memfilter kelompok dan tab tanpa mengganti otorisasi server atau menggandakan navigasi pada role gabungan. Pada ponsel bar dapat digeser horizontal tanpa overflow halaman. Gunakan implementasi bersama di `internal/web/section_navigation.go`, partial `section-navigation-tabs`, dan stylesheet `.navigation-*`; jangan membuat sistem tab baru. Halaman tunggal tidak perlu tab, sedangkan form/dialog/wizard bukan tab navigasi. Kelompok panjang dibagi secara logis tanpa tab bertingkat. Rincian implementasi dan aksesibilitas: [UI Standards](docs/UI_STANDARDS.md).

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
| Kelas Saya | Daftar kelas tahun aktif yang ditugaskan dan roster siswa per kelas | Wali Kelas; dibatasi role dan penugasan server |
| Jadwal | Jam Pelajaran, Jadwal, Riwayat Perubahan | Kurikulum; Guru melihat Jadwal Saya |
| Absensi | Guru GPS/Selfie, Siswa Harian QR, Absensi Pelajaran, Koreksi | Sesuai role/penugasan |
| Kesiswaan | Siswa/Rekap Kehadiran, Batas Kedatangan, Catatan Pembinaan | Kesiswaan seluruh sekolah; Wali Kelas/Kepala Sekolah membaca catatan sesuai cakupan PRD bab 3B |
| BK | Siswa yang Ditangani, Catatan Bimbingan | BK sesuai siswa/kasus; Wali Kelas/Kepala Sekolah membaca catatan sesuai cakupan PRD bab 3B |
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
- Kurikulum mendapat menu **Transisi Siswa** untuk memilih tahun sumber/tujuan, melihat pratinjau setiap siswa, menetapkan hasil dan kelas tujuan, lalu mengonfirmasi batch. Kelas tahun tujuan dibuat Admin lebih dahulu. Jangan tampilkan tindakan transfer tengah tahun pada halaman ini; alur itu tersedia terpisah melalui Aksi pada daftar Penempatan (D-031).
- Penugasan wali/pengampu dan jadwal tetap di halaman Kurikulum, terpisah dari pengelolaan kelas/penempatan awal oleh Admin.
- Tampilkan ringkasan hak yang akan diperoleh dan histori perubahan role/penugasan.
- **Jangan** menampilkan tombol “Ajukan ke Kepala Sekolah”, “Minta Mandat”, atau “Tunggu Persetujuan”.

### 3.3 Kurikulum: Akademik
- Tahun/semester: siapkan tahun beserta dua semester, daftar, status, aktifkan, tutup dengan pemeriksaan kewajiban; cegah status aktif ganda.
- Kelas/penempatan awal: baca-saja, daftar tingkat, nama, wali kelas, kesiapan dan siswa dengan pencarian/filter; hubungi Admin untuk pembuatan/koreksi kelas dan penempatan awal. Akun multi-role Admin+Kurikulum tetap dapat mengelola kelas melalui kewenangan Admin.
- Perpindahan tengah tahun: form siswa, kelas asal/tujuan, tanggal efektif, alasan, **catatan keputusan Kepala Sekolah** (tanggal/rujukan); simpan setelah validasi, bukan approval digital.
- Transisi tahun: pratinjau daftar naik/tidak naik/lulus/keluar, konflik, konfirmasi eksekusi; tanpa inbox persetujuan digital.

### 3.4 Jadwal dan penugasan mengajar
- Jam Pelajaran menyediakan pilihan hari Senin–Sabtu dan 12 baris bernomor siap pakai pada setiap hari. Pengisian waktu memakai Form in Modal terpusat dengan backdrop blur dan tombol “Simpan jam hari ini”. Semua baris input tetap dalam satu form; daftar ringkasan memakai pagination standar. Kolom waktu boleh kosong berpasangan, tanpa menghapus data lama. Minggu/nomor di atas 12 hanya ditambahkan ke tampilan bila terdapat definisi lama.
- Form jadwal: kelas → mapel sesuai penugasan pada tanggal draf → nama guru otomatis baca-saja → hari → dropdown jam mulai/selesai sesuai waktu yang sudah diatur. Pilihan akhir hanya rentang nomor berurutan. Tampilkan petunjuk dan nonaktifkan simpan bila pengampu atau jam belum tersedia; pertahankan input saat validasi gagal.
- Panduan yang disetujui 22 Oktober 2026 mengikuti [PRD utama bab 6](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#6-penugasan-dan-jadwal--final-inti): pilih semester, tetapkan satu pengampu per kelas-mapel, susun jadwal manual, lalu publikasikan. Guru yang sama dapat dipilih untuk banyak kelas/mapel; mata pelajaran yang sama dapat memiliki guru berbeda di kelas berbeda.
- Kurikulum mengatur jam pelajaran per hari, nomor jam, waktu mulai/selesai dan memilih rentang jam untuk jadwal mingguan berulang. Tampilkan semester dan tanggal mulai berlaku dengan jelas; Jadwal Saya hanya menampilkan versi yang sudah dipublikasikan sesuai penugasan. Penugasan semester sebelumnya dapat disalin sebagai bahan awal untuk disesuaikan. Pengampu bersama tetap ditunda; generator tersedia melalui tombol Generate jadwal pada Jadwal Mingguan.
- **Pengaturan D-024/D-025 — sudah tersedia:** sediakan Struktur Kurikulum per semester dengan bawaan tingkat dan pengecualian rombel–mapel. Tampilkan JP/minggu, pembagian pertemuan, aturan hari, sumber nilai bawaan/khusus, dan aksi kembali ke bawaan. Contoh Matematika 7A 2 JP, 7B 5 JP, semester baru 8 JP berasal dari pengaturan, bukan opsi angka tetap. Pengaturan entri/edit memakai Form in Modal TailAdmin; pilihan periode/rombel, pencarian instan, filter dan pagination mengikuti komponen bersama.
- **Istirahat dan Kegiatan — sudah tersedia:** tambahkan pengaturan istirahat mulai/selesai per hari dan kegiatan non-mapel. Di tampilan jadwal, istirahat berlabel **Istirahat** dan memblokir pembelajaran; Senin jam ke-1 berlabel **Upacara**, pembelajaran mulai jam ke-2. Tidak menampilkan upacara sebagai kosong atau memakai guru/mapel fiktif. JP mapel dihitung hanya dari slot belajar; jeda ditampilkan terpisah agar rentang belajar tidak menutupi waktu istirahat.
- **Generator draf — sudah tersedia:** alur pilih semester/tanggal → struktur dan pengecualian rombel → pengampu → waktu sekolah/istirahat/upacara → cek kesiapan → **Generate jadwal** → draf → tinjau/edit → validasi → **Publikasikan**. Tampilkan proses dan hasil, target versus JP terjadwal per rombel–mapel, kendala yang terbukti, serta perbedaan timeout dengan tidak ada solusi. Penggantian draf existing harus eksplisit; publikasi tetap tindakan Kurikulum. Detail aturan/status pada [PRD utama bab 6.1](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#61-rancangan-pengembangan-jadwal-fleksibel-dan-generator).
- Tabel jadwal berfilter hari/kelas/mapel/guru, ringkasan target versus JP, aktivitas Upacara/Istirahat, dan rentang belajar bersegmen jika melintasi jeda. Menu tiga titik draf menyediakan Edit/Hapus; riwayat publikasi memiliki Lihat versi. Modal generator menampilkan batasan wajib/preferensi, kapasitas guru, dan konfirmasi penggantian draf. Batas waktu pencarian bukan bukti jadwal mustahil.
- Perubahan jadwal: ubah hari/jam atau penugasan **pengampu resmi** sesuai kewenangan, dengan tanggal efektif dan alasan; tampilkan riwayat sebelum/sesudah. **Tidak ada** pilihan Guru Pengganti/Cadangan, penunjukan untuk satu pertemuan, atau tombol alihkan ke guru lain.
- Jika guru berhalangan, jadwal tetap tercatat tanpa membuat jurnal mengajar otomatis. Tampilkan kondisi belum ada jurnal tanpa menyimpulkan pembelajaran terlaksana.
- Guru memiliki tampilan **Jadwal Saya**, hanya penugasan aktif.

#### Batasan Jadwal — diterapkan, fase 7B

- Mengikuti [PRD utama bab 6.2](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#62-rancangan-batasan-jadwal-dan-preferensi) dan D-026. Antarmuka tersedia: menu **Batasan Jadwal** pada Kurikulum, dengan pilihan sasaran Guru atau Mata Pelajaran dan semester. Tabel mengikuti komponen pencarian/filter, pagination, nomor urut dan menu titik tiga; tambah/edit memakai Form in Modal sesuai standar TailAdmin proyek.
- Format waktu fase 7B (antarmuka utama kini kisi D-028) memilih guru/mapel, satu atau beberapa hari, waktu mulai/selesai aktual pada hari terkait atau **Sepanjang hari**, serta **Tidak tersedia** atau **Usahakan dihindari**. Tampilkan penjelasan: yang pertama wajib dipatuhi, yang kedua preferensi yang boleh dipakai untuk jadwal lengkap. Jangan otomatis memilih semua hari untuk permintaan menghindari pagi; waktu siang PJOK tidak disimpulkan dari nomor jam. Contoh jam boleh mengikuti konfigurasi sekolah sekarang tetapi tidak dikunci di frontend.
- Sebelum generate, tampilkan ringkasan larangan wajib dan preferensi secara terpisah serta kapasitas guru setelah larangan wajib. Slot preferensi tetap tersedia. Saat hasil menjadi draf, tampilkan status pemeriksaan wajib dan laporan preferensi: guru/mapel, hari dan waktu aktual yang mengenai preferensi, JP terkait, dan jumlah hari guru masuk pagi. Preferensi belum terpenuhi merupakan informasi tinjauan, bukan error yang memblokir publikasi.
- Label kualitas hasil harus jujur: **hasil terbaik yang ditemukan** bila belum terbukti optimal; jika pencarian berhenti tanpa jadwal lengkap, jelaskan batas pencarian. Hindari pesan “harus masuk pagi” atau “tidak mungkin menghindari pagi” tanpa bukti. Alasan kapasitas yang terukur boleh ditampilkan. Larangan wajib yang dilanggar tetap menolak edit/publikasi; edit pada waktu preferensi memperbarui penanda/laporan.
- Perubahan pengaturan menandai draf perlu diperiksa ulang dan tetap menjaga versi terbit. Laporan preferensi perlu terlihat pada tinjauan sebelum publikasi dan pada versi yang dipublikasikan sesuai cakupan akses. Penilaian: pasangan guru–hari preferensi dahulu, lalu JP preferensi unik; posisi versi lama sebagai urutan pilihan berikutnya. Cakupan mapel seluruh rombel; bobot/pemerataan khusus atau batasan mapel khusus rombel belum tersedia.

#### Kisi ketersediaan — diterapkan, D-028
Editor utama mengikuti [PRD bab 6.3](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#63-kisi-ketersediaan-guru-dan-mapel--diterapkan-22-oktober-2026): pilih sasaran/nama, buka modal lebar terpusat dengan backdrop buram, lalu tandai kisi hari × nomor jam dengan Tersedia/Kondisional/Tidak tersedia. Simbol, teks aksesibel dan warna ringan membedakan status. Gunakan klik biasa, radio status dan tombol satu hari; reset Semua Tersedia hanya mengubah editor hingga disimpan. Tampilkan waktu aktual tiap sel. Upacara/slot tanpa waktu terkunci; baris tanpa slot tidak dapat ditandai. Kisi dapat digeser horizontal dalam modal ponsel, header hari tetap terlihat; halaman tidak melebar dan matriks editor tidak dipaginasi.

Daftar kisi tersimpan memakai pencarian instan, nomor urut, pagination 10 dan menu tiga titik horizontal. Batasan waktu lama yang belum dialihkan ditampilkan terpisah beserta aksi Atur kisi; pratinjau pemetaan dan persetujuan disediakan sebelum Simpan. Perubahan jam sekolah menampilkan permintaan tinjauan/konfirmasi ulang. Konflik versi tidak menimpa tab lain, validasi mempertahankan pilihan sel dan membuka kembali modal. Halaman dan generator menggunakan istilah Kondisional untuk preferensi; mekanisme JP/publikasi/laporan tetap mengikuti 6.2.

#### Impor XML aSc — D-029
Tombol Impor XML berada pada Jadwal Mingguan Kurikulum. Form unggah menggunakan modal TailAdmin terpusat/backdrop buram; tanggal mengikuti draf bila sudah ada. Setelah unggah, halaman menampilkan padanan kelas/guru/mapel/hari/jam per hari, saran nama unik, pencarian instan dan pagination 10 per tabel, serta pratinjau nomor/hari/rentang/waktu sumber/waktu sekolah dan pemeriksaan. Pilihan jam mengikuti hari tujuan; perubahan padanan menonaktifkan konfirmasi sampai pratinjau diperiksa ulang. Pesan rincian per baris tetap terlihat; notifikasi operasi mengikuti komponen bersama.

Modal konfirmasi hanya aktif setelah pemeriksaan berhasil, menampilkan jumlah pertemuan dan persetujuan penggantian draf, draf belum lengkap dan penggunaan waktu sekolah jika diperlukan. Ekspor terpotong menahan penyimpanan; kelengkapan target sekolah dibedakan dari validitas blok sumber. Tidak ada publikasi otomatis. Tampilan ponsel membatasi scroll tabel dalam panel dan mempertahankan tombol utama yang muat. Batas format/validasi pada [PRD bab 6.4](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#64-impor-jadwal-xml-asc--23-oktober-2026).

### 3.5 Absensi — D-033 (24 Oktober 2026)
- Fondasi pertama: Kurikulum → Kalender Akademik → Kalender Sekolah (semester/rentang/hari mingguan/libur/pengecualian, form modal, versi dan histori). Admin → Penempatan → Tanggal Mulai Penempatan (lengkapi tanggal lama tanpa menebak; koreksi modal beralasan). Waktu WIB; konteks roster/jadwal per tanggal.
- D-038/D-039: Admin mengatur koordinat sekolah dan radius 25–500 meter (awal 100 m); akurasi GPS browser maksimal 50 m. Guru aktif yang memiliki akun tertaut memakai GPS dan selfie langsung untuk Masuk/Pulang. Selfie maksimal 100 KB, privat dan dihapus setelah dua bulan kalender; metadata/histori tetap. TU Pengelola Kehadiran Guru menangani fallback manual dan koreksi historis tanpa batas usia tanggal; Petugas Absensi Siswa tidak mengelola kehadiran guru. Guru dapat mengajukan permintaan koreksi tanpa memutasi catatan. Izin/Sakit/Dinas Luar dapat dilaporkan guru untuk tanggal lampau/hari ini/mendatang, form laporan tersedia pada Kehadiran Saya, dengan riwayat berhalaman serta detail/Minta Koreksi. TU membuka Laporan Guru untuk filter, detail, histori, koreksi dan pembatalan beralasan. Guru tidak mendapat tombol Edit/Hapus; validasi dan hak akses diperiksa server. DEV-03G3 menyediakan ringkasan hari ini, Rekap Kehadiran Saya, dan Rekap Kehadiran Guru untuk pasangan role TU berwenang: filter rentang tanggal, pencarian guru, pagination, jumlah per status dan detail dasar penilaian/tautan histori. Belum Dicatat, Belum Lengkap, Belum Dapat Dinilai, Tidak Terjadwal, Sedang Berlangsung dan Akan Datang dibedakan. Belum ada sumber tugas nonmengajar atau rumus persentase final. Implementasi lokal menunggu UAT. Ketentuan status merujuk PRD utama §7.2 dan D-039.
- Petugas: pemindai QR kartu siswa → nama/kelas/waktu/hasil; kedatangan saja; scan ganda aman; status lama tidak ditimpa. Daftar kelas untuk Sakit/Izin/Alpa/manual. Hilang → Ganti QR (kode lama mati), rusak masih dipegang → Cetak ulang kode sama; alasan/histori, identitas/absensi utuh.
- Guru pengampu: Jadwal Saya → tanggal/pertemuan → absensi pelajaran; harian dan pelajaran terpisah; Hadir/Sakit/Izin/Alpa/Belum Dicatat. Tandai semua Hadir hanya lewat aksi eksplisit pengguna.
- Koreksi absensi per pelajaran mengikuti hak historis D-037. Guru pengampu lama membuka catatan dari daftar tanggal atau jadwal pertemuan; Kurikulum melihat daftar fallback administratif pada tanggal yang sama hanya ketika akun/role Guru Mapel historis tidak lagi aktif atau tidak terhubung. Detail memakai ID catatan permanen, tidak bergantung pada jadwal/roster terbaru. Halaman detail menampilkan status, versi, konteks snapshot, dan histori pelaku/waktu WIB/status sebelum-sesudah/alasan. Tombol koreksi membuka modal dengan status lama, status baru, alasan wajib, ID dan expected version tersembunyi serta CSRF; koreksi administratif diberi label jelas. Validasi mempertahankan isian. HTTP 409 menampilkan status terkini dan meminta pengguna meninjau ulang tanpa mengganti versi atau submit otomatis. Status yang sama adalah no-op yang diberi pesan.
- Koreksi tidak memerlukan approval Kepala Sekolah. Wali/Kesiswaan/Kepala Sekolah membaca sesuai PRD 7; hak tersebut tidak memperluas kewenangan koreksi. Ketentuan otoritatif dan urutan implementasi lengkap di PRD utama 7.1–7.5.

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

### 3.9 Kesiswaan dan BK sederhana — rencana v1 disetujui 22 Oktober 2026

Rujukan kebutuhan otoritatif: [PRD utama bab 3B](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#3b-kesiswaan-dan-bk--final-22-oktober-2026). Catatan Pembinaan Kesiswaan tersedia pada 8A-02, sedangkan koordinasi kasus telah terintegrasi lokal ke branch review melalui migration 00029–00030, service, dan UI. UAT FINAL Product Owner PASS (8/8 fungsional, 4/4 UX-FIX-01, 5/5 handoff D-046); push tertahan karena autentikasi SSH ditolak. Modul BK mandiri tidak termasuk tahap ini.

- Kesiswaan melihat siswa/rekap kehadiran sekolah dan mengelola catatan pembinaan; BK mengelola catatan bimbingan untuk siswa/kasus yang ditangani.
- Gunakan daftar catatan dan form sederhana berisi field yang ditetapkan bab 3B. Form entri mengikuti Form in Modal; tabel, pencarian, filter, badge status, dan pagination mengikuti komponen TailAdmin bersama yang sudah digunakan.
- Status hanya **Perlu tindak lanjut** dan **Selesai**. Tidak menampilkan skor/poin, sanksi otomatis, atau antrean persetujuan.

**Koordinasi kasus — [D-043](docs/DECISIONS.md#d-043--konsep-koordinasi-penanganan-kasus-siswa-30-oktober-2026)/[D-044](docs/DECISIONS.md#d-044--alur-berjenjang-koordinasi-kasus-siswa-30-oktober-2026)/[D-045](docs/DECISIONS.md#d-045--otorisasi-pemantauan-kasus-dan-kerahasiaan-konseling-30-oktober-2026)/[D-046](docs/DECISIONS.md#d-046--pengambilalihan-penanganan-kasus-akibat-perubahan-wali-kelas-30-oktober-2026):** alur formal normal di UI wajib Wali Kelas → Guru BK → Kesiswaan. Wali Kelas aktif membuat pembinaan awal dan memantau ringkasan/perkembangan; penanggung jawab aktif berpindah mengikuti tahap. UI hanya menawarkan rujukan rutin ke tahap berikutnya dan tidak menyediakan aksi lewati tahap. Setelah perubahan penempatan/penugasan, Wali aktif baru dapat mengambil alih secara eksplisit hanya pada kasus terbuka tahap Wali, dengan alasan dan histori atomik; kasus selesai atau tahap BK/Kesiswaan tidak dialihkan. Kondisi penempatan/penugasan ambigu ditolak, dan GET tidak memutasi. Siswa yang datang langsung ke BK tetap mendapat bantuan; aplikasi tidak boleh menahan pertolongan darurat demi pengisian form, dan bantuan awal/darurat dilakukan menurut prosedur resmi sekolah di luar aplikasi. Hanya Guru BK penanggung jawab aktif yang membaca catatan konseling pribadi. Kepala Sekolah membaca ringkasan, Kesiswaan hanya informasi koordinasi yang perlu, dan Admin/role lain tidak mendapat akses otomatis. Isi konseling tidak boleh masuk daftar, pencarian, pesan error, atau log. Penanggung jawab aktif dapat menyelesaikan/membuka kembali kasus dengan alasan dan histori tanpa approval. Entri pembinaan awal disimpan pada domain kasus terpisah dari Catatan Pembinaan existing menurut D-042. UAT awal kasus dilaporkan PASS; handoff baru menunggu UAT tambahan.
- Wali Kelas membaca catatan siswa di kelasnya dan Kepala Sekolah membaca seluruh sekolah; keduanya mendapat tampilan baca-saja sesuai role tersebut. Batas baca/tulis tetap diperiksa server-side.

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

Nama menu dan layout detail adalah **RANCANGAN**; aturan peran/penugasan sederhana adalah **FINAL**. Jangan menambah approval digital hanya karena tersedia komponen tombol/modal pada TailAdmin. Detail rumus nilai, katalog laporan, dan GPS/selfie dan QR siswa mengikuti D-033/PRD utama bab 7.

Perpindahan D-031: aksi Pindah kelas hanya Kurikulum pada penempatan; Pindah sekolah hanya Admin pada master siswa. Form dan ringkasan konfirmasi mengikuti Form in Modal TailAdmin (pusat/blur), tombol leading icon/grup, histori bernomor/pagination. Tanggal berlaku dan alasan wajib; sekolah tujuan opsional. Badge Pindah sekolah atau Pindah terjadwal dengan tanggal berbeda dari nonaktif umum.

D-032: Data Siswa Admin menyediakan Terima kembali hanya untuk siswa yang sudah keluar dan belum memiliki penerimaan terjadwal. Form/ringkasan memakai modal TailAdmin bersama. Tujuan belum dipilih otomatis; kelas pada tahun aktif, tingkat sama jika kembali pada tahun asal. Tampilkan sekolah asal terakhir opsional, tanggal kembali, alasan, checkbox konfirmasi, dan badge Kembali terjadwal. Riwayat menampilkan setiap keluar/kembali tanpa menggandakan identitas siswa.

### Tanggal awal penempatan massal — D-034
Halaman tanggal Admin menyediakan tombol Isi tanggal massal. Form mencakup filter kelas awal, tanggal dengan saran Semester 1, alasan, checkbox siswa, Pilih semua/Batalkan pilihan, hitungan seluruh pilihan, pencarian dan pagination 10 baris. Pencarian tidak membatalkan pilihan pada halaman lain. Pratinjau valid membuka modal ringkasan siswa/tanggal/alasan dengan checkbox konfirmasi; pratinjau bermasalah menampilkan alasan per siswa dan tidak menawarkan Simpan. Batas masa pratinjau 10 menit dijelaskan. Form penempatan awal tetap meminta tanggal dan dapat memakai saran semester yang terlihat.

### Absensi harian siswa dan kartu — D-035
Role berhak baca memperoleh menu Absensi Harian, filter tanggal/tahun-kelas, status dan pencarian; tabel bernomor/pagination 10 menampilkan status, sumber dan waktu WIB. Filter tanggal/kelas dipertahankan pada URL sesudah simpan/koreksi; filter status tersimpan selama sesi browser. Petugas memakai menu aksi Catat manual/Koreksi-histori; Wali/Kesiswaan/Kepsek hanya melihat histori. Modal menunjukkan nilai lama, nilai baru, alasan dan histori. Menu Pemindai QR dan Kartu QR Siswa hanya Petugas. Pemindai meminta kamera melalui tombol, menampilkan hasil nama/kelas/waktu, berhenti menunggu Scan siswa berikutnya dan menawarkan USB/manual saat gagal. Tidak mengunggah video/foto kamera pemindai. Kartu menyediakan filter kelas/pencarian, modal tindakan beserta konfirmasi dan alasan, serta halaman cetak privat yang hanya mencetak kartu. Cetak ulang dan penggantian kode dibedakan jelas.

Perbaikan waktu manual harian (24 Oktober 2026): isian waktu datang memakai input 24 jam HH:MM dengan keypad angka dan format otomatis empat angka (0611 → 06:11, 1811 → 18:11). Tidak memakai pemilih native AM/PM yang kontrasnya bergantung perangkat. Teks bantuan menjelaskan pagi/sore; sesuai D-036 waktu wajib untuk Hadir baru/koreksi. Validasi browser/server menolak format AM/PM dan jam/menit di luar rentang. Berlaku untuk Catat manual dan Koreksi absensi harian.

### Keterlambatan/Isi cepat — D-036
Kesiswaan memiliki menu Batas Kedatangan: semester, semua/rombel, mingguan/tanggal, mulai berlaku, jam HH:MM 24 jam, toleransi menit, alasan dan histori versi. Perubahan tidak menulis ulang bukti lama. Tabel harian menampilkan keterangan ketepatan waktu. Jam Hadir baru/koreksi wajib; indikator data lama belum lengkap/batas belum diatur dibedakan dari status kehadiran.
Petugas membuka Isi cepat: semua siswa satu kelas tanpa pagination saat mengisi; H/S/I/A langsung di tiap baris, waktu Hadir per siswa, pengisian jam bersama yang eksplisit, tombol tandai sisanya Hadir, alasan kelompok dan ringkasan sebelum Simpan. Baris QR/manual tersimpan terkunci. Pencarian hanya mengubah tampilan. Isian tetap saat validasi/konflik; navigasi diperingatkan saat ada perubahan belum disimpan. Tabel rekap tetap pagination 10. Aksi Catat datang sekarang mencatat Hadir dengan waktu server ketika petugas menerima siswa, bukan waktu rekap yang dianggap waktu datang.


## Standar Instant Search lintas role — D-040 (29 Oktober 2026)

Keputusan Product Owner: seluruh pencarian/filter daftar existing dan baru memakai Instant Search. Saat JavaScript aktif, ketikan/clear dan filter valid memperbarui komponen hasil tanpa Enter, tombol Cari/Tampilkan atau reload halaman penuh. Server memakai GET HTMX dengan debounce default 350 ms; SQL mencari seluruh cakupan berwenang sebelum pagination. Pencarian lokal hanya untuk data lengkap yang tersedia dalam cakupan browser. Tidak perlu membuat kotak pencarian baru pada halaman yang tidak membutuhkannya.

Fokus, posisi kursor, teks, filter aktif, URL dan pagination dipertahankan; perubahan filter kembali ke halaman pertama. URL memakai replace tanpa entri setiap karakter. Rentang tanggal parsial/terbalik diberi petunjuk tanpa menghapus nilai atau menjalankan query. Hasil lama tetap terlihat saat loading/gagal, tersedia retry dan pesan kosong **Tidak ada data yang cocok dengan pencarian.** Respons basi tidak menimpa ketikan terbaru. Modal terbuka dan isian belum disimpan dilindungi; form POST/Simpan/Koreksi/Batalkan bukan pencarian. Fallback request normal tetap menghasilkan halaman lengkap. Ketentuan ini tidak mengubah aturan kehadiran atau kewenangan role. Pola teknis/inventaris: [INSTANT_SEARCH_AUDIT.md](docs/INSTANT_SEARCH_AUDIT.md). UAT ulang Product Owner DEV-03G3 beserta revisi ini: PASS; publikasi menunggu final gate.
