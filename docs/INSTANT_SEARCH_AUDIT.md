# DEV-03G3-UX-01 — inventaris pencarian dan filter

Keputusan UI Product Owner (29 Oktober 2026): pencarian/filter daftar otomatis, tanpa submit manual atau reload halaman saat JavaScript tersedia. Bukan perubahan aturan kehadiran/role. DEV-03G3 tetap NEEDS REVISION sampai UAT ulang dinyatakan PASS.

## Pola dan batas komponen

`instant-search.js` meningkatkan hanya form GET di region daftar. HTMX mengambil fragment `list-fragment`; layout, sidebar, header, judul dan notifikasi global tidak diganti. Komponen daftar berisi toolbar dan hasil/dependensi semester/kelas; node input pencarian server dipertahankan dengan `hx-preserve` dan fokus/posisi kursor dipulihkan. Toolbar/opsi lain dirender ulang sesuai konteks baru. Respons penuh tetap tersedia untuk request biasa dan seluruh POST.

Debounce 350 ms, abort ketika input baru masuk, serta pemeriksaan generasi respons mencegah respons lama mengalahkan ketikan terbaru, termasuk selama debounce. Query/filter dipertahankan melalui GET dan URL `replaceState` setelah respons sah, tanpa entri per karakter. Pagination server tetap berasal dari URL backend; filter baru kembali ke halaman pertama. Filter tanggal opsional boleh sama-sama kosong, rentang parsial/terbalik tidak mengirim request. Loading mempertahankan tabel lama; error menampilkan pesan/retry. Panel filter terbuka dipertahankan.

Tidak ada pencarian yang memicu POST. Dialog terbuka atau isian inline POST yang belum disimpan menahan pembaruan. Pencarian lokal pada formulir batch hanya menyembunyikan baris, bukan membuang node/pilihan. Inisialisasi konten sesudah swap idempoten per elemen; listener beforeunload dibersihkan ketika form dilepas. Modul capture kamera/QR, editor detail dan form mutasi tidak ditingkatkan menjadi filter GET.

## Inventaris seluruh pola existing

| Halaman / sumber template | Role sesuai akses existing | Mode dan tindakan |
|---|---|---|
| Pengguna — page.html/users | Admin | Server: nama/username + role/status, pagination. Menggantikan requestSubmit/reload dengan GET HTMX. |
| Master siswa/guru/mapel — master.html | Admin/Kurikulum dan pembaca yang berwenang | Server: nama/NIS/NISN/NIP/kode sesuai store + status. Tetap mencari sebelum LIMIT/OFFSET. |
| Penempatan siswa — placement.html | Admin; pembaca sesuai domain | Server: q + tahun/kelas + pagination. Dua input q menyatakan filter yang sama dan diselaraskan. |
| Calon siswa belum ditempatkan — placement.html | Admin | Diperbaiki: sebelumnya filter lokal atas 10 baris server; kini GET q memakai UnplacedStudents dan pagination server yang sama. Tidak mengambil seluruh siswa ke browser. |
| Tanggal mulai penempatan — attendance_foundation.html | Admin | Tahun GET HTMX; nama/kelas lokal atas seluruh PlacementDates pada tahun pilihan, baru dipaginasi di browser. |
| Tanggal massal — placement_date_batch.html | Admin | Cakupan kelas GET HTMX; nama/kelas lokal atas seluruh kandidat cakupan. Isian tanggal/alasan/pilihan batch terlindung. |
| Tahun ajaran, semester, kelas — academic.html | Kurikulum/Admin sesuai tugas | Nama/kelas lokal atas seluruh daftar yang dikembalikan store; tidak ada pagination SQL pada sumber ini. |
| Pengampu — academic.html | Kurikulum | Semester GET HTMX; daftar/opsi penugasan diperbarui bersama. Tidak menambah search box secara paksa. |
| Jam pelajaran — academic.html | Kurikulum | Semester/hari GET HTMX. Form Simpan tetap POST biasa. |
| Jadwal mingguan + target JP — academic.html | Kurikulum | Semester GET HTMX; pencarian target rombel/mapel lokal atas seluruh target revisi. Draf belum disimpan dilindungi. |
| Jadwal Saya — academic.html | Guru Mapel | Filter hari/kelas/mapel lokal atas seluruh jadwal terbit yang berhak dibaca; pagination sesudah filter. |
| Kelas Saya/roster — academic.html | Wali Kelas | Daftar yang sudah tersedia dipaginasi lokal; tidak menambah pencarian tanpa kebutuhan. Hak roster tidak diperluas. |
| Struktur kurikulum / kebutuhan efektif — curriculum.html | Kurikulum | Semester/tanggal GET HTMX; tingkat/rombel/mapel/guru lokal atas seluruh aturan/hasil cakupan pilihan. |
| Istirahat/kegiatan — curriculum.html | Kurikulum | Semester GET HTMX; hari/kegiatan lokal atas seluruh blok semester. |
| Batasan jadwal/kisi — schedule_constraints.html | Kurikulum | Semester/sasaran GET HTMX setelah pilihan valid; pencarian kisi lokal atas seluruh kisi semester. Modal/paint yang terbuka atau berubah dilindungi. |
| Kalender sekolah — attendance_foundation.html | Kurikulum | Semester GET HTMX; tidak mengubah transaksi kalender. |
| Transisi siswa — transition.html | Kurikulum | Tahun sumber/tujuan GET HTMX; hasil dan kelas tujuan belum disimpan dilindungi. |
| Impor Excel — import.html | Admin/Kurikulum sesuai kind | Pencarian lokal atas seluruh baris pratinjau unggahan, bukan pencarian database global. Pemetaan/konfirmasi tetap terpisah. |
| Impor aSc XML — asc_import.html | Kurikulum | Pencarian lokal per kelompok pemetaan dan seluruh pratinjau pertemuan. Tidak membuang pilihan lintas halaman. |
| Batas kedatangan — arrival_rules.html | Kesiswaan | Semester GET HTMX; rombel/hari/batas lokal atas seluruh aturan dan versi yang tersedia. |
| Absensi harian — daily_attendance.html | Petugas siswa/Wali/Guru/Kesiswaan/Kepala Sekolah sesuai cakupan | Tanggal/kelas GET HTMX; nama/NIS/status lokal atas seluruh roster bertanggal. Tidak mengubah status atau memperluas akses. |
| Kartu QR siswa — daily_attendance.html | Petugas Absensi Siswa | Nama/NIS/kelas dan kelas lokal atas seluruh daftar kartu tahun aktif; pagination sesudah filter. |
| Isi cepat harian — daily_quick.html | Petugas Absensi Siswa | Tanggal/kelas GET HTMX bila tidak ada draft; pencarian lokal atas seluruh roster, tidak mengganti form. |
| Absensi pelajaran — lesson_attendance.html | Guru Mapel; fallback koreksi sesuai D-037 | Tanggal GET HTMX bila tidak ada draft; pencarian lokal seluruh roster pertemuan. Listener form diinisialisasi ulang tanpa duplikasi. |
| Kelola kehadiran guru — teacher_attendance_management.html | Pasangan role TU | Server: nama + tanggal, pagination guru dan inbox. Tidak mencari lokal pada 10 baris. |
| Penanganan kehadiran TU — template yang sama | Admin sesuai fallback D-039 | Server dengan pembatasan target existing. Tidak memperoleh rekap sekolah hanya karena Admin. |
| Laporan guru / laporan sendiri / fallback — teacher_attendance_reports.html | TU/guru/Admin sesuai scope | Server: nama/jenis/status/rentang + pagination; riwayat sendiri memakai pagination HTMX tanpa memaksa search box baru. |
| Histori laporan — template yang sama | Pemilik/TU/fallback sesuai scope | Pagination GET HTMX, koreksi/pembatalan tetap POST dan modal terlindung. |
| Rekap sendiri / rekap guru — teacher_attendance_recap.html | Guru atau pasangan role TU | Server: rentang + nama guru; pagination guru/tanggal, fokus/search/clear otomatis. Mesin penilaian DEV-03G3 tidak diubah. |
| Histori akses/akademik/wali/penempatan | Role pemilik domain | Tidak ada input pencarian yang perlu dikonversi; pagination lokal existing tetap. Sumber histori terbatas 100 terbaru tetap dinyatakan sebagai histori, bukan search seluruh arsip. |
| Login, password, detail identitas, GPS/selfie, pemindai QR, lokasi sekolah, form edit | Sesuai role | Bukan filter daftar; tidak diberikan Instant Search dan tidak mengubah aksi mutasinya. |

Sumber pagination SQL yang diperiksa: auth.Store.List, master.Store.List, academic.Placements/UnplacedStudents, TeacherAttendanceManaged, TeacherAttendanceReports, TeacherAttendanceRecaps. Pencarian dilakukan di SQL yang diparameterisasi sebelum LIMIT/OFFSET. Data untuk pencarian lokal lainnya tersedia lengkap dalam cakupan halaman/pratinjau; bukan filter atas halaman pertama SQL. Tidak ada perubahan SQL, schema, migration, role atau bisnis absensi dalam revisi ini.

## Verifikasi

Bukti tes dan matriks browser aktual dicatat di HANDOVER setelah pemeriksaan. Inventaris ini merupakan peta cakupan kode, bukan klaim bahwa tiap role telah diuji pada ponsel fisik. Halaman pratinjau impor memerlukan unggahan; pengujian HTTP otomatis dan pemeriksaan browser tanpa fixture unggahan harus dibedakan.
