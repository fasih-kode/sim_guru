# Lingkungan pengembangan

## Terverifikasi pada 21 Oktober 2026
Ubuntu 26.04.1 LTS (disetujui pengguna), Go global 1.27.1 (proyek memakai toolchain 1.27.3 melalui go.mod), PostgreSQL 18.6, Node.js 22.22.1, npm 9.2.0, Git 2.53.0. RAM sekitar 6.9 GiB; ruang kosong volume proyek sekitar 268 GiB saat diperiksa. Dependensi Go dipatok di go.mod/go.sum; frontend di package.json/package-lock.json.

SSH GitHub berhasil mengenali akun fasih-kode memakai key yang sudah tersedia. Remote origin: git@github.com:fasih-kode/sim_guru.git. Pemeriksaan remote dan fetch berhasil tanpa ref; repository remote masih kosong saat diperiksa. Pada inspeksi awal branch main belum memiliki commit. Pengguna kemudian mengotorisasi baseline commit/push fondasi pada 21 Oktober 2026; gunakan git log dan git status untuk kondisi terkini.

## Database lokal proyek
Cluster di `.local/postgres/data`, socket di `.local/postgres/socket`, log di `.local/postgres/server.log`. Listen hanya 127.0.0.1:55436; PostgreSQL sistem pada 5433 tidak diubah.

```sh
scripts/dev-db.sh start
scripts/dev-db.sh status
scripts/dev-db.sh stop
```

Cluster sudah diinisialisasi dan dua database tersedia: sim_guru_dev dan sim_guru_test. Role aplikasi sim_guru_app bukan superuser dan tidak dapat membuat database/role. Autentikasi TCP memakai SCRAM; koneksi socket administrator cluster memakai peer pengguna OS fasih. Data development/test harus tetap terpisah.

`.env` berizin 0600 menyimpan kredensial acak lokal; `.env.example` hanya berisi placeholder. Aplikasi memakai godotenv: .env opsional, environment yang diekspor tetap lebih tinggi prioritasnya. Jangan mencetak atau menyalin .env ke laporan/Git. sslmode=disable hanya untuk koneksi loopback development ini.

Cluster tidak otomatis mulai setelah reboot; jalankan perintah start. Script membutuhkan pg_config dan binary PostgreSQL versi cluster. Jangan upgrade versi mayor atau menghapus .local tanpa strategi migrasi/pemulihan dan persetujuan.

## Menjalankan fondasi

```sh
scripts/dev-db.sh start
npm ci
npm run build
make migrate-up
make migrate-status
make run
```

Buka http://127.0.0.1:18743. APP_HOST default 127.0.0.1; APP_PORT default 18743. APP_ENV menerima development/test/production. DATABASE_URL wajib berupa URL PostgreSQL dengan nama database; production mensyaratkan sslmode=verify-full. File .env.example menyediakan nama variabel tanpa rahasia. Jangan menggunakan .env development pada produksi.

Untuk binary mandiri dengan template/aset embed: make build lalu ./bin/sim-guru dari root agar .env ditemukan. Aset harus dibangun sebelum binary ketika CSS/htmx berubah. Server tidak menjalankan migrasi otomatis dan menolak mulai bila database belum siap. Ctrl+C atau SIGTERM memicu graceful shutdown.

## Standar visual frontend
TailAdmin Free menjadi acuan visual default untuk semua halaman/role dan komponen baru. Sebelum implementasi, buka [demo utama](https://demo.tailadmin.com/), halaman komponen yang paling dekat (contoh: [Modals](https://demo.tailadmin.com/modals), [Basic Tables](https://demo.tailadmin.com/basic-tables)), dan [source repository](https://github.com/TailAdmin/tailadmin-free-tailwind-dashboard-template). Ikuti komposisi, ukuran/lebar, spacing, tipografi, warna, ikon, state, responsivitas, dan perilaku interaksi dari varian terdekat. Permintaan form dalam modal otomatis memakai pola Form in Modal; tidak perlu menunggu screenshot tambahan.

Adaptasi markup/style ke Go `html/template`, CSS Tailwind proyek, dan htmx yang tersedia; pertahankan alur, otorisasi, dan aturan bisnis SIM-GURU. Jangan mengganti stack atau mengimpor seluruh dashboard tanpa kebutuhan. Verifikasi secara visual pada desktop dan ponsel terhadap demo sebelum menyatakan hasil cocok. Jika demo tidak dapat diakses, telusuri komponen yang sama di source resmi; tanyakan hanya bila pilihan antarvarian memang mengubah alur produk.

Toolchain proyek memerlukan Go 1.27.3; instalasi global tidak diubah. Biarkan GOTOOLCHAIN=auto agar Go mengunduh patch yang diminta. Versi ini dipilih setelah pemeriksaan keamanan; lihat [keputusan](DECISIONS.md).

Identitas pengguna di header bersama semua role memakai dropdown TailAdmin “Dropdown with Icon”: trigger menampilkan avatar, nama, role, dan chevron; menu menyediakan tautan Kata Sandi dan aksi Keluar yang tetap memakai POST/CSRF. Menu Kata Sandi tidak diduplikasi pada sidebar; tautannya membuka modal langsung melalui `/account/password?modal=1`. Akses `/account/password` tanpa parameter tetap menyediakan tombol untuk membuka modal. Gunakan elemen `<details>` agar dropdown bisa dioperasikan dengan keyboard tanpa JavaScript khusus. Pertahankan aksi sesuai route dan otorisasi yang tersedia; jangan menambahkan tautan profil palsu.

Migrasi up/status hanya menarget DATABASE_URL. Tidak ada perintah reset/down otomatis. Jangan mengedit migrasi 00001 yang sudah diterapkan. Tambahkan migrasi berikutnya; rencana rollback harus melindungi data dan mendapat persetujuan sebelum dijalankan.

## Tabel, filter, dan pencarian
Gunakan kelas bersama `.table-surface`, `.app-table`, dan `.table-toolbar` untuk tabel SSR. Rata-kan pencarian dan tombol Filter ke kanan pada baris judul tabel. Panel Filter dibuka sebagai baris inline yang menyatu di atas tabel (bukan popover); setiap perubahan kontrol langsung menerapkan parameter GET yang divalidasi handler/service, tanpa tombol Terapkan. Aksi per baris memakai menu tiga titik dan tetap mengikuti otorisasi yang sama. Semua tabel memakai 10 baris per halaman. Tabel daftar penuh yang dimuat di halaman menggunakan `data-table-paginate` dan `data-table-search` untuk pagination serta pencarian instan client-side; tabel daftar besar memakai pagination server-side dengan ukuran halaman 10 dan mempertahankan filter/pencarian GET. Tombol halaman memakai pola TailAdmin “Pagination with Text and Icon”; nomor aktif berwarna brand dan nomor lain berubah warna saat hover. Periksa total data dan pola pemuatan sebelum memilih client-side atau server-side; jangan memuat seluruh kumpulan besar hanya demi pagination client-side.

Setiap tabel diawali kolom `No.` untuk nomor urut tampilan. Daftar server-side menghitung nomor global berdasarkan halaman (10 baris per halaman); tabel client-side menomori hasil pencarian berurutan mulai dari 1 dan mempertahankan urutan itu lintas halaman. Pada pratinjau impor, `No.` adalah urutan tampilan; kolom `Baris Excel` tetap menunjukkan nomor baris sumber.

## Belum selesai
Login, sesi, pengguna/role, audit akses, master siswa/guru/mapel, impor .xlsx, serta kalender tahun/semester dan daftar kelas tersedia. Penempatan awal siswa oleh Admin dengan daftar/histori baca-saja Kurikulum tersedia. Perpindahan/transisi siswa, wali/pengampu, jadwal dan operasional akademik lain, CI, backup/restore teruji, serta deployment belum tersedia. Kebijakan OPEN PRD belum berubah.

## Bootstrap pada mesin baru — prosedur belum dijalankan ulang
Periksa versi toolchain dan port 55436 dahulu. Instruksi ini hanya untuk direktori proyek baru yang belum memiliki .env atau .local/postgres/data; jangan jalankan pada cluster yang sudah berisi data.

```sh
mkdir -p .local/postgres/socket
chmod 700 .local/postgres
"$(pg_config --bindir)/initdb" -D .local/postgres/data --auth-local=peer --auth-host=scram-sha-256 --encoding=UTF8 --locale=C.UTF-8
scripts/dev-db.sh start
createuser -h "$PWD/.local/postgres/socket" -p 55436 --pwprompt --no-superuser --no-createdb --no-createrole sim_guru_app
createdb -h "$PWD/.local/postgres/socket" -p 55436 --owner=sim_guru_app sim_guru_dev
createdb -h "$PWD/.local/postgres/socket" -p 55436 --owner=sim_guru_app sim_guru_test
cp .env.example .env
chmod 600 .env
```

Isi kedua URL dalam .env secara lokal memakai password kuat yang dimasukkan melalui prompt createuser. Jangan memasukkan password dalam argumen shell atau Git. User OS menjadi administrator cluster melalui peer socket; role aplikasi hanya memiliki dua database proyek. Jika bootstrap gagal sebagian, inspeksi kondisi dan laporkan; jangan menghapus/reset cluster otomatis.

## Mengaktifkan autentikasi
CSRF_AUTH_KEY wajib berupa 64 digit heksadesimal acak. Key development lokal sudah dibuat tanpa ditampilkan. Pada mesin baru, ganti placeholder .env.example di .env dengan secrets.token_hex(32) melalui editor lokal atau script yang tidak mencetak key ke log. Pertahankan key antar restart; rotasi key membuat form/cookie CSRF lama tidak berlaku. Jangan menyalin key antar lingkungan.

Migrasi terbaru adalah versi 6. Jalankan make migrate-up sebelum menjalankan server versi ini. Tidak ada akun/password bawaan dan tidak ada akun development yang dibuat oleh pengujian.

Buat Admin pertama melalui terminal sendiri:

```sh
go run ./cmd/admin -username admin -name "Administrator Sekolah"
```

Password diminta dua kali tanpa echo, lalu konfirmasi BUAT. Bootstrap hanya berhasil jika tabel akun kosong; tidak mengubah password/role akun yang sudah ada. Jangan memberikan password melalui argumen, chat, environment, pipe, atau source code. Setelah bootstrap, buka http://127.0.0.1:18743/login lalu buat akun personel melalui Pengguna dan role.

Perubahan/reset password dan penonaktifan mencabut seluruh sesi target. Ganti password sendiri memerlukan password saat ini dan mengarahkan kembali ke login. Role baru langsung tampil pada sesi aktif tanpa approval; Guru Mapel/Wali Kelas belum memiliki fitur operasional sebelum modul penugasan dibuat.

Production belum dideploy: diperlukan HTTPS/reverse proxy yang benar, role DB least privilege terpisah dari migrator, prosedur backup/restore, limiter lintas instance bila relevan, serta pemeriksaan keamanan operasional. Cookie Secure hanya aktif ketika APP_ENV=production; HTTP loopback development memakai cookie non-Secure agar browser dapat mengirimkannya.

## Menggunakan master data
Admin membuka Data Siswa/Data Guru pada sidebar. Tambah, cari, filter status, lihat detail, edit dan nonaktif tersedia; koreksi/status memerlukan alasan dan dapat ditinjau melalui Histori perubahan. Tidak ada penghapusan. Guru dapat ditautkan ke akun yang sudah ada, tanpa mengubah role atau status akun tersebut.

Kurikulum membuka Siswa/Guru baca-saja (Hubungi Admin untuk koreksi) serta Mata Pelajaran untuk tambah/edit/nonaktif mapel. Admin tanpa role Kurikulum tidak otomatis memperoleh menu/hak mapel. Bila orang yang sama menjalankan kedua tugas, pemberian multi-role tetap dilakukan secara eksplisit pada administrasi akun.

Jalankan make migrate-up sebelum binary terbaru; versi 3 menambah tabel master/histori, versi 4 staging impor, versi 5 kalender/kelas/histori akademik, dan versi 6 penempatan awal serta histori. Migration 00006 aditif; tidak mengubah migration 00001–00005 atau menulis ulang akun, siswa, tahun, semester dan kelas. Jangan mengedit migrasi yang sudah diterapkan. Data contoh pengujian hanya ada di database test. Akun Admin pengguna dan port 18743 tetap digunakan.

## Penempatan siswa oleh Admin
Pada menu Administrasi → Penempatan Siswa, pilih tahun dan kelas, cari nama/NIS/NISN, lalu pilih Tempatkan. Daftar ditampilkan 10 baris per halaman dan dapat dipaginasi. Satu siswa hanya dapat memiliki satu penempatan normal per tahun ajaran; duplikat ditolak database meskipun beberapa Admin menyimpan bersamaan. Status master siswa ditampilkan untuk pemeriksaan, tetapi aplikasi belum melarang penempatan siswa nonaktif karena PRD belum menentukan aturan tersebut.

Kurikulum dapat membaca daftar dan histori penempatan. Histori menyimpan pelaku, waktu, dan kelas tujuan awal. Form normal tidak mengedit/menghapus penempatan atau mencatat perpindahan/transisi; alur tersebut menunggu modul khusus dan ketentuan PRD yang relevan. Pembuatan penempatan memerlukan role Admin dan CSRF; jangan mengubah data melalui SQL manual.

## Impor Excel siswa/guru
1. Masuk sebagai Admin pada port 18743. Buka Data Siswa atau Data Guru, pilih Impor Excel, lalu Unduh template Excel.
2. Isi sheet Data mulai baris 2; jangan mengubah header/urutan kolom. Sheet Petunjuk tidak diimpor. Baris kosong diabaikan. Tidak ada contoh data fiktif dalam template.
3. Simpan nomor identitas/telepon sebagai teks sebelum mengisi/menempel. Nomor Excel yang sudah dibulatkan atau kehilangan nol awal tidak dapat dipulihkan oleh aplikasi. Tanggal lahir dapat berupa tanggal Excel atau teks YYYY-MM-DD; jenis kelamin L/P; aktif YA/TIDAK, kosong berarti YA.
4. Unggah .xlsx, maksimal 5 MiB dan 500 baris. Periksa seluruh pratinjau. Baris salah/potensi duplikat ditandai dengan nomor baris Excel; perbaiki file lalu unggah ulang. Tidak ada baris master tersimpan pada tahap ini.
5. Konfirmasi dalam 15 menit dari akun yang sama. Semua baris beserta histori disimpan dalam satu transaksi; konflik setelah pratinjau membatalkan seluruh batch. Mengulang konfirmasi tidak menggandakan data.

Header siswa: nama, tanggal_lahir, jenis_kelamin, nis, nisn, telepon, aktif.
Header guru: nama, nip, nuptk, email, telepon, aktif.

Impor hanya menambah data baru. Untuk koreksi identitas gunakan formulir edit beralasan. Akun/role/penugasan dan tautan akun guru tidak diimpor. Nama/nomor yang cocok hanya dianggap kandidat duplikat, tidak menjadi dasar overwrite/merge otomatis. NIS/NISN tetap opsional dan unik bila diisi; input siswa tanpa nomor yang identitasnya sama perlu ditinjau agar tidak terduplikasi.

File .xls lama harus disimpan ulang sebagai .xlsx sesuai template; CSV, workbook terenkripsi, macro, formula dan tautan workbook eksternal tidak didukung. Tidak ada pemetaan header bebas atau mode update massal. Pratinjau maksimal lima yang aktif per akun. Payload staging dibersihkan setelah selesai/expiry diperiksa atau pada pratinjau berikutnya; belum ada job pembersihan berkala.

Template versi aplikasi berada di internal/web/import_templates. Saat mengubah formatnya, perbarui ImportHeaders/parser, petunjuk dan tes template secara bersamaan. Jangan memakai file berisi data pribadi sebagai template repository. Baca TESTING sebelum memperbarui versi library Excel.

## Tahun ajaran, semester, dan kelas
Role Kurikulum membuka Akademik, menyiapkan tahun ajaran beserta dua semester serta memilih kalender aktif. Admin membuka Kelas pada Administrasi untuk membuat/mengoreksi kelas; Kurikulum membuka Kelas (baca-saja) untuk daftar dan histori. Admin tanpa Kurikulum tidak dapat membuka halaman/tindakan kalender. Buat tahun ajaran; aplikasi membuat Semester 1 dan Semester 2 otomatis. Pilih satu tahun aktif lalu satu semester di tahun tersebut. Mengaktifkan tahun lain menonaktifkan semester aktif sebelumnya. Tahun/semester tidak dapat dihapus atau ditutup pada tahap ini.

Sebagai Admin, buat kelas dengan memilih tahun ajaran yang telah disiapkan Kurikulum, tingkat 7/8/9, dan nama rombongan (misalnya A; daftar menampilkan 7A). Nama unik dalam tingkat+tahun tanpa membedakan kapitalisasi. Saat koreksi tingkat/nama diperlukan alasan. ID kelas tidak berubah dan kelas tidak dapat dipindah ke tahun lain; buat identitas kelas baru untuk tahun lain. Aplikasi menandai kelas belum siap operasional sampai penempatan siswa dan penetapan wali kelas tersedia.


## Lebar area kerja
Konten halaman memakai seluruh lebar yang tersedia setelah sidebar dan padding halaman diperhitungkan. Jangan menambahkan batas `max-width` global pada `#content`; formulir sempit sebaiknya memakai batas lebar per komponen.

### Tombol dengan ikon pembuka
Semua tombol aksi berlabel yang memakai `.button-primary` atau `.button-secondary` menggunakan ikon SVG kecil di sebelah kiri teks, mengikuti varian “Primary Button with Leading Icon” dan “Secondary Button with Leading Icon” pada [demo Buttons TailAdmin](https://demo.tailadmin.com/buttons). Ikon umum didefinisikan sekali pada template bersama `page.html`; pilih ikon sesuai aksi. Kontrol yang memang ikon-saja seperti tutup dialog, toggle sidebar, filter, pencarian, dan menu titik tiga tidak diubah menjadi tombol berlabel.

### Badge status berlatar muda
Status operasional menggunakan komponen `.status-badge` dengan latar muda mengikuti bagian “With Light Background” pada [demo Badge TailAdmin](https://demo.tailadmin.com/badge). Aktif/valid memakai hijau, nonaktif netral, dan hasil impor bermasalah memakai merah; label tetap ditampilkan agar status tidak hanya dibedakan oleh warna. Terapkan pada seluruh role dan halaman yang menampilkan status data.

### Grup tombol aksi
Tombol berdekatan yang membentuk satu rangkaian aksi ditampilkan sebagai `.button-group button-group--primary`, mengikuti [Primary Button Group with Left Icon TailAdmin](https://demo.tailadmin.com/buttons-group). Setiap item tetap memiliki ikon pembuka, grup memakai bidang primary tersambung dengan pemisah halus dan radius hanya di ujung. Terapkan pada aksi header data, aksi detail, dan aksi submit/batal dalam modal; jangan gabungkan tombol yang berada pada bagian/alur berbeda.

Pada `.button-group--primary`, tiap segmen berwarna putih dengan teks dan border biru saat idle. Segmen yang di-hover atau menerima fokus keyboard berubah menjadi latar biru dan teks putih, sesuai state interaktif demo; warna hover tidak dipertahankan sebagai state terpilih setelah pointer berpindah.

### Notification aplikasi
Pesan keberhasilan/kesalahan aplikasi menggunakan pola notifikasi TailAdmin: kartu putih, ikon berwarna, tombol tutup, dan garis waktu berwarna. Notification hilang otomatis setelah 10 detik; tombol tutup menutupnya lebih awal. Pesan kesalahan yang terjadi pada form tetap muncul sebagai notification di dalam modal/form, sedangkan rincian kesalahan validasi impor per baris tetap berada di pratinjau agar dapat diperbaiki. Implementasi bersama ada pada template `page.html`, CSS `web/styles/app.css`, dan timer/penutupan `internal/web/static/app.js`.

Semua notification diratakan ke sisi kanan kontainer masing-masing dengan `margin-inline-start:auto`, termasuk notification di dalam modal.
