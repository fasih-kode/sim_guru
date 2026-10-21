# Arsitektur

## Implementasi aktual — 21 Oktober 2026
Modular monolith Go dengan SSR. Fondasi, autentikasi, administrasi akses, master siswa/guru/mapel, impor, kalender akademik, katalog kelas dan penempatan awal siswa tersedia; penugasan wali/pengampu, jadwal dan operasional akademik lanjut belum diimplementasikan.

| Lokasi | Tanggung jawab |
|---|---|
| cmd/server | Composition root, koneksi DB, pemeriksaan migrasi, lifecycle HTTP dan graceful shutdown |
| cmd/admin | Bootstrap interaktif Admin pertama, hanya ketika belum ada akun |
| internal/master | Validasi, akses, PostgreSQL dan histori siswa/guru/mapel dengan ID permanen |
| internal/academic | Otorisasi per tindakan: kalender Kurikulum, kelas Admin, baca kelas bersama; transaksi dan histori |
| internal/auth | Validasi, bcrypt, sesi PostgreSQL, transaksi pengguna/role dan audit akses |
| cmd/migrate | Perintah eksplisit up/status; tidak menjalankan migrasi otomatis saat server mulai |
| internal/config | .env opsional, prioritas environment, validasi konfigurasi dan TLS production |
| internal/database | Pool database/sql dengan driver pgx, ping terbatas waktu, error tanpa DSN |
| internal/web | Routing net/http, template HTML, fragmen htmx, aset embed dan header keamanan |
| migrations | SQL embed, namespace app, Goose dengan advisory lock PostgreSQL |
| web/styles | Sumber Tailwind; template Go menjadi sumber pemindaian kelas |
| scripts | Lifecycle cluster lokal dan penyalinan aset htmx |

Alur: browser → middleware keamanan → router → template lengkap atau fragmen HTML. GET / menghasilkan halaman; HX-Request: true menghasilkan section overview dengan Vary: HX-Request. Aset dilayani lokal dari binary. /health/live memeriksa proses; /health/ready memeriksa DB dan kesesuaian versi migrasi tanpa menampilkan detail koneksi. Server bind loopback secara default, memiliki timeout HTTP dan shutdown 10 detik.

Database memakai namespace app; tabel versi Goose berada di public. Migration 00001 tetap utuh. Migration 00002 menambahkan users, roles, user_roles, sessions dan access_audit, dengan foreign key dan delapan role PRD. Migration 00003 menambah students, teachers, subjects, subject_codes dan tabel histori ber-FK per entitas. Migration 00004 menambah master_imports untuk pratinjau impor sementara dan bukti konfirmasi. Migration 00005 menambah school_years, semesters, classes dan academic_history; indeks parsial membatasi satu tahun/semester aktif. Migration 00006 menambah student_placements (unik siswa+tahun dengan FK kelas+tahun majemuk) dan student_placement_history, serta indeks pasangan kelas+tahun. Migration sebelumnya tidak diubah. Guru dapat ditautkan opsional ke satu akun melalui teachers.user_id UNIQUE FK; penautan tidak menyalin identitas atau mengubah role/password/status akun.

## Autentikasi dan otorisasi aktual
- Login memakai username ternormalisasi dan bcrypt cost 12. Password minimal 12 karakter, maksimal 72 byte UTF-8; tidak dipangkas diam-diam. Akun tidak ada/password salah/nonaktif memakai pesan gagal yang sama; dummy bcrypt mengurangi perbedaan waktu untuk akun tidak ada.
- Sesi memakai token acak 256 bit. Hanya SHA-256 token tersimpan di PostgreSQL. Cookie host-only, HttpOnly, SameSite=Lax, Secure pada production; token baru pada login dan sesi lama direvokasi. Batas absolut 8 jam, idle 30 menit. Logout, deaktivasi, dan perubahan/reset password mencabut sesi di server. Reaktivasi tidak menghidupkan kembali sesi lama.
- Status akun dan role diambil ulang pada setiap request terlindungi. Role bukan cakupan penugasan: Admin tidak otomatis memperoleh Kurikulum atau akses nilai. Modul /akademik mencakup kalender dan daftar kelas; penempatan awal tersedia sebagai alur terpisah.
- Gorilla CSRF memvalidasi token untuk seluruh form mutasi termasuk login/logout, ditambah NewCrossOriginProtection dan batas body 32 KiB. Key signing acak 32 byte wajib di konfigurasi lokal; tidak disimpan Git. Request HTTP development ditandai plaintext untuk library, namun pemeriksaan origin dan token tetap aktif.
- Login dibatasi per proses: 10 percobaan/username dan 30/IP dalam 15 menit, maksimal sekitar 10.000 bucket. Tidak mempercayai X-Forwarded-For. Reset proses mereset limiter; deployment multi-instance/proxy memerlukan desain limiter bersama dan konfigurasi proxy yang benar sebelum produksi.
- Perubahan akun/role, revokasi sesi terkait, dan audit dilakukan dalam transaksi yang sama. Advisory transaction lock menyerialkan mutasi akun/bootstrap/login dengan perubahan password; optimistic version menolak form edit kedaluwarsa. Minimal satu Admin aktif dipertahankan agar akses pengelolaan tidak terkunci.
- Audit menyimpan actor, target ID, waktu, aksi, snapshot aman sebelum/sesudah; tidak menyimpan password/hash password/token. Login/logout/password change juga dicatat. UI Admin menampilkan 100 catatan terbaru dalam WIB. Tidak ada workflow approval.
- users tidak dihapus melalui UI; status aktif/nonaktif mempertahankan referensi dan histori. Role katalog tetap sesuai PRD; Admin memilih kombinasi role, tidak membuat definisi kewenangan baru.

## Route aktual
Publik: GET/POST /login, GET /health/live, GET /health/ready, GET /static/*.
Terautentikasi: GET / (SSR/fragmen htmx), POST /logout, GET/POST /account/password.
Admin: GET /admin/users (cari/filter/pagination 50), GET/POST /admin/users/new, GET/POST /admin/users/{id}/edit, GET /admin/audit.
Kurikulum: GET /akademik; GET/POST /akademik/tahun-ajaran, POST /akademik/tahun-ajaran/{id}/aktifkan; GET /akademik/semester, POST /akademik/semester/{id}/aktifkan; GET /akademik/histori/{school_year|semester}/{id}.
Admin: GET/POST /akademik/kelas/new dan /akademik/kelas/{id}/edit; POST /admin/placements.
Admin/Kurikulum: GET /akademik/kelas, GET /admin/placements, GET /akademik/histori/penempatan/{id}, dan /akademik/histori/class/{id}. Route baca memeriksa role di service berdasarkan domain; route tulis penempatan hanya Admin, role lain ditolak. URL kelas dipertahankan agar tautan lama tetap berlaku.

## Master data aktual
- internal/master memakai Kind dengan whitelist tetap students/teachers/subjects; nama tabel SQL tidak berasal dari input bebas. Query nilai terparameterisasi. ID bigint internal tidak berubah saat nama/nomor/kode dikoreksi.
- Admin membuat/mengubah/menonaktifkan siswa dan guru. Kurikulum membaca identitas siswa/guru dan mengelola mapel. Kontak telepon/email serta relasi akun dihapus dari hasil baca-saja pada service, bukan sekadar disembunyikan tombolnya. Role lain belum mendapat daftar identitas luas sebelum cakupan penugasannya diimplementasikan.
- Nama/tanggal lahir/jenis kelamin siswa wajib; NIS/NISN opsional menjadi NULL ketika kosong. UNIQUE berlaku juga bagi data nonaktif. Jenis kelamin disimpan L/P. Tidak menetapkan format digit NIS/NISN atau aturan sekolah tambahan yang belum disepakati; batas panjang hanya validasi teknis.
- Guru memiliki nama, status master, NIP/NUPTK/email/telepon opsional dan tautan akun opsional. Status master tidak menonaktifkan akun atau memberi/mencabut role. Larangan penugasan baru bagi guru nonaktif harus diterapkan pada modul penugasan mendatang.
- Mapel memiliki kode unik yang dinormalisasi trim/huruf besar. subject_codes mencadangkan semua kode historis ke ID mapel yang sama; rename/nonaktif tidak membebaskan kode ke mapel lain.
- Koreksi/status memerlukan alasan; before/after, actor, waktu, ID entitas disimpan pada tabel histori dalam transaksi yang sama. Form memakai version agar edit kedaluwarsa/concurrent tidak menimpa perubahan. Shared advisory lock dengan modul akun memastikan role diperiksa kembali sebelum write. Tidak tersedia operasi hapus/reset.
- Daftar menyediakan pencarian, filter aktif/nonaktif dan pagination 50. Detail memakai ID internal yang sama antara Admin dan Kurikulum. Histori dibatasi penulis domain: Admin untuk siswa/guru, Kurikulum untuk mapel; UI menampilkan 100 perubahan terakhir dalam WIB.
- HTTP GET /master/{kind}, /new, /{id}, /{id}/edit dan /{id}/history dilindungi autentikasi dan otorisasi per kind. POST hanya /new dan /{id}/edit, dengan CSRF. Tidak ada API PATCH yang mengabaikan aturan form.

## Impor Excel aktual
- internal/master/xlsx.go membaca .xlsx menggunakan Excelize yang dipatok pada revisi upstream berisi perbaikan keamanan; tidak mengevaluasi formula. File maksimum 5 MiB, isi ZIP maksimum 20 MiB/200 entry, 500 baris data dan 2000 baris fisik. Macro/tautan workbook eksternal ditolak; .xls, .xlsm, CSV dan file terenkripsi tidak didukung.
- Template siswa/guru tersimpan sebagai aset embed di internal/web/import_templates, dibuat dengan Artifact Tool dan diperiksa visual. Template diunduh melalui route Admin, bukan aset publik. Sheet Data memiliki header tetap; sheet Petunjuk menjelaskan kolom. Identifier/telepon bertipe teks; DOB menerima teks ISO atau tanggal Excel 1900/1904. Nomor sel numerik ditolak untuk mencegah kehilangan nol awal/presisi.
- GET/POST /master/{students|teachers}/import menyediakan unggah dan pratinjau, GET .../import/template menyediakan template, POST .../import/confirm menyimpan batch. Seluruhnya hanya Admin, memerlukan sesi dan CSRF untuk mutasi. Batas request besar hanya berlaku pada dua route upload tersebut; form lain tetap 32 KiB. Temporary multipart dibersihkan setelah request.
- Pratinjau memvalidasi baris, duplikat dalam file dan potensi duplikat terhadap master. Data master belum berubah. Token acak 256 bit hanya hash-nya disimpan; staging terikat owner/jenis data, berlaku 15 menit, maksimal lima pratinjau pending per akun. Payload berisi data pribadi sementara; setelah konfirmasi atau pemeriksaan expiry payload dikosongkan. Payload pending kedaluwarsa juga dibersihkan saat pratinjau berikutnya; belum ada scheduler pembersihan di luar aktivitas impor.
- Konfirmasi membaca staging server, memeriksa role/status/expiry dan duplikat ulang di bawah advisory lock, lalu memakai saveRecord yang sama dengan form dalam satu transaksi bersama histori. Semua baris berhasil atau tidak ada yang tersimpan. Double-click/concurrent replay mengembalikan receipt batch yang sama tanpa membuat data ganda. Payload setelah sukses kosong; metadata owner/count/waktu dipertahankan.
- Impor hanya menambah data baru, tidak memperbarui master, membuat akun/role/penugasan, atau menautkan akun guru. Identifikasi duplikat adalah pengaman impor: siswa berdasarkan NIS/NISN atau nama+tanggal lahir+jenis kelamin; guru berdasarkan NIP/NUPTK terisi, atau nama+email+telepon bila kedua nomor kosong. Kandidat ambigu harus diperiksa manual, bukan digabung otomatis. Ini bukan penetapan UNIQUE baru pada NIP/NUPTK secara global.

## Kalender dan kelas aktual
- `internal/academic` memisahkan hak per operasi: tulis kalender hanya Kurikulum; tulis kelas hanya Admin; daftar/detail/histori kelas untuk Admin dan Kurikulum. Pilihan tahun dapat dibaca Admin untuk kelas; daftar semester dan histori kalender tetap Kurikulum. Pemeriksaan akun aktif/role tetap di transaksi, dengan advisory lock pada mutasi yang juga dipakai perubahan role. Sidebar/form kelas mengikuti hak Admin; Kurikulum hanya mendapat tampilan baca-saja. Histori kelas kembali ke daftar kelas agar Admin tidak diarahkan ke dashboard Kurikulum.
- Admin mencari siswa master dan membuat penempatan awal pada kelas tahun yang dipilih. Daftar calon mendukung pencarian/paginasi dan menampilkan status serta NIS/NISN; siswa dari status aktif/nonaktif sama-sama terlihat karena kebijakan belum membatasi kelayakan. Satu placement per siswa/tahun dijaga indeks UNIQUE dan transaksi; kelas harus berada pada tahun yang sama. Setiap insert menyimpan actor/waktu dan histori dalam transaksi atomik. Tidak tersedia edit/hapus/perpindahan di alur awal; Kurikulum membaca daftar/historinya tanpa melihat calon yang belum ditempatkan.
- Tahun ajaran baru otomatis mendapat Semester 1 dan Semester 2 dalam transaksi yang sama. Indeks database mencegah lebih dari satu tahun aktif atau semester aktif. Mengaktifkan tahun lain menonaktifkan semester aktif sebelumnya; semester hanya dapat diaktifkan bila tahun induknya aktif.
- Penempatan normal disimpan terpisah dari master siswa dan kelas. Tidak tersedia edit/hapus atau alur perpindahan/transisi pada tahap ini.
- Kelas memiliki ID stabil dan terikat ke tahun saat dibuat. Tingkat dibatasi 7–9 (konteks satu SMP); nama unik tanpa membedakan kapitalisasi pada tingkat+tahun. Koreksi nama/tingkat memakai version, alasan dan histori atomik. Tidak ada hapus atau pindah tahun.
- Tanggal kalender dan status penutupan belum tersedia. Penutupan menunggu pemeriksaan kewajiban yang bergantung pada penempatan/penugasan. UI menandai kesiapan operasional kelas belum tersedia.

## Batas domain tahap berikutnya
Handler menerjemahkan HTTP, sedangkan internal/auth, internal/master, dan internal/academic menangani aturan domain serta transaksi masing-masing. Domain kalender/kelas dan penempatan awal Admin tersedia. Perpindahan tengah tahun, penugasan wali/pengampu, jadwal, transisi dan pemeriksaan koreksi Final Kurikulum, absensi, jurnal, penilaian dan laporan belum diimplementasikan. Kebijakan di [PRD](PRD.md) tidak berarti modul tersebut sudah tersedia. Guru Mapel/Wali Kelas harus diperiksa terhadap role sekaligus penugasan aktif; jangan memperluas hak dari role saja.

## Basis UI
TailAdmin Free HTML 2.4.0 diadaptasi untuk shell/sidebar/header/card dan token warna. Provenance dan MIT license ada di [third_party](../third_party/tailadmin/SOURCE.md). Demo dan font eksternal tidak disertakan. Tailwind 4.3.0 dan htmx 2.0.11 dipatok dalam package-lock.json; tidak ada Alpine karena shell belum memerlukannya. Tidak ada SPA atau backend Node.

## Belum selesai
Perpindahan/transisi siswa, penugasan dan workflow akademik lanjut, CI, backup/restore, deployment, MFA/reset password mandiri, serta retensi/arsip audit dan sesi belum diimplementasikan. Tidak ada data atau angka akademik contoh.
