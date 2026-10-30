# PRD SIM-GURU v1.0

**Status:** Baseline kebutuhan untuk agen AI, revisi penyederhanaan kewenangan dan penghapusan guru pengganti/cadangan, 21 Oktober 2026; penyelarasan pembagian tugas dan lingkungan disetujui pemilik proyek.\
**Ruang lingkup:** Satu SMP, aplikasi web mandiri.\
**Dokumen pendamping:** `PRD_FE_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md`, `ROADMAP_IMPLEMENTASI_SIM_GURU_REVISI_PEMBAGIAN_TUGAS.md`.

## 0. Instruksi yang mengikat agen AI

1. Baca ketiga dokumen. Keputusan bertanda **FINAL** mengikat; **OPEN** hanya menahan fitur yang terdampak; **RANCANGAN** boleh diputuskan secara teknis tanpa mengubah kebijakan sekolah.
2. **Jangan membangun sistem mandat, delegasi, inbox approval, atau persetujuan digital berlapis.** Sekolah memutuskan penugasan di luar aplikasi; Admin menyediakan data induk siswa/guru, membuat akun, menetapkan role, membuat kelas dan menempatkan siswa pada awal tahun; Kurikulum mengatur penugasan akademik; hak mengikuti role dan penugasan yang aktif.
3. Kepala Sekolah **bukan pemberi persetujuan rutin**. Hanya tindakan khusus yang secara eksplisit disebut memerlukan persetujuan beliau. Persetujuan dalam rapat/offline tidak harus direplikasi menjadi tombol persetujuan digital.
4. Periksa hak akses di server untuk setiap tindakan. Catat perubahan penting: pelaku, waktu, objek, nilai sebelum/sesudah bila relevan, dan alasan untuk koreksi.
5. Jangan menambahkan integrasi E-SPP, Tabungan Siswa, E-Ujian, WhatsApp, akun siswa/orang tua, rapor resmi, atau modul di luar v1.
6. **Tidak ada Guru Pengganti atau Guru Cadangan**, termasuk role, penugasan sementara, hak akses sementara, formulir, dan workflow pengganti. Perubahan pengampu resmi adalah proses akademik berbeda yang dikelola Kurikulum sesuai keputusan sekolah.
7. Jangan mengklaim fitur selesai sebelum tes nyata. Jangan menebak bobot nilai, rumus, atau kebijakan sekolah yang masih OPEN.

## 1. Teknologi dan lingkungan pengembangan — FINAL

| Lapisan | Pilihan |
|---|---|
| Komputer pengembangan | Ubuntu 26.04 LTS (periksa varian dan kondisi mesin saat mulai) |
| Backend | Go |
| Render HTML | Pustaka Go `html/template` |
| Interaksi dinamis | htmx, respons fragmen HTML dari Go |
| Database | PostgreSQL |
| Styling dan UI | Tailwind CSS + TailAdmin Free versi HTML |
| Template sumber | https://github.com/TailAdmin/tailadmin-free-tailwind-dashboard-template |
| JavaScript pendukung | Alpine.js bila dibutuhkan komponen visual |
| Build assets | Node.js/npm hanya tooling frontend bila diperlukan |
| Arsitektur | Modular monolith dengan server-side rendering |

**Larangan mengganti stack** dengan React/Next/Vue, SPA wajib, atau backend Node tanpa persetujuan. Periksa versi Go/PostgreSQL/Node, RAM, disk, port, nama DB, dan path repo secara aktual; jangan mengarang spesifikasi mesin. Aset CSS/JS dibangun dan dilayani lokal. PostgreSQL SIM-GURU terpisah dari proyek lain. Gunakan `.env` lokal, migrasi terversi, transaksi untuk operasi kritis, sesi aman, CSRF, dan validasi server-side.

## 2. Tujuan dan batas aplikasi — FINAL

SIM-GURU menyatukan master data siswa/guru/mapel, tahun ajaran dan semester, kelas/penempatan, penugasan dan jadwal, absensi guru GPS dan selfie, absensi siswa harian dan per pelajaran, catatan pembinaan Kesiswaan dan bimbingan BK sederhana, jurnal mengajar, penilaian, serta laporan akademik dan kehadiran. Data historis dipertahankan; laporan bukan rapor resmi. Satu sekolah, bukan multi-tenant.

## 3. Model kewenangan sederhana — FINAL (menggantikan rumusan mandat/delegasi sebelumnya)

**Alur tunggal:** rapat/keputusan sekolah di luar aplikasi → **Admin mengelola data siswa/guru, akun, role, kelas dan penempatan awal; Kurikulum mengelola kalender, penugasan dan jadwal akademik** → hak akses langsung aktif → personel bekerja sesuai tugas. Tidak ada permintaan dan approval digital untuk penetapan rutin.

| Peran | Hak utama | Batas |
|---|---|---|
| Admin | Pengelola identitas siswa/guru, akun, role, status akses, konfigurasi teknis, pembuatan/koreksi kelas dan penempatan awal siswa | Tidak otomatis mengelola kalender, mapel, wali kelas, pengampu, jadwal, transisi atau nilai |
| Kurikulum | Menyiapkan tahun ajaran beserta dua semester dan mengaktifkannya; kelola mapel, wali kelas, pengampu, jadwal, transisi kenaikan/kelulusan dan pemeriksaan koreksi nilai Final | Kelas dan penempatan awal siswa baca-saja; tidak mengubah identitas siswa/guru; tidak membutuhkan persetujuan digital rutin |
| Guru Mapel | Absensi per pelajaran, jurnal, TP/komponen/nilai/remedial pada kelas-mapel yang ditugaskan | Tidak boleh mengubah kelas/mapel di luar penugasan |
| Wali Kelas | Pantau siswa dan absensi kelas yang ditugaskan, tindak lanjut kelas; baca catatan Kesiswaan/BK siswa di kelasnya | Hak baca catatan tidak memberi hak edit; tidak otomatis mengubah nilai seluruh mapel |
| Kepala Sekolah | Lihat ringkasan/laporan sekolah dan baca catatan Kesiswaan/BK seluruh sekolah; menangani keputusan akademik khusus | Hak baca catatan tidak memberi hak edit; tidak menjadi antrean persetujuan pekerjaan harian |
| BK | Lihat siswa yang ditangani; catat bimbingan/konseling dan tindak lanjutnya | Terbatas pada siswa/kasus yang ditangani; tidak otomatis mengubah identitas, penempatan, jadwal, atau nilai |
| Kesiswaan | Lihat seluruh siswa dan rekap kehadiran sekolah; atur batas kedatangan/toleransi siswa (D-036); catat pelanggaran serta pembinaan yang dilakukan | Tidak otomatis membuka catatan BK atau mengubah identitas, sumber absensi, penempatan, jadwal, atau nilai |
| Petugas Absensi Siswa | Kelola absensi harian dan kartu QR siswa | Tidak memiliki akses kehadiran guru |
| Pegawai TU | Role pegawai administratif | Tidak otomatis mengelola kehadiran guru |
| TU Pengelola Kehadiran Guru | Catat manual, lengkapi, dan koreksi histori kehadiran guru; melihat permintaan guru | Harus merangkap Pegawai TU; tidak boleh mengubah kehadirannya sendiri |

**Role Kurikulum langsung memberi akses ke menu dan fungsi Kurikulum.** Role dapat rangkap; penugasan Guru Mapel dan Wali Kelas tetap membatasi kelas/mapel. Admin boleh mengubah role berdasarkan keputusan sekolah, dengan audit sederhana. Pengelolaan penugasan oleh Kurikulum dalam modul akademik dapat berupa data operasional, tetapi **Admin tetap pengelola akun dan role; Kurikulum mengelola penugasan akademik**; tidak perlu approval silang. Bila Kurikulum menyusun pengampu/jadwal, sistem harus menyelaraskan dengan penugasan yang telah ditetapkan tanpa memberikan hak baru diam-diam.

### 3.1 Tindakan yang memerlukan persetujuan Kepala Sekolah

| Tindakan | Aturan v1 |
|---|---|
| Penunjukan Kurikulum, Wali Kelas, Guru Mapel, BK, Kesiswaan | **Tidak perlu approval digital**; Admin mengatur role; Kurikulum mencatat penugasan akademik hasil keputusan sekolah |
| Membuat kelas, penempatan awal, jadwal, perubahan jadwal | **Tidak perlu approval Kepala Sekolah di aplikasi**; dilakukan petugas berwenang |
| Absensi, jurnal, input/finalisasi nilai rutin | **Tidak perlu approval Kepala Sekolah** |
| Kenaikan kelas, kelulusan, transisi tahun ajaran | Berdasarkan keputusan sekolah; Kurikulum mengeksekusi setelah pratinjau/validasi, **tanpa approval digital ulang** |
| Perpindahan siswa antarkelas di tengah tahun | Kurikulum menjalankan langsung dengan alasan dan histori; tanpa persetujuan Kepala Sekolah |
| Koreksi nilai yang sudah Final | Perlu alasan, histori, dan pemeriksaan oleh Kurikulum; **tidak otomatis memerlukan Kepala Sekolah**. Sengketa luar biasa dapat dieskalasi secara offline |
| Koreksi jurnal Final atau absensi | Pelaksana berwenang mengoreksi dengan alasan dan histori; **tanpa approval Kepala Sekolah** |

**Catatan perubahan:** ketentuan lama yang menyebut “mandat”, “delegasi”, “persetujuan berlapis”, atau “larangan self-approval” untuk penugasan rutin **digantikan** oleh bab ini. Jangan hidupkan kembali mekanisme tersebut dari ringkasan kontrak lama. Berdasarkan keputusan D-030, perpindahan tengah tahun ditangani langsung oleh Kurikulum dengan alasan dan histori, tanpa persetujuan Kepala Sekolah.

## 3A. Kepemilikan data induk dan batas tugas — FINAL (revisi 21 Oktober 2026)

- **Admin adalah satu-satunya pihak yang membuat, mengubah, menonaktifkan, mengoreksi, dan melengkapi identitas induk siswa dan guru.** Ini meliputi identitas, NIS/NISN jika ada, data kontak yang diperlukan, status siswa/guru, dan identitas akun terkait. Hak baca bagi role lain diberikan sesuai kebutuhan tugas, bukan hak mengubah identitas.
- **Admin mengelola akun dan role, membuat/mengoreksi kelas, serta melakukan penempatan awal siswa.** Admin membaca pilihan tahun ajaran untuk kelas; hak ini tidak memberikan kewenangan menyiapkan/mengaktifkan kalender, mengelola mapel, wali kelas, pengampu, jadwal, transisi atau nilai.
- **Kurikulum menerima data induk dan kelas secara baca-saja**, menyiapkan tahun ajaran beserta dua semester, mengaktifkan tahun/semester, mengelola mata pelajaran, penugasan guru pengampu, wali kelas, jadwal, transisi kenaikan/kelulusan, serta pemeriksaan koreksi nilai Final. Kurikulum tidak membuat/mengoreksi kelas, melakukan penempatan awal, atau mengubah identitas induk siswa/guru. Kurikulum menangani perpindahan tengah tahun secara langsung dengan alasan dan histori; persetujuan Kepala Sekolah tidak diperlukan. Perubahan penempatan hasil transisi mengikuti eksekusi transisi oleh Kurikulum, bukan hak mengedit penempatan awal secara bebas.
- **Guru Mapel, Wali Kelas, BK, Kesiswaan, dan Kepala Sekolah** membaca data siswa/guru sesuai cakupan role; hanya mengubah data operasional milik modul mereka. Permintaan pembetulan identitas diteruskan ke Admin melalui komunikasi sekolah biasa, tanpa membangun workflow approval digital.
- **Satu sumber data:** siswa/guru dibuat sekali oleh Admin dan dipakai melalui referensi ID internal pada seluruh modul; tidak boleh ada salinan identitas yang diedit bebas oleh modul lain.
- **Hak akses gabungan:** role diberikan Admin; cakupan kelas/mapel/wali berasal dari penugasan Kurikulum. Akses server harus memeriksa keduanya bila diperlukan.
- Audit perubahan identitas dan penugasan mencatat pelaku, waktu, objek, serta sebelum/sesudah. Jangan menghapus histori yang telah digunakan modul akademik.

**Acceptance criteria:** Kurikulum tidak bisa mengubah nama/NISN siswa atau identitas guru; Admin dapat membuat/mengoreksi kelas dan melakukan penempatan awal; Kurikulum tanpa role Admin tidak dapat melakukannya; Admin tanpa Kurikulum tidak bisa menetapkan pengampu atau mengubah kalender; Admin dapat memperbaiki identitas dan perubahannya tercermin pada seluruh modul; penugasan oleh Kurikulum langsung mengaktifkan cakupan akses Guru Mapel/Wali Kelas tanpa persetujuan digital.

**Revisi disetujui pemilik proyek 21 Oktober 2026:** pembuatan/koreksi kelas dan penempatan awal beralih ke Admin; kalender dan seluruh tugas Kurikulum yang disebut di atas tetap Kurikulum. ID kelas dan histori sebelum perubahan kebijakan dipertahankan. Rujukan keputusan: [D-015](docs/DECISIONS.md#d-015--pemisahan-kelasadmin-dan-kalenderkurikulum-disetujui-21-oktober-2026).

## 3B. Kesiswaan dan BK — FINAL (D-018, diperbarui D-042 pada 29 Oktober 2026)

**Status:** kebutuhan awal disetujui melalui [D-018](docs/DECISIONS.md#d-018--kesiswaan-dan-bk-sederhana-disetujui-22-oktober-2026). Catatan Pembinaan Kesiswaan tahap 8A-02 telah diimplementasikan sesuai D-042. Koordinasi kasus dan layanan konseling kasus Fase 8A-03 tersedia lokal melalui migration 00029–00030, service, dan UI; UAT FINAL Product Owner PASS (8/8 fungsional, 4/4 UX-FIX-01, 5/5 handoff D-046). Final gate publikasi berlangsung; `sim_guru_dev` tidak dimigrasikan dalam proses ini.

- **Kesiswaan:** melihat seluruh siswa dan rekap kehadiran sekolah, serta mencatat pelanggaran dan pembinaan yang dilakukan.
- **BK:** melihat siswa yang menjadi tanggung jawabnya, serta mencatat bimbingan/konseling dan tindak lanjut pada siswa/kasus yang ditangani.
- **Wali Kelas:** membaca catatan Kesiswaan siswa berdasarkan penempatan kelas aktif siswa dan penugasan Wali Kelas yang berlaku saat pembacaan; membaca catatan BK sesuai cakupan yang berlaku untuk catatan BK. Perpindahan siswa mengalihkan hak baca catatan Kesiswaan kepada Wali Kelas baru, dan mantan Wali Kelas tidak mempertahankan akses melalui penugasan historis. Jika penempatan atau penugasan aktif tidak dapat dipastikan, akses catatan Kesiswaan ditolak. Wali Kelas tidak memperoleh hak mengubah catatan.
- **Kepala Sekolah:** membaca catatan dan ringkasan Kesiswaan/BK seluruh sekolah; tidak menjadi pemberi persetujuan rutin atau pengubah catatan dari role Kepala Sekolah.
- Setiap catatan cukup memuat **siswa, tanggal, kejadian/keperluan, tindakan, petugas, dan status**. Status hanya **Perlu tindak lanjut** atau **Selesai**. Petugas pencatat tetap tercatat sesuai akun yang melakukan tindakan.
- Siswa mengacu pada ID master yang dikelola Admin. Identitas, kelas/penempatan, jadwal, nilai, dan pencatatan/koreksi sumber absensi tetap mengikuti kewenangan modul masing-masing.
- **Tidak ada poin pelanggaran, sanksi otomatis, atau alur persetujuan pada v1 ini.** Pengembangan lebih lanjut dilakukan bila ada kebutuhan dan persetujuan baru; usulan audit yang lebih luas tidak otomatis masuk cakupan.
- Identitas dan kelas siswa pada saat kejadian disimpan sebagai snapshot historis dan tidak menentukan hak baca saat ini. BK, Guru Mapel, Admin, dan role lain tidak otomatis mendapat akses baca catatan Kesiswaan. Role gabungan hanya memperoleh hak dalam cakupan masing-masing role yang sah; role lain tidak memperluas cakupan.
- Akses baca catatan BK tetap mengikuti D-018: Kesiswaan tidak otomatis mendapat akses ke catatan BK dan cakupan BK hanya mencakup siswa/kasus yang ditangani. D-042 hanya menetapkan cakupan baca catatan Kesiswaan.

**Acceptance criteria rencana:** Kesiswaan mengelola catatan pembinaan seluruh sekolah; BK dibatasi pada siswa/kasus yang ditangani; Wali Kelas membaca catatan Kesiswaan hanya untuk penempatan aktif yang cocok dengan penugasan aktif saat baca dan akses ditolak jika cakupan tidak jelas; Kepala Sekolah membaca seluruh sekolah. Identitas/kelas saat kejadian disimpan sebagai snapshot historis. Catatan menggunakan field dan dua status di atas, pemeriksaan akses dilakukan di server, serta tidak ada poin/sanksi otomatis/approval rutin. Perubahan catatan penting mengikuti audit sederhana bab 11. Cara mencatat penanggung jawab siswa/kasus BK perlu ditetapkan saat fase implementasi agar cakupan dapat diperiksa, tanpa menambahkan workflow persetujuan. Cakupan Wali Kelas untuk catatan Kesiswaan ditetapkan FINAL melalui D-042; keputusan tersebut belum menetapkan detail cakupan BK.

### Konsep koordinasi kasus — D-043 (disetujui 30 Oktober 2026)

Setiap kasus memiliki ID permanen, siswa master dan snapshot konteks, satu penanggung jawab saat ini, status Perlu tindak lanjut atau Selesai, dan banyak entri tindakan/tindak lanjut. Rujukan kepada petugas lain yang berwenang dan perubahan penanggung jawab menjadi peristiwa histori tersendiri; perubahan tidak menimpa catatan atau tanggung jawab terdahulu. Catatan Pembinaan Kesiswaan existing tetap dipertahankan dan dapat dihubungkan ke kasus secara aditif. Konsep ini tidak mengubah akses D-042, tidak membuka isi konseling pribadi, serta tidak menambahkan poin, sanksi otomatis, atau approval digital berlapis.

**Rancangan relasi minimal (belum skema implementasi):** entitas `student_cases` menyimpan ID kasus, ID siswa resmi, snapshot identitas/konteks saat kasus dibuka, status, dan penanggung jawab saat ini; entitas anak `student_case_entries` menyimpan banyak tindakan/tindak lanjut beserta aktor dan waktu server; histori append-only `student_case_history` mencatat rujukan dan perubahan penanggung jawab dengan aktor, waktu, alasan bila relevan, serta nilai sebelum/sesudah. Catatan `student_guidance_records` yang sudah ada tetap menjadi catatan yang berdiri sendiri dan dapat memiliki tautan nullable ke kasus; baris lama tidak dipindahkan, digabung, atau diubah otomatis. Rujukan tidak dengan sendirinya memberikan hak baca. Pilihan ini adalah rancangan teknis, bukan keputusan otorisasi.

Catatan status keputusan: D-043 semula mencatat alur penanggung jawab dan rincian akses sebagai OPEN. D-044 kini menetapkan urutan normal dan pihak perujuk pada tiap tahap. Kewenangan penugasan ulang di luar alur normal, penutupan kasus, serta batas baca isi konseling tetap OPEN; persetujuan konsep tidak memberi akses konseling otomatis kepada Kesiswaan, Wali Kelas, Kepala Sekolah, Admin, atau petugas yang dirujuk.

### Alur formal kasus berjenjang — D-044 (FINAL, 30 Oktober 2026)

Alur normal wajib **Wali Kelas → Guru BK → Kesiswaan**. Wali Kelas memulai kasus formal, mencatat pembinaan awal dan mengevaluasi perkembangannya. Bila belum berhasil atau membutuhkan konseling, Wali Kelas merujuk ke Guru BK. Guru BK mencatat layanan dan tindak lanjut; bila belum ada perbaikan dan perlu penegakan disiplin, Guru BK meneruskan ke Kesiswaan. Kesiswaan menentukan penanganan disiplin menurut tata tertib dan kewenangan sekolah, tanpa sanksi otomatis.

Kasus tetap memiliki satu **penanggung jawab aktif** pada tahap yang berjalan. **Wali Kelas tetap menjadi pemantau/koordinator** dan menerima informasi koordinasi/perkembangan yang diperlukan untuk siswa binaannya setelah penanggung jawab aktif beralih. Kedua fungsi ini berbeda dan tidak boleh digabung menjadi satu pemilik. Perubahan tahap/rujukan dicatat append-only. Status hanya Perlu tindak lanjut atau Selesai; tidak ada approval digital berlapis.

Siswa yang datang langsung ke BK memperoleh bantuan awal; Wali Kelas menerima informasi yang diperlukan untuk koordinasi tanpa rincian konseling pribadi. Jalur ini bukan bypass rutin atas tahap formal Wali Kelas. Prosedur darurat sekolah didahulukan dan aplikasi tidak boleh menghambat pertolongan. Tata cara pencatatan intake langsung dan keadaan darurat harus mengikuti keputusan/prosedur sekolah yang berlaku; implementasi tidak boleh menjadikan formulir atau persetujuan sebagai prasyarat bantuan.

**Batas rancangan implementasi:** model menyimpan tahap dan satu penanggung jawab aktif pada kasus, terpisah dari Wali Kelas pemantau. Service mengunci/memeriksa versi kasus, memvalidasi transisi normal secara berurutan, mengubah penanggung jawab, dan menulis histori append-only dalam satu transaksi agar dua rujukan bersamaan tidak membuat jalur paralel. Migration membatasi nilai tahap/status dan referensi aktor; UI hanya menawarkan tindakan tahap berikut yang sah, sementara server tetap menolak request bypass. Intake langsung BK dicatat tanpa mengubahnya menjadi transisi rutin yang melewati Wali Kelas. Informasi koordinasi untuk Wali Kelas harus dipisahkan dari isi konseling pribadi dan dibatasi pada kebutuhan koordinasi; D-044 belum menentukan daftar field/rincian yang boleh dibagikan. Pembinaan awal Wali Kelas adalah entri domain kasus baru, bukan mutasi otomatis ke `student_guidance_records` existing. Hak akses dan histori record existing tetap mengikuti D-042.

**Status setelah D-045:** hak pemantauan Wali Kelas aktif, pemilihan Guru BK aktif pada rujukan, baca ringkasan oleh Kepala Sekolah, akses catatan konseling hanya untuk BK penanggung jawab aktif, batas Kesiswaan, serta larangan akses otomatis Admin/role lain ditetapkan FINAL. Penanggung jawab aktif dapat menyelesaikan atau membuka kembali kasus dengan alasan dan histori; hanya penanggung jawab aktif yang dapat melakukan perubahan status. Penutupan tidak memerlukan approval. Tata cara formal pencatatan intake BK langsung/darurat serta penugasan ulang di luar alur normal masih perlu dirancang tanpa menahan bantuan awal/darurat.

D-045 memperjelas pembacaan D-018: hak Kepala Sekolah terhadap ringkasan tidak mencakup isi konseling pribadi. Catatan konseling dipisahkan dari ringkasan koordinasi dan `student_guidance_records` existing. Hak akses Catatan Pembinaan Kesiswaan existing tetap mengikuti D-042.

### Handoff penanganan tahap Wali Kelas — D-046 (FINAL, 30 Oktober 2026)

Jika siswa berpindah kelas atau penugasan Wali berubah ketika kasus masih terbuka di tahap Wali Kelas, Wali Kelas baru yang dapat diverifikasi melalui penempatan dan penugasan aktif boleh mengambil alih secara eksplisit bila owner lama tidak lagi menjadi Wali aktif siswa. Pengambilalihan mencatat owner lama/baru, pelaku, waktu, alasan, dan histori append-only dalam transaksi atomik; tidak ada approval, pengalihan otomatis, atau mutasi dari GET. Akses mantan Wali tidak bertahan berdasarkan histori. Handoff tidak mengubah owner kasus pada tahap BK/Kesiswaan dan tidak berlaku untuk kasus Selesai. Ketidakjelasan cakupan, versi stale, dan pengulangan ditolak tanpa mutasi. Lihat [D-046](docs/DECISIONS.md#d-046--pengambilalihan-penanganan-kasus-akibat-perubahan-wali-kelas-30-oktober-2026).

## 4. Master Data — FINAL inti

- **Siswa:** ID internal permanen; nama, tanggal lahir, jenis kelamin wajib; NIS/NISN opsional, bukan primary key. Jika NIS diisi, unik di sekolah; NISN tidak boleh digunakan dua siswa berbeda. Koreksi identitas tidak memutus riwayat.
- **Guru:** ID permanen, nama, status aktif; NIP/NUPTK/email/telepon opsional. Status akun, status kepegawaian, dan penugasan berbeda. Guru nonaktif tidak menerima penugasan baru; penugasan lama tetap historis.
- **Mata pelajaran:** ID permanen, kode unik sekolah, nama, status. Kode historis tidak dialihkan kepada mapel lain.
- Admin mengelola identitas siswa/guru, akun, role, kelas dan penempatan awal. Kurikulum mengelola kalender dan penugasan akademik; keduanya tidak memerlukan persetujuan digital satu sama lain untuk operasi normal.

**AC:** siswa tanpa NIS/NISN dapat disimpan; NIS duplikat ditolak; guru nonaktif tidak bisa diberi penugasan baru; ubah nama mapel tidak menghapus riwayat.

## 5. Tahun Ajaran, Semester, Kelas, Penempatan — FINAL inti

- Kurikulum menyiapkan tahun ajaran beserta dua semester dan mengaktifkan tahun/semester. Admin membuat/mengoreksi kelas pada tahun yang sudah disiapkan serta melakukan penempatan awal siswa. Kurikulum dapat membaca kelas/penempatan awal untuk penugasan dan jadwal.
- Satu tahun ajaran aktif. Setiap tahun ajaran memiliki dua semester berurutan; maksimal satu semester aktif.
- Kelas memiliki ID stabil, tingkat, nama, tahun ajaran; nama unik pada tingkat+tahun yang sama. Nama boleh dipakai lagi di tahun berikutnya dengan ID baru.
- Kelas dapat dibuat sebelum wali kelas ditetapkan; sebelum dipakai secara operasional tampilkan pemeriksaan kesiapan.
- Satu penempatan normal siswa per tahun ajaran. Perpindahan tengah tahun memakai catatan perpindahan khusus dan riwayat, bukan mengedit penempatan lama.
- **Transisi akhir tahun:** Kurikulum membuat tahun ajaran tujuan beserta dua semester (tetap belum aktif); Admin kemudian membuat kelas tujuan. Saat tahun sumber masih aktif dan tujuan belum aktif, Kurikulum meninjau pratinjau per siswa dari kelas asal ke hasil/kelas tujuan: naik (tingkat berikutnya), tidak naik (tingkat sama), lulus (kelas 9), atau keluar. Setelah batch transisi berhasil, Kurikulum mengaktifkan tahun dan semester baru. Tidak ada approval digital Kepala Sekolah; seluruh batch atomik dan historis. Urutan ini diperlukan karena sumber transisi wajib tahun aktif.
- **Perpindahan tengah tahun:** Kurikulum dapat memindahkan siswa langsung ke kelas lain dalam tahun ajaran yang sama, dengan alasan dan histori; tidak memerlukan persetujuan Kepala Sekolah. Perpindahan tidak mengubah identitas atau menghapus riwayat penempatan.
- Perpindahan tengah tahun tidak memerlukan persetujuan Kepala Sekolah. Fitur perpindahan tengah tahun tersedia sesuai D-031; Kurikulum mencatat tanggal berlaku, alasan dan histori.

**AC:** tahun/semester aktif ganda ditolak; penempatan normal ganda ditolak; pratinjau transisi menampilkan perubahan; kegagalan batch tidak meninggalkan hasil parsial; perpindahan menjaga riwayat.

### Perpindahan siswa — D-031, disetujui 23 Oktober 2026
- **Pindah kelas:** Kurikulum memilih siswa/penempatan pada tahun aktif → kelas tujuan berbeda dalam tahun/tingkat sama → tanggal berlaku dan alasan → ringkasan → konfirmasi Simpan. Tidak memerlukan approval Kepala Sekolah. Penempatan awal tidak ditulis ulang; catatan perpindahan bertanggal menentukan kelas aktif. Wali kelas membaca roster yang sudah memperhitungkan perpindahan berlaku.
- **Pindah sekolah keluar:** Admin memilih siswa → tanggal keluar, alasan wajib, sekolah tujuan opsional → ringkasan → konfirmasi Simpan. Efektif pada tanggal tersebut (WIB), status menjadi Pindah sekolah dan siswa tidak masuk roster/daftar penempatan aktif maupun calon kenaikan. Catatan terjadwal terlihat terpisah dari nonaktif umum. Identitas dan seluruh histori tetap tersimpan.
- Catatan menyimpan kelas asal/tujuan bila ada, alasan, tanggal berlaku, petugas dan waktu pencatatan. Tidak ada penghapusan atau penyuntingan penempatan awal. Penyimpanan atomik, otorisasi di server, perubahan bersamaan atau konfirmasi ganda ditolak.
- Perpindahan kelas dibatasi berurutan; tanggal harus setelah catatan perpindahan terakhir dan hanya satu perpindahan kelas mendatang yang belum berlaku. Kelas sumber yang berubah sejak form dibuka ditolak. Pindah kelas selama catatan keluar belum ditutup oleh penerimaan kembali yang sudah berlaku (termasuk keluar/kembali terjadwal) ditolak agar kelas asal pada histori keluar tetap konsisten. Susun perpindahan kelas terlebih dahulu sebelum mencatat keluar. Keluar bertanggal sebelum/sama dengan perpindahan kelas tercatat ditolak untuk menjaga urutan; periksa histori sebelum mencatat.
- Catatan pindah sekolah yang belum ditutup penerimaan kembali efektif (termasuk keluar/kembali terjadwal) menahan penempatan baru dan tidak ditawarkan pada transisi akhir tahun. Transisi ditahan jika ada perpindahan kelas mendatang yang belum berlaku. Perpindahan/keluar yang bertentangan dengan transisi tahun sumber yang sudah dijalankan ditolak. Penerimaan kembali siswa lama kini tersedia sesuai D-032. Koreksi/pembatalan kejadian dan penerimaan siswa pindahan baru dari sekolah lain tetap belum termasuk alur ini.
- Daftar aktif/jumlah siswa/roster mengikuti tanggal WIB melalui pembacaan database, tanpa scheduler atau penggantian status massal. Riwayat awal dan perpindahan tetap dapat dibaca oleh Admin/Kurikulum. Tanggal kalender tahun ajaran belum dimodelkan; petugas memastikan tanggal berlaku sesuai tahun sumber. Nilai/absensi historis nantinya harus memakai penempatan pada tanggal kejadian, bukan roster hari ini.

### Siswa diterima kembali — D-032, disetujui 23 Oktober 2026
- **Admin** mencari identitas siswa lama → Aksi Terima kembali → tanggal kembali, kelas tujuan pada tahun aktif, alasan wajib, sekolah asal terakhir opsional → ringkasan → konfirmasi Simpan. Tidak membuat identitas siswa baru dan tidak menghapus/mengubah kejadian keluar. Kurikulum membantu memastikan tingkat akademik bila belum jelas, tanpa approval digital tambahan.
- Pada tahun yang sama dengan kelas asal keluar, tujuan wajib tingkat yang sama (misalnya 7A → keluar → kembali di 7D). Tahun berikutnya, Admin memilih tingkat berdasarkan riwayat akademik, bukan kenaikan otomatis. Jika siswa keluar tanpa riwayat kelas asal, petugas harus memastikan tingkatnya dari bukti akademik.
- Penempatan yang sudah ada pada tahun aktif dipakai kembali, tanpa digandakan atau ditulis ulang. Penerimaan mencatat kelas efektif baru; penempatan baru hanya dibuat jika tahun aktif belum mempunyai penempatan siswa itu. Penempatan yang sudah ada dengan tingkat berbeda ditolak untuk ditinjau, bukan ditimpa.
- Kejadian keluar dan kembali mendukung siklus berulang. Satu kejadian kembali terkait tepat satu kejadian keluar, dan tanggal kembali harus sesudah tanggal keluar (minimal hari berikutnya). Keluar berikutnya harus sesudah tanggal kembali terakhir. Siswa harus sudah keluar saat pencatatan penerimaan, bukan baru memiliki rencana keluar mendatang. Pengiriman ulang, versi siswa/kejadian yang berubah, dan penerimaan ganda ditolak atomik.
- Mulai tanggal kembali (WIB), siswa aktif dan masuk kelas tujuan. Kembali terjadwal ditampilkan dengan tanggal, tetapi belum masuk roster/hitungan aktif sebelum tanggalnya. Masa di luar sekolah tetap menjadi jeda: penempatan pada tanggal dalam jeda tidak menghasilkan kelas. Identitas, penempatan asli, seluruh kejadian dan petugas/waktu pencatatan tetap dapat ditelusuri.
- Catatan keluar yang penerimaannya sudah berlaku tidak lagi mengunci penempatan, pindah kelas atau transisi. Selama masih keluar/menunggu kembali, batasan tetap berlaku. Transisi akhir tahun menunggu penerimaan terjadwal pada tahun sumber; penerimaan pada tahun yang sudah dijalankan batch transisinya ditahan sampai Kurikulum mengaktifkan tahun tujuan agar siswa tidak terlewat. Edit identitas saat masa keluar tidak boleh membatalkan aktivasi kembali terjadwal.
- Ini alur penerimaan kembali siswa lama. Pembatalan keluar yang belum terjadi, koreksi tanggal/kejadian dan penerimaan siswa pindahan yang belum pernah terdaftar tetap merupakan alur terpisah yang belum tersedia.

## 6. Penugasan dan Jadwal — FINAL inti

- Kurikulum menetapkan Guru Mapel pada kelas-mapel dan Wali Kelas pada kelas sesuai keputusan sekolah; hak langsung berlaku.
- **Keputusan 22 Oktober 2026 — cakupan pengampu v1:** penugasan Guru Mapel berlaku per semester, dengan satu pengampu resmi pada satu kombinasi semester–kelas–mata pelajaran untuk periode berlaku yang sama. Satu guru boleh mengajar banyak kelas dan mata pelajaran; mata pelajaran yang sama boleh diajar guru berbeda di kelas berbeda. Contoh: Pak Imam mengajar Bahasa Inggris di 7A/7B/7C, sedangkan Pak Budi mengajar Bahasa Inggris di 7D/7E serta kelas 8/9 yang ditugaskan kepadanya. Penggantian resmi di tengah semester tetap boleh dengan tanggal efektif dan histori. Semester berikutnya dapat menyalin penugasan sebagai bahan awal lalu menyesuaikannya.
- Kurikulum menyusun jam pelajaran, jadwal draf/publikasi, dan perubahan jadwal. Benturan guru/kelas/jam dicegah.
- **Pola jadwal v1:** Kurikulum mengatur jam pelajaran untuk setiap hari (Senin, Selasa, dan seterusnya), termasuk nomor jam serta waktu mulai dan selesai; jadwal memilih rentang nomor jam pada hari itu, misalnya Senin jam 1–2, PAI, Pak Imam, 7A. Jadwal mingguan berulang berlaku untuk semester terkait, memiliki tanggal mulai berlaku serta status Draf/Publikasi. Guru melihat jadwal yang sudah dipublikasikan sesuai penugasannya.
- **Template jam disetujui 22 Oktober 2026:** aplikasi menyediakan Senin–Sabtu dengan nomor jam 1–12 per hari. Kurikulum mengisi pasangan waktu mulai/selesai pada baris yang digunakan dan menyimpan satu hari sekaligus. Jam kosong tidak wajib diisi, tidak dibuat di database, dan tidak menghapus definisi lama. Waktu antarhari boleh berbeda. Definisi lama untuk Minggu atau nomor di atas 12 tetap dapat dibaca/diedit; template bukan pembatas baru skema.
- **Durasi dan mulok disetujui 22 Oktober 2026:** JP pembelajaran diseragamkan menjadi 40 menit; Bahasa Indramayu dan Ke-NU-an masing-masing mendapat 2 JP per minggu per rombel. Slot upacara Senin bukan JP pembelajaran; pembelajaran Senin dimulai jam ke-2 pukul 07.00. Konfigurasi waktu semester aktif dicatat pada [HANDOVER](docs/HANDOVER.md#penyeragaman-jp-dan-pengisian-jam--22-oktober-2026), dengan alasan/status penerapan pada [D-022](docs/DECISIONS.md#d-022--durasi-jp-dan-alokasi-mulok-disetujui-22-oktober-2026). Ini acuan operasional sekolah pada saat D-022; target JP dan pengecualian upacara kini diperiksa pada draf berkonfigurasi D-025 sesuai bab 6.1.
- **Pembagian JP untuk draf 2026/2027 Semester 1:** Bahasa Indonesia 6 JP (3+3); Matematika dan IPA masing-masing 5 JP (3+2); IPS dan Bahasa Inggris masing-masing 4 JP (2+2). PAI, Pendidikan Pancasila, PJOK, Informatika, dan Seni Rupa masing-masing 3 JP dalam satu pertemuan; Bahasa Indramayu dan Ke-NU-an masing-masing 2 JP dalam satu pertemuan. Dua pertemuan mapel yang sama ditempatkan pada hari berbeda. Total per rombel 43 JP/17 pertemuan, dengan kapasitas Senin 9 JP (jam 2–10), Selasa 10 JP, Rabu/Kamis masing-masing 9 JP, Jumat 6 JP; tanpa pembelajaran pada upacara Senin. Draf awal enam rombel sudah diisi menurut pembagian ini; lihat [D-023](docs/DECISIONS.md#d-023--pengisian-draf-jadwal-enam-rombel-22-oktober-2026). Ini konfigurasi draf sekolah, bukan fitur generator atau validasi otomatis target JP.
- **Pengisian jadwal sederhana:** pilih kelas → mapel yang sudah ditugaskan pada tanggal berlaku draf → guru otomatis mengikuti penugasan → hari → rentang jam yang sudah memiliki waktu pada hari itu. Server menentukan guru dari penugasan dan tetap menolak kombinasi tidak sah/bentrokan. Template ini bukan generator jadwal otomatis dan tidak memakai impor aSc.
- **Alur yang sudah tersedia:** tetapkan pengampu → susun jadwal secara manual → validasi → publikasikan. Sistem menolak guru mengajar dua kelas pada waktu bersamaan, satu kelas menerima dua pelajaran pada waktu bersamaan, serta jadwal yang tidak sesuai penugasan guru–kelas–mapel. Validasi benturan memakai waktu aktual, bukan hanya kesamaan nomor jam. Generator dan pengaturan fleksibel tersedia sebagai pengembangan bab 6.1; alur manual tetap didukung.
- Jadwal = rencana, jurnal = kegiatan aktual. Perubahan jadwal tidak mengubah jurnal masa lalu.
- **FINAL: Tidak ada Guru Pengganti atau Guru Cadangan.** Ketidakhadiran guru tidak menghasilkan penugasan pengganti, pemberian akses sementara, atau jurnal atas nama guru lain.
- **Perubahan guru pengampu resmi tetap didukung:** Kurikulum memperbarui penugasan sesuai keputusan sekolah, dengan tanggal efektif dan riwayat pengampu lama; Kurikulum menyesuaikan jadwal untuk periode mendatang. Ini bukan penugasan sementara.
- Pertemuan yang tidak terlaksana tetap dapat terlihat sebagai jadwal tanpa jurnal Final; jangan otomatis menganggap telah diajar atau mencatat guru lain. Detail status kegiatan tidak terlaksana **OPEN P1** dan tidak boleh mengubah status absensi siswa secara otomatis.
- **Pengampu bersama ditunda dari cakupan v1** sesuai keputusan 22 Oktober 2026, menggantikan ketentuan sebelumnya yang mengizinkannya. Jika kelak dibutuhkan, tetapkan pembagian tanggung jawab dan hak nilai akhir sebelum mengaktifkannya; keputusan itu tidak menghalangi pengampu tunggal v1.

**AC:** satu pengampu per semester–kelas–mapel pada periode berlaku yang sama; guru dapat ditugaskan pada banyak kelas/mapel; benturan waktu guru/kelas dan jadwal di luar penugasan ditolak; Guru Mapel tanpa penugasan ditolak; draf tidak tampil sebagai jadwal terpublikasi kepada guru; tidak ada pengganti/cadangan atau hak akses sementara; perubahan pengampu/jadwal bertanggal efektif menyimpan histori dan tidak menimpa catatan masa lalu.

### 6.1 Rancangan pengembangan jadwal fleksibel dan generator

**Status 22 Oktober 2026: inti pengaturan dan generator telah diimplementasikan sesuai persetujuan lanjutan; lihat D-025 pada DECISIONS dan bukti di HANDOVER.** Lihat [D-024](docs/DECISIONS.md#d-024--rancangan-jadwal-fleksibel-per-rombel-dan-generator-22-oktober-2026). Pengaturan berikut menjadi sumber aturan, sehingga angka JP/pola pertemuan tidak dikunci di source code. Alokasi bab 6 untuk draf 2026/2027 adalah konfigurasi sekolah saat itu, bukan batas permanen bagi semester atau rombel lain.

**Struktur Kurikulum — kewenangan Kurikulum:**

- Pilih tahun ajaran/semester, lalu atur mapel, JP pembelajaran per minggu, dan pembagian JP tiap pertemuan sebagai bawaan tingkat VII/VIII/IX. Jumlah pembagian pertemuan harus sama dengan target JP; kebijakan hari berbeda untuk pertemuan mapel yang sama dicatat sebagai pengaturan, bukan disimpulkan dari jumlah JP.
- Rombel mewarisi bawaan tingkat dan boleh memiliki pengecualian per mapel. Aturan khusus rombel mengalahkan bawaan tingkat; tanpa pengecualian, bawaan tingkat berlaku. Tampilkan sumber nilai dengan jelas dan sediakan kembali ke bawaan. Pengaturan yang belum lengkap harus ditandai, bukan ditebak oleh generator.
- Contoh semester yang sama: bawaan Matematika VII 5 JP (3+2), 7A pondok dikecualikan menjadi 2 JP (satu pertemuan), sedangkan 7B reguler tetap 5 JP. Semester berikutnya boleh menjadi 8 JP (4+4) berdasarkan keputusan Kurikulum. Label pondok/reguler pada contoh tidak mengharuskan penambahan jenis kelas atau perubahan identitas rombel Admin.
- Pengaturan semester sebelumnya dapat disalin sebagai bahan awal, ditinjau dan disesuaikan; tidak diberlakukan otomatis. Perubahan menyimpan actor, waktu, nilai sebelum/sesudah dan versi/tanggal berlaku. Pemeriksaan memakai aturan efektif masing-masing rombel–mapel.

**Jam sekolah, istirahat, dan upacara:**

- Kurikulum menyiapkan hari, nomor jam, waktu mulai/selesai, serta jenis slot: pembelajaran, istirahat, atau kegiatan non-mapel seperti upacara. Waktu bisa berbeda antarhari. Definisi jam dan aturan yang dipakai versi jadwal harus tetap dapat ditelusuri.
- Istirahat dicatat eksplisit dengan hari dan waktu mulai/selesai, terlihat sebagai **Istirahat**, serta memblokir pembelajaran bagi rombel yang tercakup. Istirahat tidak menambah JP mapel; jangan menampilkan rentang belajar seolah berlangsung selama istirahat. Implementasi menampilkan istirahat sebagai interval tersendiri tanpa nomor jam pembelajaran; kegiatan seperti upacara tetap memiliki nomor jam.
- **Upacara Senin tetap menempati jam ke-1**, ditampilkan berlabel **Upacara**, bukan sel kosong. Jam ke-1 ini terisi kegiatan sekolah dan tidak dapat dipakai mapel; pembelajaran mulai jam ke-2. Upacara bukan JP mapel dan tidak memerlukan guru pengampu/mapel fiktif untuk dapat ditampilkan.
- Generator, edit manual, dan publikasi harus memeriksa benturan waktu aktual dengan istirahat/upacara, selain bentrok guru/kelas. Pengguna telah menyetujui satu pertemuan dilanjutkan sesudah istirahat. Tampilkan segmen belajar terpisah tanpa menghitung atau mengisi jeda istirahat; pola pertemuan tetap dihitung sebagai satu pertemuan sesuai JP-nya.

**Alur Kurikulum:**

1. Pilih semester dan tanggal mulai berlaku jadwal.
2. Siapkan Struktur Kurikulum bawaan tingkat dan pengecualian rombel; periksa ringkasan JP/pertemuan efektif setiap kelas–mapel.
3. Periksa penugasan guru–kelas–mapel yang telah tersedia. Generator memakai pengampu resmi sesuai tanggal berlaku, tanpa memilih guru baru atau mengubah penugasan secara diam-diam.
4. Atur jam sekolah, istirahat dan kegiatan Upacara pada jam ke-1. Batas khusus guru/mapel dan preferensi diatur melalui Batasan Jadwal sesuai bab 6.2.
5. Klik **Generate jadwal**. Sistem memeriksa kelengkapan pengaturan, kapasitas JP rombel dan beban waktu guru, lalu mencari susunan yang memenuhi semua aturan efektif. Pengecekan kapasitas awal saja tidak membuktikan solusi pasti ada.
6. Hasil lengkap yang valid disimpan sebagai **Draf** beserta acuan versi pengaturan dan ringkasan JP/pertemuan. Jika masih ada draf, minta pilihan yang jelas sebelum mengganti isinya. Jadwal terpublikasi tetap utuh. Penyimpanan draf hasil generate harus atomik, sehingga kegagalan tidak meninggalkan sebagian hasil.
7. Kurikulum meninjau per hari/kelas/mapel/guru dan boleh mengedit draf dengan validasi yang sama. Tampilkan target versus JP terjadwal serta masalah yang harus diperbaiki. Perubahan pengaturan setelah generate menandai draf perlu diperiksa ulang.
8. Klik **Publikasikan** setelah pemeriksaan ulang server: target JP dan pembagian pertemuan tepat, penugasan sesuai, tidak bentrok guru/kelas, serta tidak mengisi istirahat/upacara dengan mapel. Simpan snapshot aturan/jam/kegiatan yang dipakai agar perubahan berikutnya tidak menulis ulang versi terbit atau jurnal masa lalu. Guru melihat versi yang sudah dipublikasikan sesuai aksesnya.

**Bila generate belum berhasil:** tampilkan pengaturan yang kurang atau kendala yang dapat dibuktikan, misalnya beban guru melebihi waktu tersedia. Bedakan hasil terbukti tidak memungkinkan dengan pencarian yang berhenti karena batas waktu. Jangan mengurangi JP, mengganti guru, mengisi istirahat/upacara, atau memublikasikan hasil tidak lengkap untuk memaksakan hasil. Pengguna menyesuaikan pengaturan lalu mencoba kembali.

**Implementasi dan batas saat ini:** migration aditif 00010 menyimpan aturan, pengecualian, istirahat/kegiatan, histori konfigurasi dan snapshot rencana pada revisi. Solver Go memakai pencarian dengan budget 8 detik/batas simpul, maksimal 1.200 pertemuan, tanpa dependency baru; susunan versi terbit diprioritaskan hanya bila masih sesuai aturan baru. Batasan individu guru/mapel dan preferensi kini tersedia melalui migration 00011 sesuai bab 6.2; tanpa batasan tambahan guru tetap tersedia pada jam sekolah. Generate mengganti draf berisi pelajaran hanya setelah konfirmasi eksplisit dan menyimpan histori; versi terbit tidak diturunkan menjadi draf. Data lama tetap utuh. Generator bukan integrasi aSc dan tidak mengaktifkan pengampu bersama.

**Kriteria penerimaan pengembangan:** 7A dapat 2 JP sementara 7B 5 JP untuk mapel/semester sama; semester baru dapat 8 JP tanpa perubahan kode; pewarisan/pengecualian terlihat; pembagian pertemuan konsisten; istirahat terblokir; Upacara terlihat pada Senin jam ke-1; generator menghasilkan draf lengkap atau menjelaskan kegagalan secara jujur; edit/publikasi menerapkan aturan efektif yang sama; data dan histori jadwal lama tetap utuh.

### 6.2 Rancangan batasan jadwal dan preferensi

**Status 22 Oktober 2026: diterapkan setelah persetujuan pengguna, fase 7B/migration 00011.** Keputusan pada [D-026](docs/DECISIONS.md#d-026--batasan-wajib-dan-preferensi-jadwal-22-oktober-2026). Ini melengkapi generator bab 6.1 dengan pengaturan, validator wajib, kapasitas guru, pencarian preferensi dan laporan revisi. Pilihan implementasi pada D-027.

**Format rentang waktu fase 7B (editor utama kini kisi pada bab 6.3), per semester:** sediakan menu Batasan Jadwal dengan sasaran Guru atau Mata Pelajaran, hari yang dipilih, waktu mulai/selesai atau sepanjang hari, serta jenis ketentuan berikut. Hari dipilih secara eksplisit; permintaan menghindari pagi tidak otomatis berlaku setiap hari. Angka jam/hari/nama mapel dan guru tidak dikunci di source code.

| Jenis | Arti | Contoh | Jika jadwal tidak dapat disusun |
|---|---|---|---|
| **Tidak tersedia** | Larangan wajib; tidak boleh ditempati oleh sasaran tersebut | PJOK tidak boleh pada waktu siang yang ditetapkan tiap hari; guru benar-benar tidak dapat hadir pada Jumat | Jelaskan kendala; jangan melanggar larangan atau mengurangi JP untuk memaksakan hasil |
| **Usahakan dihindari** | Preferensi; boleh ditempati untuk memperoleh jadwal lengkap | Guru mengusahakan tidak masuk pagi, tetapi bersedia datang pagi pada sebagian hari jika diperlukan | Cari hasil dengan sesedikit mungkin hari pagi; tampilkan preferensi yang belum terpenuhi pada hasil draf |

Kurikulum menetapkan jenisnya berdasarkan kondisi sebenarnya. Permintaan “ingin tidak mengajar Jumat” dapat menjadi preferensi; “benar-benar tidak bisa hadir Jumat” menjadi Tidak tersedia. Preferensi tidak boleh membatalkan larangan wajib, bentrok, target JP/pola pertemuan, penugasan resmi, istirahat, atau Upacara. Tanpa pengaturan khusus, waktu sekolah yang tersedia tetap berlaku.

**Waktu aktual per hari:** gunakan rentang waktu nyata untuk menghindari pembelajaran siang, bukan menganggap jam ke-7 selalu siang. Audit 22 Oktober menunjukkan Senin jam 7 masih 10.50–11.30, sedangkan Selasa jam 7 sudah 12.25–13.05 dan masih ada jam 10. Contoh konfigurasi PJOK: tidak tersedia mulai 12.25 sampai akhir pembelajaran pada hari yang dipilih, mengikuti waktu sekolah yang berlaku. Pukul 12.25 adalah contoh berdasarkan konfigurasi sekarang, bukan konstanta atau asumsi permanen waktu Dzuhur. Jika jam sekolah/istirahat berubah, batasan perlu ditinjau. Setiap JP dalam satu pertemuan harus berada pada waktu yang diizinkan; izin menyambung sesudah istirahat tidak boleh dipakai untuk menyisipkan pembelajaran pada waktu terlarang.

**Perilaku pencarian yang direncanakan:**

1. Tetap penuhi seluruh ketentuan wajib: target JP dan pola efektif rombel, pengampu, hari pertemuan, jam sekolah, istirahat/Upacara, bentrok guru/kelas, serta semua batas Tidak tersedia.
2. Usahakan seluruh preferensi terpenuhi. Untuk preferensi menghindari pagi, tujuan yang disepakati adalah meminimalkan **jumlah hari guru tetap masuk pagi**, bukan hanya jumlah JP pagi. Beberapa JP pagi pada satu hari dihitung satu hari untuk tujuan tersebut; laporkan pula JP-nya.
3. Jika preferensi tidak seluruhnya dapat terpenuhi, hasil lengkap yang memenuhi ketentuan wajib boleh menjadi draf dengan laporan preferensi yang belum terpenuhi. Semua JP tetap harus terjadwal. Preferensi yang belum terpenuhi bukan pelanggaran larangan wajib.
4. Pilihan teknis yang masih berupa rekomendasi: setelah meminimalkan hari preferensi guru yang tidak terpenuhi, kurangi jumlah JP yang mengenai preferensi, lalu gunakan posisi versi terbit sebagai preferensi terakhir untuk stabilitas. Rincian penilaian antarpreferensi lain, pemerataan antar-guru dan strategi pencarian perlu dirinci sebelum implementasi; belum ada bobot khusus yang disetujui.
5. Pencarian yang dibatasi waktu boleh menyajikan jadwal lengkap valid dengan label **hasil terbaik yang ditemukan**, tanpa menjamin minimum/optimal. Jangan menyebut masuk pagi sebagai “pasti terpaksa” jika belum terbukti semua alternatif lebih baik tidak mungkin. Bedakan hasil valid dengan preferensi belum terpenuhi, kendala wajib yang terbukti, serta timeout tanpa hasil lengkap. Tidak memublikasikan otomatis atau menulis sebagian hasil.

**Pemeriksaan kapasitas:** hitung waktu tersedia setiap guru setelah larangan wajib, bukan hanya total slot sekolah. Waktu yang hanya diminta dihindari tetap boleh dihitung sebagai kapasitas, tetapi diberi konsekuensi preferensi. Contoh audit: sekolah 43 JP; larangan wajib jam 1–2 setiap hari menutup 9 JP belajar (Senin jam 1 adalah Upacara), menyisakan 34 JP. Guru berbeban 36 JP tidak mungkin memenuhinya dengan larangan wajib itu. Bila pagi hanya preferensi, sedikitnya 2 JP perlu ditempatkan pada waktu yang dihindari dalam contoh tersebut; hari dan penempatannya masih bergantung pada pola pertemuan/bentrok. Jika hanya beberapa hari dipilih, hitung ulang sesuai hari itu. Pemeriksaan kapasitas cukup untuk menemukan sebagian kendala, bukan bukti bahwa seluruh kombinasi pasti mempunyai solusi.

**Alur Kurikulum setelah pengembangan:**

1. Pilih semester/tanggal berlaku; periksa struktur JP per rombel dan guru pengampu.
2. Periksa jam sekolah, istirahat, dan Upacara pada jam ke-1.
3. Isi Batasan Jadwal: pilih guru/mapel → hari → waktu aktual atau sepanjang hari → Tidak tersedia/Usahakan dihindari. Tidak perlu membuat baris bagi guru/mapel yang tidak memiliki batasan.
4. Generate: cek kesiapan dan kapasitas berdasarkan larangan wajib, kemudian cari jadwal lengkap sambil mengusahakan preferensi.
5. Tinjau draf beserta ringkasan target JP, hasil pemeriksaan wajib dan laporan preferensi. Contoh laporan berdasarkan hasil nyata: “Pak A terjadwal pagi Selasa, 2 JP; preferensi menghindari pagi belum seluruhnya terpenuhi.” Jangan memberi alasan sebab-akibat yang tidak dibuktikan oleh pencarian.
6. Kurikulum boleh mengedit draf atau mengubah pengaturan lalu generate ulang. Edit manual menolak larangan wajib dan memperbarui laporan preferensi; penempatan pada preferensi cukup ditandai, tidak dilarang.
7. Publikasikan setelah pemeriksaan ulang server dan tinjauan laporan preferensi. Larangan wajib harus bebas pelanggaran; preferensi yang belum terpenuhi tidak menghalangi publikasi dan tetap terlihat. Versi terbit lama dipertahankan.

**Keberlanjutan data dan batas implementasi:** simpan versi/histori perubahan batasan serta snapshot larangan/preferensi dan hasil pemeriksaannya bersama revisi jadwal. Perubahan batasan membuat draf perlu diperiksa ulang; jangan mengubah versi terbit secara diam-diam. Batas khusus guru/mapel tidak boleh dibuat sebagai blok istirahat global karena akan menutup sekolah bagi semua sasaran. Diperlukan penyimpanan tambahan, pembaruan input/fingerprint solver, kapasitas per guru, validator bersama untuk generate/edit/publikasi, pencarian preferensi, serta tampilan laporan. Migration aditif 00011 menyimpan batasan serta histori; snapshot rencana dan fingerprint mencakup batasan. Sasaran mapel berlaku bagi seluruh rombel pada semester terkait; pengecualian batasan mapel per rombel belum tersedia. Satu aturan boleh memilih beberapa hari dengan waktu sama; waktu berbeda memakai aturan terpisah. Pencarian branch-and-bound terbatas mengutamakan jumlah pasangan guru–hari yang mengenai preferensi guru, kemudian jumlah JP yang mengenai preferensi apa pun (JP tumpang tindih dihitung sekali); posisi versi terbit menjadi urutan pilihan berikutnya. Tidak ada bobot/pemerataan tambahan. Jika budget habis setelah ada hasil lengkap, gunakan hasil terbaik yang sudah divalidasi; tanpa hasil lengkap draf tetap utuh. Laporan dihitung ulang dari snapshot dan entri revisi, sehingga edit memperbarui laporan dan konfigurasi baru tidak mengubah laporan versi terbit. Tidak menanam larangan PJOK/guru/Jumat dari contoh percakapan.

**Kriteria penerimaan pengembangan:** batas PJOK mengikuti waktu aktual per hari; guru dapat memiliki batas hanya pada hari terpilih atau sepanjang Jumat; larangan wajib tidak pernah dilanggar; preferensi pagi boleh tidak terpenuhi untuk mencapai JP lengkap dan hari/JP-nya dilaporkan; slot preferensi tidak dianggap hilang dalam pemeriksaan kapasitas; edit/publikasi memeriksa aturan yang sama; timeout tidak diklaim sebagai bukti tidak mungkin/optimal; histori, snapshot, serta versi terbit tetap utuh.

### 6.3 Kisi ketersediaan guru dan mapel — diterapkan 22 Oktober 2026
Pengguna menyetujui editor hari × nomor jam sebagai alur utama Batasan Jadwal (D-028), menggantikan pengisian rentang waktu satu per satu pada antarmuka bab 6.2. Aturan wajib/preferensi, JP lengkap, larangan bentrok, istirahat/kegiatan, snapshot dan publikasi tetap berlaku.

Alur: pilih semester → Guru atau Mata Pelajaran → nama → **Atur kisi** → pilih status lalu klik sel jam atau tombol satu hari → **Simpan kisi** → generate ulang draf → tinjau → publikasikan. Status **Tersedia** boleh digunakan bila aturan lain mengizinkan; **Kondisional** berarti preferensi Usahakan dihindari dan boleh digunakan agar JP lengkap; **Tidak tersedia** larangan wajib. Tersedia pada guru tidak mengalahkan Tidak tersedia pada mapel, maupun sebaliknya. Mapel berlaku seluruh rombel semester terkait.

Kisi menampilkan Senin–Sabtu dan jam 1–12, dengan tambahan Minggu/nomor lebih tinggi jika data lama memerlukannya. Tiap sel menampilkan waktu aktual hari tersebut, simbol dan label aksesibel. Klik biasa/keyboard dan sentuhan ponsel didukung. Tombol hari menerapkan status terpilih ke seluruh slot pembelajaran hari itu; Semua Tersedia mereset pilihan sasaran dalam editor, baru berlaku setelah Simpan. Slot tanpa waktu dan kegiatan seperti Upacara terkunci. Istirahat bukan slot pembelajaran dan tidak dapat dibuka melalui kisi. Matriks formulir tidak dipaginasi; daftar kisi tetap bernomor dan 10 baris/halaman.

**Integritas data dan perubahan waktu:** migration aditif 00012 menyimpan satu kisi per semester–sasaran, versi optimistic, snapshot jam/blok dan histori sebelum/sesudah. Rentang batasan lama tetap disimpan dan berlaku selama sasaran belum mempunyai kisi. Editor memetakan batasan waktu lama ke JP yang beririsan, dengan larangan wajib mengalahkan preferensi; pengguna meninjau dan mencentang persetujuan sebelum menyimpan. Setelah disimpan, kisi menjadi aturan aktif sasaran tersebut dan catatan waktu lama dipertahankan. Reset Tersedia tidak menghidupkan kembali batasan waktu lama. Tidak memigrasikan/mengonversi pilihan pengguna otomatis pada deployment.

Pilihan kisi mengikuti nomor jam pada hari yang dipilih. Jika definisi waktu/jeda/kegiatan sekolah berubah, generate/edit/publikasi meminta tinjauan dan penyimpanan ulang kisi terkait sebelum memakai perubahan itu. Sel baru ditampilkan Tersedia untuk ditinjau; tidak otomatis mengizinkan perubahan tanpa konfirmasi. Ini menjaga kesadaran bahwa nomor jam yang sama dapat bergeser dari pagi ke siang. Token formulir mencakup konfigurasi jam/blok dan aturan sasaran, diperiksa di bawah mutation lock; perubahan serentak ditolak tanpa menimpa. Simpan semua sel/histori atomik dan server menolak status ilegal, sel duplikat/hilang, serta slot terkunci. Tidak mengubah snapshot, hasil laporan atau status versi terbit lama.

### 6.4 Impor jadwal XML aSc — 23 Oktober 2026
Pengguna mengonfirmasi sampel ekspor terpotong karena batas lisensi dan meminta fitur impor diterapkan (D-029). Kurikulum dapat memilih **Jadwal Mingguan → Impor XML**, mengunggah aSc Timetables 2012 XML, mencocokkan referensi, memeriksa pratinjau, kemudian menyimpan sebagai draf. Publikasi tetap manual melalui validator jadwal yang sudah ada. Impor tidak membuat atau menimpa akun, role, identitas, kelas, mapel, penugasan guru, definisi jam, struktur JP, kisi ketersediaan atau versi jadwal terbit.

**Pemetaan eksplisit:** pilih semester/tanggal berlaku; cocokkan kelas, guru, mapel, hari, serta nomor jam sumber per hari ke jam tujuan SIM-GURU. Nama yang sama persis setelah normalisasi kapital/spasi menjadi saran hanya bila unik; ID acak aSc tidak dianggap ID aplikasi. Hari memakai definisi/nama hari di XML, bukan menganggap kode biner sebagai angka hari; yang tidak dikenali harus dipilih pengguna. Jam tujuan hanya slot pembelajaran hari itu, tanpa kegiatan/istirahat. Seluruh JP satu pertemuan harus dipetakan ke rentang nomor berurutan. Perbedaan waktu XML–sekolah ditampilkan dan memerlukan persetujuan memakai waktu sekolah; aplikasi tidak mengubah jam sekolah dari XML.

**Kelengkapan:** format yang diuji mencatat setiap JP pada `cards`, sedangkan `periodspercard` menentukan panjang pertemuan; JP berurutan dikelompokkan sesuai panjangnya, tanpa menggandakan durasi. Kekurangan/kelebihan penempatan dibanding `periodsperweek`, blok terpotong, slot duplikat dan referensi tidak dikenal merupakan masalah sumber yang menahan penyimpanan. Sampel 10 slot untuk 11 JP tidak boleh dianggap lengkap atau dilengkapi diam-diam. XML yang berisi pertemuan utuh tetapi baru mencakup sebagian target sekolah boleh disimpan sebagai **draf belum lengkap** setelah persetujuan; publikasi tetap ditolak sampai seluruh target JP/pola sekolah terpenuhi. Ini berbeda dari memaksakan blok yang terpotong.

**Validasi dan integritas:** pratinjau dan konfirmasi memeriksa pengampu aktif sesuai tanggal, JP/pola/hari pertemuan, bentrok guru/kelas, istirahat/Upacara dan larangan wajib, memakai validator bersama. Preferensi boleh belum terpenuhi dan tetap terlihat pada laporan revisi. Draf yang sudah berisi hanya diganti setelah persetujuan eksplisit dengan jumlah pertemuan yang akan diganti ditampilkan; histori sebelum/sesudah disimpan atomik. Jika pengaturan, penugasan, atau isi draf berubah setelah pratinjau, konfirmasi ditolak dan pratinjau harus diperiksa ulang. Perubahan pemetaan dari tab lain ditolak melalui versi; konfirmasi ulang tidak menggandakan hasil.

**Cakupan awal dan batas:** maksimal 5 MiB, 1.200 pertemuan, 10.000 catatan kartu/slot dan 2.000 definisi pelajaran; satu guru, satu rombel utuh, satu pola minggu/term. Pengampu bersama, kelas gabungan, kelompok/sebagian siswa, JP mingguan pecahan atau rotasi minggu/term ditandai belum didukung. Format default dengan representasi kartu berbeda harus diaudit, bukan ditebak. Data ruang, siswa dan kontak dari XML bukan sumber master atau pemeriksaan konflik ruang SIM-GURU. Parser menerima encoding yang didukung (termasuk Windows-1252 pada sampel), menolak DTD/entity directive dan tidak mengambil resource jaringan.

Migration aditif 00013 menyimpan unggahan sementara yang hanya dapat dibuka pengunggah ber-role Kurikulum, dengan token acak yang disimpan hash-nya, masa berlaku 30 menit, maksimal lima unggahan aktif, versi pratinjau dan provenance file. Kontak siswa/guru dan XML mentah tidak disimpan. Payload unggahan selesai dibersihkan; histori entri menyimpan hash file, referensi pelajaran/slot XML dan hasil tujuan. Unggahan kedaluwarsa tidak dapat dipakai dan payload dibersihkan saat pengguna mengunggah lagi. Tidak menjalankan impor contoh otomatis pada database pengguna.

## 7. Absensi — FINAL rancangan disetujui 24 Oktober 2026

**Keputusan:** D-033. Guru memakai GPS dan selfie masuk/pulang; absensi harian siswa memakai kartu QR; absensi per pelajaran dicatat guru pengampu. Ketiga sumber terpisah; kehadiran guru tidak membuktikan mengajar atau membuat jurnal, dan Hadir harian tidak mengisi tiap mapel. Kosong tetap **Belum Dicatat**, bukan Hadir/Alpa. Status siswa hanya Hadir, Sakit, Izin, Alpa, Belum Dicatat; Belum Diverifikasi bukan status keenam. Tanpa guru pengganti, poin pelanggaran, atau approval Kepala Sekolah.

### 7.1 Fondasi tanggal dan akses — urutan implementasi pertama
- Kurikulum mengatur rentang tanggal semester (inklusif), hari sekolah mingguan (Senin–Minggu dapat dipilih), serta tanggal pengecualian libur/hari sekolah khusus. Tidak menebak tanggal dari nama tahun. Rentang semester dalam satu tahun berurutan/tidak overlap. Rentang tahun sekolah mengikuti rentang semester yang dikonfigurasi, bukan sumber tanggal ganda.
- Admin menentukan tanggal siswa mulai mengikuti kelas pada penempatan awal; tanggal mengetik data bukan tanggal mulai. Penempatan lama tanpa tanggal tidak diisi otomatis. Admin melengkapinya melalui Tanggal Mulai Penempatan dengan alasan dan histori. Tanggal tidak boleh melewati kejadian perpindahan yang sudah diketahui. Penempatan baru melalui UI wajib tanggal; penempatan dari penerimaan kembali memakai tanggal kembali, transisi memakai awal kalender tujuan bila tersedia, selain itu perlu dilengkapi Admin.
- Kalender dan tanggal penempatan memakai transaksi serta optimistic version. Riwayat mencatat pelaku/waktu/alasan/sebelum/sesudah.
- Roster untuk absensi dibaca sesuai tanggal WIB, meliputi pindah kelas, keluar, dan kembali. Data tanpa tanggal yang dapat memengaruhi roster menahan pembacaan absensi sampai dilengkapi; daftar operasional lama tetap kompatibel. Tanggal sebelum mulai penempatan tidak masuk roster. Siswa baru tidak dinyatakan absen pada masa sebelum mulai.
- Jadwal menggunakan versi **published** terbaru dengan tanggal berlaku <= tanggal pertemuan; draf tidak digunakan. Hari libur tidak menghasilkan pertemuan pembelajaran. Tidak menimpa snapshot jadwal lama.
- Hak baca fondasi siswa: Petugas Absensi Siswa/Kesiswaan/Kepala Sekolah seluruh sekolah, Wali Kelas hanya kelas penugasan saat ini, Guru Mapel hanya kelas dan pelajaran sesuai penugasan efektif pada tanggal yang dipilih. Kehadiran guru dikelola melalui kewenangan D-039. Admin tanpa role operasional tidak memperoleh akses absensi; Kurikulum mengelola kalender/jadwal, bukan input kehadiran. Hak baca bukan hak tulis.
- Menu pengaturan kalender/tanggal dan helper konteks tanggal merupakan fondasi, bukan formulir pencatatan absensi. Modul pencatatan, pembekuan konteks yang sudah digunakan absensi, dan tampilan operasional dilanjutkan pada urutan berikutnya.

### 7.2 Kehadiran guru GPS dan selfie
- **D-038 dan D-039 (27–28 Oktober 2026):** Admin mengatur titik sekolah; radius awal 100 m, rentang 25–500 m. Akurasi browser maksimal 50 m dan diperiksa terpisah dari jarak terhadap radius. Server menghitung jarak Haversine. Bukti menyimpan jarak, akurasi, serta snapshot koordinat/radius sekolah, bukan koordinat perangkat mentah. Tata kelola TU dan kontrak status kehadiran mengikuti [D-039](docs/DECISIONS.md#d-039--tata-kelola-kehadiran-guru-oleh-tu-28-oktober-2026).
- Guru login dengan akun sendiri → pilih Masuk/Pulang → ambil GPS → selfie kamera langsung → kirim → bukti waktu server/WIB. Satu identitas guru berlaku untuk semua role; satu Masuk dan satu Pulang per tanggal WIB. Pulang tanpa Masuk diperbolehkan dan Masuk tetap Belum Dicatat. Guru Mapel/Wali Kelas aktif dengan akun dan profil tertaut boleh absen mandiri; guru tanpa akun tidak. Replay identik mempertahankan bukti/histori, request berbeda untuk event sama konflik.
- Foto bukan pilihan unggah dari galeri; GPS/selfie adalah bukti pendukung, tanpa klaim menjamin pencegahan titip absen atau pengenalan wajah.
- Selfie wajib masuk dan pulang. Perkecil di perangkat; server memvalidasi format dan ukuran **maksimal 100 KB per foto**. Foto privat; guru membaca bukti sendiri dan TU Pengelola Kehadiran Guru memeriksa sesuai kewenangan. Petugas Absensi Siswa tidak mendapat akses kehadiran guru/foto.
- Foto disimpan **dua bulan kalender sejak pencatatan**, lalu berkasnya dihapus otomatis. Metadata kehadiran dan histori tetap ada; detail menampilkan masa penyimpanan foto berakhir. Penghapusan mencakup berkas asli/turunan yang dikelola aplikasi; kebijakan backup foto harus selaras dengan retensi ini sebelum deployment.
- Waktu dari server, pengiriman berulang idempoten. Lokasi di luar radius, akurasi buruk, kamera/GPS/internet gagal menampilkan kendala; TU Pengelola Kehadiran Guru mencatat manual dengan alasan. Pencatatan susulan dan koreksi historis tidak dibatasi usia tanggal, tetapi waktu kejadian tidak boleh di masa depan. Waktu kejadian dan waktu rekam server terpisah. Koreksi memakai version, alasan wajib dan histori append-only; bukti GPS/selfie asli dipertahankan. Guru dapat mengirim permintaan pelengkapan/koreksi yang tidak memutasi kehadiran. Izin/Sakit/Dinas Luar dilaporkan guru untuk tanggal lampau/hari ini/mendatang dan langsung berlaku tanpa persetujuan; form laporan beserta koreksi/pembatalan TU dan histori tersedia pada DEV-03G2. Laporan memakai satu rentang inklusif, keterangan wajib, dan menolak overlap laporan aktif secara atomik. Laporan tetap terpisah dari event Masuk/Pulang; mesin status akhir dan rekap berbasis jadwal DEV-03G3 telah melewati UAT Product Owner serta final gate publikasi menurut HANDOVER. Penilaian memakai revisi published serta penugasan efektif pada tanggal kejadian, tanpa toleransi; selisih positif dibulatkan ke atas ke menit. Tanggal mendatang/berjalan dibedakan dari hasil akhir, sumber belum lengkap menjadi Belum Dapat Dinilai, dan tugas nonmengajar belum dinilai karena sumber penugasan terstruktur belum tersedia. Rekap hanya jumlah per status tanpa persentase final. Tidak ada approval Kepala Sekolah.

### 7.3 Absensi harian siswa dan siklus kartu QR
- Petugas Absensi menangani seluruh sekolah: login → pemindai → scan kartu → nama/kelas/waktu/hasil. Tahap awal hanya kedatangan siswa.
- Kartu berisi kode acak yang merujuk ID siswa, tanpa nama/NISN di payload. Scan ganda tidak menduplikasi. Penempatan harus berlaku pada tanggal itu. Status Sakit/Izin/Alpa yang sudah ada tidak ditimpa scan; petugas meninjau dan mengoreksi beralasan.
- Petugas juga mengisi Sakit/Izin/Alpa dan manual bila kartu/perangkat bermasalah. Belum scan tetap Belum Dicatat.
- Kartu hilang: Petugas memilih Ganti QR karena hilang, menonaktifkan kode lama dan menerbitkan kode baru untuk identitas yang sama; petugas/waktu/alasan dicatat. Kode lama tetap tidak berlaku bila kartu ditemukan. Sambil menunggu cetak kartu baru, kehadiran manual beralasan Kartu QR hilang.
- Kartu rusak tetapi masih dipegang: cetak ulang kode sama. Cetak ulang berbeda dari penggantian kode. Kehadiran lama tidak berubah.

### 7.4 Absensi per pelajaran, koreksi, dan rekap
- Guru → Jadwal Saya → tanggal/pertemuan → daftar siswa → isi → Simpan. Server memeriksa akun/role/guru aktif, penugasan resmi yang efektif, pertemuan terpublikasi dan roster bertanggal. Satu catatan per siswa/pertemuan.
- Tombol Tandai semua Hadir adalah aksi eksplisit pengguna, lalu status siswa disesuaikan sebelum simpan. Tidak mengisi Hadir dari data kosong atau dari QR harian.
- Koreksi absensi per pelajaran mengikuti keputusan final **D-037**: Guru Mapel historis yang akunnya aktif, ber-role Guru Mapel, dan identitas gurunya aktif boleh mengoreksi catatan yang `teacher_id`-nya sama, meskipun penugasan/jadwal/roster kini berubah. Guru pengampu baru tidak mewarisi hak tersebut. Kurikulum hanya menjadi fallback administratif bila akun historis tidak aktif/tidak terhubung atau role Guru Mapel historis telah dicabut; profil guru nonaktif saja tidak cukup. Admin, Kesiswaan, Kepala Sekolah, dan guru lain tidak mendapat bypass. Koreksi memakai status lama/baru, alasan wajib 1–500 karakter, expected version, mutation lock, dan transaksi atomik dengan histori before/after; konflik meminta tinjau ulang, status sama adalah no-op tanpa histori baru. Catatan dan histori tetap dicari berdasarkan ID permanen serta snapshot kejadian asli. Tidak ada approval Kepala Sekolah. Keputusan ini tidak mengubah hak baca rekap yang masih mengikuti cakupan peran tersendiri.
- Wali membaca kelasnya, Kesiswaan membaca rekap siswa sekolah, Kepala Sekolah membaca ringkasan; role gabungan mengikuti masing-masing cakupan.
- Perbedaan sumber hanya ditandai untuk pemeriksaan; tidak menyimpulkan pelanggaran atau menimpa sumber. Definisi indikator anomali rinci masih dibahas saat integrasi. Keterlambatan siswa mengikuti batas kedatangan Kesiswaan pada D-036/bab 7.6; pulang cepat belum ditetapkan.

### 7.5 Urutan dan bukti lulus
1. Fondasi kalender/tanggal penempatan/akses.
2. Harian siswa, QR, cetak/ganti kartu, manual, koreksi.
3. Per pelajaran melalui Jadwal Saya.
4. GPS/selfie guru, fallback, retensi otomatis.
5. Rekap dan pengujian gabungan, lalu Fase 8A Kesiswaan/BK.

FE mengikuti TailAdmin/pola aplikasi: filter bertahan, tabel bernomor/pagination 10, badge, modal koreksi, menu aksi saling menutup, notifikasi 10 detik, pemindai/kamera nyaman di ponsel. Uji akses negatif setiap role; roster sebelum/sesudah pindah/keluar/kembali; hari libur; batas semester; revisi jadwal; scan/kirim ganda; konflik koreksi; ukuran/akses/retensi foto; fallback dan histori. Bukti implementasi aktual dicatat di HANDOVER per urutan, bukan menganggap seluruh rancangan sudah tersedia.

## 8. Jurnal Mengajar — FINAL inti

- Jurnal mencatat kelas, mapel, blok pelajaran aktual, **guru pengampu resmi yang berwenang**, waktu, dan materi. Tidak ada pemilihan Guru Pengganti/Cadangan.
- Status **Draf** dan **Final**; guru berwenang dapat memfinalisasi sendiri. Koreksi Final dengan alasan dan riwayat, **tanpa inbox approval berlapis**.
- Anomali antara jadwal, kehadiran guru, absensi siswa, dan jurnal hanya ditandai untuk pemeriksaan; tidak otomatis membuktikan pelanggaran atau menimpa sumber.

**AC:** guru di luar penugasan ditolak; jurnal Final tidak dapat diubah tanpa mekanisme koreksi beralasan; tidak ada jurnal oleh Guru Pengganti/Cadangan; jadwal yang tidak terlaksana tidak otomatis memiliki jurnal Final.

## 9. Penilaian — FINAL prinsip, OPEN rincian formula

- Hak input, remedial, dan finalisasi nilai hanya mengikuti guru pengampu resmi yang ditugaskan. Tidak ada akses nilai sementara untuk Guru Pengganti/Cadangan. Jika pengampu resmi berubah, akses baru berlaku menurut tanggal efektif dan riwayat nilai sebelumnya tidak ditimpa.

- Nilai terikat siswa, kelas, mapel, semester, TP, dan komponen penilaian yang sah.
- Guru berwenang menginput nilai, menjalankan remedial, menetapkan nilai akhir dan memfinalisasi sesuai penugasan.
- Rekomendasi sistem hanya dihitung setelah rumus/bobot/pembulatan disetujui; guru dapat memilih nilai akhir berbeda dengan alasan yang tersimpan.
- **Koreksi nilai Final:** guru mengajukan koreksi beserta alasan; Kurikulum memeriksa dan menyetujui koreksi, atau Kurikulum dapat menjalankan koreksi administratif yang tercatat bila kebijakan sekolah mengizinkan. **Tidak perlu Kepala Sekolah untuk koreksi biasa.** Riwayat nilai sebelum/sesudah wajib disimpan. Rincian apakah Kurikulum dapat mengoreksi langsung tetap **OPEN P0**.
- Sengketa nilai khusus dapat dibawa ke pimpinan sekolah melalui mekanisme di luar aplikasi; tidak perlu membangun mesin eskalasi digital.

**AC:** guru tanpa penugasan ditolak; nilai rekomendasi tidak direka; beda nilai akhir punya alasan; nilai Final tidak dapat diedit langsung; koreksi beralasan dan historis.

## 10. Laporan — FINAL prinsip

- Laporan akademik, kehadiran, dan kelengkapan mengambil sumber sah dan tidak mengubah data.
- Bedakan data **Final**, **Sementara**, dan **Belum Lengkap**; laporan bukan rapor resmi.
- Kepala Sekolah dapat memantau ringkasan dan laporan sekolah; Kurikulum melihat laporan akademik sesuai tugas; Guru/Wali Kelas hanya cakupan tugasnya. Detail katalog laporan dan ekspor masih **OPEN P0/P1**.

**AC:** data kosong ditandai belum lengkap; pengguna di luar cakupan ditolak; laporan historis tidak berubah hanya karena pergantian guru/kelas kini.

## 11. Audit sederhana — FINAL

- Catat siapa, kapan, objek, jenis tindakan, perubahan penting, dan alasan bila koreksi sensitif.
- Audit berlaku untuk perubahan role/penugasan, jadwal, penempatan, transisi, absensi sensitif, jurnal Final, dan nilai Final.
- **Tidak perlu** modul “Delegasi”, “Inbox Persetujuan”, “Mandat”, atau approval berlapis. Riwayat audit bukan alur persetujuan.
- Admin dapat memeriksa perubahan akses; akses log sensitif disesuaikan role yang relevan dan dilindungi di backend.

## 12. Workflow per peran

1. **Admin:** lengkapi data siswa/guru → buat akun/role → buat kelas pada tahun yang disiapkan Kurikulum → tempatkan siswa. **Kurikulum:** tetapkan mapel/wali/pengampu → hak operasional mengikuti penugasan aktif.
2. **Kurikulum:** siapkan tahun beserta dua semester → aktifkan kalender → baca kelas/penempatan dari Admin → susun penugasan/jadwal → publikasi → kelola transisi dengan pratinjau dan validasi.
3. **Guru Mapel:** buka jadwal saya → catat absensi pelajaran → isi/finalkan jurnal → input/remedial/finalkan nilai.
4. **Wali Kelas:** buka kelas saya → pantau kehadiran/kelengkapan → tindak lanjut.
5. **Kepala Sekolah:** lihat dashboard dan laporan → keputusan khusus di luar alur rutin.
6. **Kesiswaan:** lihat siswa/rekap kehadiran sekolah → catat pelanggaran dan pembinaan → tandai Perlu tindak lanjut atau Selesai.
7. **BK:** buka siswa/kasus yang ditangani → catat bimbingan dan tindak lanjut → tandai Perlu tindak lanjut atau Selesai. Wali Kelas/Kepala Sekolah membaca sesuai cakupan bab 3B.
8. **Petugas Absensi Siswa:** kelola QR dan absensi harian siswa sesuai cakupan tugas; tidak mengelola kehadiran guru.
9. **TU Pengelola Kehadiran Guru:** melengkapi atau mengoreksi kejadian guru sesuai D-039.

## 13. Acceptance criteria lintas modul

| ID | Kasus | Hasil wajib |
|---|---|---|
| XM-01 | Admin memberikan role Kurikulum kepada guru | Akses Kurikulum langsung aktif tanpa approval |
| XM-02 | Kurikulum menetapkan wali kelas 7A | Akses wali kelas 7A aktif, kelas lain tetap dibatasi |
| XM-03 | Guru mengajar dua kelas | Hanya dua kelas-mapel penugasannya dapat dikelola |
| XM-04 | Admin mengganti role | Hak baru berlaku dan hak lama yang tidak relevan dicabut; audit tercatat |
| XM-05 | Guru tanpa penugasan mengakses nilai | Ditolak oleh server |
| XM-06 | Jadwal bentrok dipublikasikan | Ditolak |
| XM-07 | Guru tidak hadir pada jadwalnya | Tidak ada pengganti otomatis, hak sementara, atau jurnal mengajar otomatis |
| XM-08 | GPS/selfie guru berhasil | Tidak otomatis menghasilkan jurnal mengajar |
| XM-09 | Siswa Hadir harian, pelajaran Belum Dicatat | Kedua catatan tetap berbeda |
| XM-10 | Guru mengoreksi jurnal Final | Alasan dan versi sebelumnya tersimpan, tanpa approval Kepala Sekolah |
| XM-11 | Nilai Final dikoreksi | Pemeriksaan Kurikulum, alasan, histori; tanpa approval Kepala Sekolah rutin |
| XM-12 | Perpindahan tengah tahun | Kurikulum memindahkan dengan alasan, histori terjaga, tanpa approval Kepala Sekolah |
| XM-13 | Transisi tahun ajaran | Pratinjau, validasi, eksekusi atomik, tanpa inbox approval digital |
| XM-14 | Laporan berisi data kosong | Ditandai Belum Lengkap |
| XM-15 | Admin mengubah role lalu membuka nilai tanpa role Guru | Akses nilai tetap ditolak |

## 14. OPEN yang benar-benar berdampak

**P0:** rincian penutupan semester; kesiapan kelas/penempatan khusus; parameter teknis GPS/akurasi dan fallback sesuai D-033; definisi status anomali; komponen/TP/bobot/rumus/pembulatan/remedial; detail koreksi nilai Final oleh Kurikulum; katalog dan cakupan laporan. Putuskan **ketika fase terkait tiba**, bukan menunda fondasi.

**Ditunda di luar cakupan v1:** pengampu bersama dan pembagian hak nilai akhirnya. Generator yang dahulu ditunda telah diterapkan sesuai bab 6.1 dan D-025; batas ketersediaan individu guru belum tersedia. Pengembangan berikutnya mengikuti kebutuhan nyata dan keputusan pengguna.

**P1:** pencatatan jadwal yang tidak terlaksana tanpa guru pengganti, format atribut opsional, pemberitahuan perubahan jadwal, tenggat jurnal/koreksi, format ekspor, penanganan aktivitas luar sekolah.

**Aturan sumber:** nomor lama GOV/MST/TA/AKD dan lainnya adalah referensi historis; ringkasannya tidak boleh dipakai untuk menghidupkan kembali approval berlapis yang telah disederhanakan secara eksplisit di PRD ini. Bila konflik ditemukan, aturan penyederhanaan terbaru menjadi acuan sampai pemilik proyek memutuskan lain.

## 15. Definisi selesai

Setiap fitur harus memenuhi workflow, hak server-side, validasi, histori/audit yang relevan, antarmuka yang bisa digunakan, tes positif/negatif, dan tes regresi. Bedakan dirancang, dibuat, diuji, dan siap dipakai.


### Pilihan hasil dan kelas tujuan transisi — disetujui 23 Oktober 2026
Setiap siswa dimulai dengan **Pilih hasil**, tanpa keputusan otomatis. Kelas 7/8 memiliki hasil Naik kelas (tujuan tingkat berikutnya), Tidak naik (tujuan tingkat sama), atau Keluar (tanpa tujuan). Kelas 9 hanya memiliki Lulus (tanpa tujuan), Tidak lulus — mengulang kelas 9 (wajib kelas 9 tahun tujuan), atau Keluar (tanpa tujuan). Dropdown tujuan muncul hanya untuk naik/mengulang dan hanya menawarkan tingkat yang sesuai. Perubahan ke Lulus/Keluar mengosongkan pilihan kelas lama; pratinjau menggunakan label bahasa Indonesia dan tanda — untuk tujuan yang tidak berlaku. Server memeriksa ulang aturan yang sama. Semua keputusan siswa harus ditetapkan sebelum menjalankan batch; keputusan yang belum ditetapkan tidak dianggap naik/lulus. Tidak membuat kelas 10, tidak menghapus riwayat, dan tidak mengubah status master siswa otomatis.

### Pelengkap fondasi Fase 8: tanggal penempatan massal (D-034)
Admin membuka Penempatan Siswa → Tanggal mulai penempatan → Isi tanggal massal. Pilih cakupan semua kelas atau kelas awal tertentu; hanya siswa tanpa tanggal ditampilkan. Tanggal awal Semester 1 ditawarkan sebagai saran yang bisa diubah; siswa pindahan dapat dikeluarkan dari pilihan. Pilihan siswa berlaku lintas pagination dan tetap dipilih saat pencarian. Setelah tanggal/alasan diisi, pratinjau memeriksa seluruh siswa terpilih dan menampilkan konflik kalender atau kejadian perpindahan. Konfirmasi eksplisit menyimpan seluruh tanggal dan histori dalam satu transaksi; tanggal yang sudah ada tidak ditimpa. Data terpilih/kalender/riwayat yang berubah sejak pratinjau membatalkan penyimpanan dan meminta pemeriksaan ulang. Pratinjau berlaku 10 menit, maksimum 5.000 siswa per batch. Penempatan baru juga menampilkan saran tanggal awal Semester 1; pengguna memastikan tanggal sebenarnya sebelum menyimpan. Tanpa kalender, tanggal tidak ditebak.

### Pelengkap implementasi 7.3 — D-035
Absensi harian hanya mencatat kedatangan. Tanggal/waktu QR dari server WIB; tanggal mendatang dan pencatatan baru pada hari libur ditolak. Status awal tetap Belum Dicatat. Manual/koreksi memerlukan alasan; sesuai D-036, waktu datang wajib untuk Hadir baru/koreksi. Data lama tanpa waktu tidak diisi otomatis. Bukti tersimpan membekukan identitas/kelas/penempatan/semester pada saat pencatatan. Perubahan roster kemudian tidak menulis ulang bukti tersebut; formulir pencatatan baru menolak roster/kalender yang berubah sejak dibuka. Koreksi memakai versi dan histori atomik, tetap bisa dilakukan atas bukti lama bila kalender kemudian diubah menjadi libur. Wali Kelas hanya membaca kelas penugasannya; Kesiswaan/Kepala Sekolah membaca seluruh sekolah; hak tulis dan kartu hanya Petugas Absensi.
Penerbitan/ganti/cetak ulang kartu memerlukan konfirmasi dan alasan. Scan ganda pada catatan Hadir mempertahankan waktu awal; status lain memerlukan koreksi terpisah. Kode acak 256 bit di-host/dihasilkan lokal, tidak berisi identitas siswa. Penggantian langsung mencabut kode lama; cetak ulang tidak mengganti kode. Kamera memakai HTTPS/localhost; pemindai USB atau manual menjadi fallback. Belum mencakup scan pulang, absensi pelajaran, keterlambatan, GPS/selfie guru, atau rekap lanjutan.

### 7.6 Keterlambatan dan isi cepat harian — D-036 (24 Oktober 2026)
Menggantikan penundaan aturan terlambat serta izin jam Hadir kosong pada 7.3/pelengkap D-035: **Kesiswaan** mengatur batas kedatangan dan toleransi, berbeda per semester/hari dengan pengecualian rombel/tanggal, serta mulai berlaku. Tidak disamakan dengan jam pelajaran pertama/upacara. Wajib jam kedatangan sebenarnya untuk pencatatan/koreksi Hadir; QR/Catat datang sekarang memakai waktu server WIB. Sakit/Izin/Alpa tanpa jam. Hadir memiliki keterangan tepat waktu/terlambat, bukan status kehadiran baru. Tidak menghasilkan sanksi atau poin otomatis.
Toleransi menentukan status; durasi terlambat dihitung dari batas kedatangan asli, dengan ketelitian menit untuk QR/manual. Pada menit batas+toleransi masih tepat waktu. Aturan ber-versi dan disalin ke bukti absensi; perubahan berlaku sejak tanggal pilihan dan tidak menghitung ulang bukti lama. Prioritas pengecualian: tanggal+rombel → tanggal semua rombel → mingguan rombel → mingguan semua rombel; cakupan sama mengikuti tanggal efektif terbaru lalu versi terakhir. Tanpa aturan, waktu tetap dicatat dan keterlambatan belum dapat dinilai. Hadir lama tanpa jam tetap perlu dilengkapi berdasarkan bukti, tanpa tebakan.
Petugas membuka Isi cepat per kelas: seluruh siswa ditampilkan, pilih status langsung, tombol eksplisit tandai sisanya Hadir, waktu bersama hanya untuk yang benar-benar diketahui, koreksi waktu per baris, alasan sekali, ringkasan jumlah status dan Simpan satu transaksi. Siswa belum dipilih tetap Belum Dicatat. Catatan yang sudah ada tidak ditimpa; koreksi melalui jalur individual. Jika aturan/roster/absensi berubah sejak form dibuka, seluruh batch ditolak dan isian dipertahankan untuk peninjauan ulang. Pencarian tidak menghapus pilihan; pergantian tanggal/kelas/halaman memberi peringatan perubahan belum tersimpan. Hak operasional dan histori tetap sesuai 7.3–7.4.

### 7.7 Kehadiran mandiri Pegawai TU — D-041 (29 Oktober 2026)
Pegawai TU aktif dengan akun aktif yang tertaut ke identitas master aktif dan role `pegawai_tu` dapat mencatat Masuk dan Pulang untuk dirinya sendiri melalui GPS, geofence, pemeriksaan akurasi, dan selfie. Waktu berasal dari server dan tanggal dihitung menurut WIB. Maksimal satu Masuk dan satu Pulang per tanggal WIB; Pulang tanpa Masuk tetap diizinkan. Pegawai hanya membaca catatan dan bukti miliknya, tanpa hak mengubah atau membatalkan catatan. Capture TU, metadata foto, dan histori disimpan dalam domain terpisah dari kehadiran guru. Retensi file foto dua bulan kalender; identitas, catatan, metadata, dan histori dipertahankan. Ketentuan D-039 untuk pengelola kehadiran guru tetap berlaku, termasuk pasangan role dan larangan koreksi diri sendiri. Kehadiran TU tidak digabung dengan rekap kewajiban mengajar guru. Belum ditetapkan jam kerja, keterlambatan berbasis jadwal, Alpa otomatis, persentase, maupun kewajiban nonmengajar TU.
