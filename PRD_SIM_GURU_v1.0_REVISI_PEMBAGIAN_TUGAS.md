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

SIM-GURU menyatukan master data siswa/guru/mapel, tahun ajaran dan semester, kelas/penempatan, penugasan dan jadwal, absensi guru QR, absensi siswa harian dan per pelajaran, jurnal mengajar, penilaian, serta laporan akademik dan kehadiran. Data historis dipertahankan; laporan bukan rapor resmi. Satu sekolah, bukan multi-tenant.

## 3. Model kewenangan sederhana — FINAL (menggantikan rumusan mandat/delegasi sebelumnya)

**Alur tunggal:** rapat/keputusan sekolah di luar aplikasi → **Admin mengelola data siswa/guru, akun, role, kelas dan penempatan awal; Kurikulum mengelola kalender, penugasan dan jadwal akademik** → hak akses langsung aktif → personel bekerja sesuai tugas. Tidak ada permintaan dan approval digital untuk penetapan rutin.

| Peran | Hak utama | Batas |
|---|---|---|
| Admin | Pengelola identitas siswa/guru, akun, role, status akses, konfigurasi teknis, pembuatan/koreksi kelas dan penempatan awal siswa | Tidak otomatis mengelola kalender, mapel, wali kelas, pengampu, jadwal, transisi atau nilai |
| Kurikulum | Menyiapkan tahun ajaran beserta dua semester dan mengaktifkannya; kelola mapel, wali kelas, pengampu, jadwal, transisi kenaikan/kelulusan dan pemeriksaan koreksi nilai Final | Kelas dan penempatan awal siswa baca-saja; tidak mengubah identitas siswa/guru; tidak membutuhkan persetujuan digital rutin |
| Guru Mapel | Absensi per pelajaran, jurnal, TP/komponen/nilai/remedial pada kelas-mapel yang ditugaskan | Tidak boleh mengubah kelas/mapel di luar penugasan |
| Wali Kelas | Pantau siswa dan absensi kelas yang ditugaskan, tindak lanjut kelas | Tidak otomatis mengubah nilai seluruh mapel |
| Kepala Sekolah | Lihat ringkasan/laporan sekolah; menangani keputusan akademik khusus | Tidak menjadi antrean persetujuan pekerjaan harian |
| BK | Pantau dan tindak lanjut data bimbingan yang relevan dengan tugas | Tidak otomatis mengubah nilai |
| Kesiswaan | Pantau dan tindak lanjut urusan kesiswaan sesuai role | Tidak otomatis mengubah jadwal atau nilai |
| Petugas Absensi | Catat/koreksi kehadiran sesuai cakupan tugas | Tidak otomatis mengubah jurnal atau nilai |

**Role Kurikulum langsung memberi akses ke menu dan fungsi Kurikulum.** Role dapat rangkap; penugasan Guru Mapel dan Wali Kelas tetap membatasi kelas/mapel. Admin boleh mengubah role berdasarkan keputusan sekolah, dengan audit sederhana. Pengelolaan penugasan oleh Kurikulum dalam modul akademik dapat berupa data operasional, tetapi **Admin tetap pengelola akun dan role; Kurikulum mengelola penugasan akademik**; tidak perlu approval silang. Bila Kurikulum menyusun pengampu/jadwal, sistem harus menyelaraskan dengan penugasan yang telah ditetapkan tanpa memberikan hak baru diam-diam.

### 3.1 Tindakan yang memerlukan persetujuan Kepala Sekolah

| Tindakan | Aturan v1 |
|---|---|
| Penunjukan Kurikulum, Wali Kelas, Guru Mapel, BK, Kesiswaan | **Tidak perlu approval digital**; Admin mengatur role; Kurikulum mencatat penugasan akademik hasil keputusan sekolah |
| Membuat kelas, penempatan awal, jadwal, perubahan jadwal | **Tidak perlu approval Kepala Sekolah di aplikasi**; dilakukan petugas berwenang |
| Absensi, jurnal, input/finalisasi nilai rutin | **Tidak perlu approval Kepala Sekolah** |
| Kenaikan kelas, kelulusan, transisi tahun ajaran | Berdasarkan keputusan sekolah; Kurikulum mengeksekusi setelah pratinjau/validasi, **tanpa approval digital ulang** |
| Perpindahan siswa antarkelas di tengah tahun | **Persetujuan Kepala Sekolah tetap diperlukan menurut baseline sebelumnya**; dapat dicatat sebagai keputusan offline dengan tanggal, alasan, dan rujukan, lalu Kurikulum menjalankan. Tidak wajib inbox approval digital |
| Koreksi nilai yang sudah Final | Perlu alasan, histori, dan pemeriksaan oleh Kurikulum; **tidak otomatis memerlukan Kepala Sekolah**. Sengketa luar biasa dapat dieskalasi secara offline |
| Koreksi jurnal Final atau absensi | Pelaksana berwenang mengoreksi dengan alasan dan histori; **tanpa approval Kepala Sekolah** |

**Catatan perubahan:** ketentuan lama yang menyebut “mandat”, “delegasi”, “persetujuan berlapis”, atau “larangan self-approval” untuk penugasan rutin **digantikan** oleh bab ini. Jangan hidupkan kembali mekanisme tersebut dari ringkasan kontrak lama. Untuk perpindahan tengah tahun, keputusan Kepala Sekolah tetap diperlukan sebagai kebijakan khusus; cara mencatatnya dibuat sederhana. Jika pemilik proyek ingin menghapus kewajiban itu, diperlukan keputusan eksplisit.

## 3A. Kepemilikan data induk dan batas tugas — FINAL (revisi 21 Oktober 2026)

- **Admin adalah satu-satunya pihak yang membuat, mengubah, menonaktifkan, mengoreksi, dan melengkapi identitas induk siswa dan guru.** Ini meliputi identitas, NIS/NISN jika ada, data kontak yang diperlukan, status siswa/guru, dan identitas akun terkait. Hak baca bagi role lain diberikan sesuai kebutuhan tugas, bukan hak mengubah identitas.
- **Admin mengelola akun dan role, membuat/mengoreksi kelas, serta melakukan penempatan awal siswa.** Admin membaca pilihan tahun ajaran untuk kelas; hak ini tidak memberikan kewenangan menyiapkan/mengaktifkan kalender, mengelola mapel, wali kelas, pengampu, jadwal, transisi atau nilai.
- **Kurikulum menerima data induk dan kelas secara baca-saja**, menyiapkan tahun ajaran beserta dua semester, mengaktifkan tahun/semester, mengelola mata pelajaran, penugasan guru pengampu, wali kelas, jadwal, transisi kenaikan/kelulusan, serta pemeriksaan koreksi nilai Final. Kurikulum tidak membuat/mengoreksi kelas, melakukan penempatan awal, atau mengubah identitas induk siswa/guru. Perpindahan tengah tahun tetap mengikuti ketentuan khusus bab 3.1/5; perubahan penempatan hasil transisi mengikuti eksekusi transisi oleh Kurikulum, bukan hak mengedit penempatan awal secara bebas.
- **Guru Mapel, Wali Kelas, BK, Kesiswaan, dan Kepala Sekolah** membaca data siswa/guru sesuai cakupan role; hanya mengubah data operasional milik modul mereka. Permintaan pembetulan identitas diteruskan ke Admin melalui komunikasi sekolah biasa, tanpa membangun workflow approval digital.
- **Satu sumber data:** siswa/guru dibuat sekali oleh Admin dan dipakai melalui referensi ID internal pada seluruh modul; tidak boleh ada salinan identitas yang diedit bebas oleh modul lain.
- **Hak akses gabungan:** role diberikan Admin; cakupan kelas/mapel/wali berasal dari penugasan Kurikulum. Akses server harus memeriksa keduanya bila diperlukan.
- Audit perubahan identitas dan penugasan mencatat pelaku, waktu, objek, serta sebelum/sesudah. Jangan menghapus histori yang telah digunakan modul akademik.

**Acceptance criteria:** Kurikulum tidak bisa mengubah nama/NISN siswa atau identitas guru; Admin dapat membuat/mengoreksi kelas dan melakukan penempatan awal; Kurikulum tanpa role Admin tidak dapat melakukannya; Admin tanpa Kurikulum tidak bisa menetapkan pengampu atau mengubah kalender; Admin dapat memperbaiki identitas dan perubahannya tercermin pada seluruh modul; penugasan oleh Kurikulum langsung mengaktifkan cakupan akses Guru Mapel/Wali Kelas tanpa persetujuan digital.

**Revisi disetujui pemilik proyek 21 Oktober 2026:** pembuatan/koreksi kelas dan penempatan awal beralih ke Admin; kalender dan seluruh tugas Kurikulum yang disebut di atas tetap Kurikulum. ID kelas dan histori sebelum perubahan kebijakan dipertahankan. Rujukan keputusan: [D-015](docs/DECISIONS.md#d-015--pemisahan-kelasadmin-dan-kalenderkurikulum-disetujui-21-oktober-2026).

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
- **Transisi:** Kurikulum menyiapkan pratinjau kenaikan/tidak naik/lulus/keluar, memeriksa data dan konsekuensi, kemudian mengeksekusi hasil keputusan sekolah. **Tidak ada tombol persetujuan Kepala Sekolah wajib untuk setiap batch.** Proses atomik dan riwayat tetap dijaga.
- Perpindahan tengah tahun membutuhkan keputusan Kepala Sekolah yang dicatat sederhana (misalnya nomor/tanggal keputusan atau catatan hasil rapat), bukan workflow digital berlapis.

**AC:** tahun/semester aktif ganda ditolak; penempatan normal ganda ditolak; pratinjau transisi menampilkan perubahan; kegagalan batch tidak meninggalkan hasil parsial; perpindahan menjaga riwayat.

## 6. Penugasan dan Jadwal — FINAL inti

- Kurikulum menetapkan Guru Mapel pada kelas-mapel dan Wali Kelas pada kelas sesuai keputusan sekolah; hak langsung berlaku.
- Kurikulum menyusun jam pelajaran, jadwal draf/publikasi, dan perubahan jadwal. Benturan guru/kelas/jam dicegah.
- Jadwal = rencana, jurnal = kegiatan aktual. Perubahan jadwal tidak mengubah jurnal masa lalu.
- **FINAL: Tidak ada Guru Pengganti atau Guru Cadangan.** Ketidakhadiran guru tidak menghasilkan penugasan pengganti, pemberian akses sementara, atau jurnal atas nama guru lain.
- **Perubahan guru pengampu resmi tetap didukung:** Kurikulum memperbarui penugasan sesuai keputusan sekolah, dengan tanggal efektif dan riwayat pengampu lama; Kurikulum menyesuaikan jadwal untuk periode mendatang. Ini bukan penugasan sementara.
- Pertemuan yang tidak terlaksana tetap dapat terlihat sebagai jadwal tanpa jurnal Final; jangan otomatis menganggap telah diajar atau mencatat guru lain. Detail status kegiatan tidak terlaksana **OPEN P1** dan tidak boleh mengubah status absensi siswa secara otomatis.
- Pengampu bersama boleh ada, tetapi pembagian hak nilai akhir **OPEN P0**; jangan berikan hak mengubah nilai Final kepada semua guru tanpa aturan.

**AC:** benturan jadwal ditolak; Guru Mapel tanpa penugasan ditolak; tidak ada pengganti/cadangan atau hak akses sementara; perubahan pengampu resmi menyimpan histori; histori jadwal utuh.

## 7. Absensi — FINAL inti

- Ketidakhadiran guru tidak memicu penunjukan pengganti/cadangan. Absensi guru, absensi siswa, jadwal, dan jurnal tetap merupakan sumber data yang terpisah.

- Guru: QR kedatangan dan kepulangan, terpisah dari bukti mengajar. Mekanisme QR/fallback ketika gagal masih **OPEN P0**.
- Siswa: absensi harian berbeda dari absensi per pelajaran. Status: **Hadir, Sakit, Izin, Alpa, Belum Dicatat**. “Belum Diverifikasi” adalah kondisi proses, bukan status keenam.
- Tidak ada inferensi otomatis Hadir/Alpa dari data kosong. Perbedaan data dapat ditandai untuk pemeriksaan.
- Koreksi dilakukan guru/petugas berwenang dengan alasan dan riwayat. Tidak perlu approval Kepala Sekolah.

**AC:** scan guru tidak membuat jurnal; Hadir harian tidak mengisi absensi tiap mapel; Belum Dicatat tidak otomatis menjadi Alpa; koreksi terlacak.

## 8. Jurnal Mengajar — FINAL inti

- Jurnal mencatat kelas, mapel, blok pelajaran aktual, **guru pengampu resmi yang berwenang**, waktu, dan materi. Tidak ada pemilihan Guru Pengganti/Cadangan.
- Status **Draf** dan **Final**; guru berwenang dapat memfinalisasi sendiri. Koreksi Final dengan alasan dan riwayat, **tanpa inbox approval berlapis**.
- Anomali antara jadwal, QR guru, absensi siswa, dan jurnal hanya ditandai untuk pemeriksaan; tidak otomatis membuktikan pelanggaran atau menimpa sumber.

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
5. **Kepala Sekolah:** lihat dashboard dan laporan → keputusan khusus di luar alur rutin; untuk perpindahan tengah tahun, keputusan dicatat.
6. **BK/Kesiswaan/Petugas Absensi:** buka data yang relevan → tindak lanjut/catat sesuai role.

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
| XM-08 | QR guru berhasil | Tidak otomatis menghasilkan jurnal mengajar |
| XM-09 | Siswa Hadir harian, pelajaran Belum Dicatat | Kedua catatan tetap berbeda |
| XM-10 | Guru mengoreksi jurnal Final | Alasan dan versi sebelumnya tersimpan, tanpa approval Kepala Sekolah |
| XM-11 | Nilai Final dikoreksi | Pemeriksaan Kurikulum, alasan, histori; tanpa approval Kepala Sekolah rutin |
| XM-12 | Perpindahan tengah tahun | Keputusan Kepala Sekolah tercatat, histori penempatan tidak hilang |
| XM-13 | Transisi tahun ajaran | Pratinjau, validasi, eksekusi atomik, tanpa inbox approval digital |
| XM-14 | Laporan berisi data kosong | Ditandai Belum Lengkap |
| XM-15 | Admin mengubah role lalu membuka nilai tanpa role Guru | Akses nilai tetap ditolak |

## 14. OPEN yang benar-benar berdampak

**P0:** rincian penutupan semester; kesiapan kelas/penempatan khusus; pembagian nilai guru pengampu bersama; fallback QR; definisi status anomali; komponen/TP/bobot/rumus/pembulatan/remedial; detail koreksi nilai Final oleh Kurikulum; katalog dan cakupan laporan. Putuskan **ketika fase terkait tiba**, bukan menunda fondasi.

**P1:** pencatatan jadwal yang tidak terlaksana tanpa guru pengganti, format atribut opsional, pemberitahuan perubahan jadwal, tenggat jurnal/koreksi, format ekspor, penanganan aktivitas luar sekolah.

**Aturan sumber:** nomor lama GOV/MST/TA/AKD dan lainnya adalah referensi historis; ringkasannya tidak boleh dipakai untuk menghidupkan kembali approval berlapis yang telah disederhanakan secara eksplisit di PRD ini. Bila konflik ditemukan, aturan penyederhanaan terbaru menjadi acuan sampai pemilik proyek memutuskan lain.

## 15. Definisi selesai

Setiap fitur harus memenuhi workflow, hak server-side, validasi, histori/audit yang relevan, antarmuka yang bisa digunakan, tes positif/negatif, dan tes regresi. Bedakan dirancang, dibuat, diuji, dan siap dipakai.
