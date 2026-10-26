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

Sidebar Kurikulum dibagi menjadi dropdown **Kalender Akademik**, **Struktur Kurikulum**, **Pengaturan Jadwal**, dan **Data dan Penempatan Siswa** mengikuti pola dropdown sidebar TailAdmin. Saat membuka halaman dalam kelompok, kelompok terkait tetap terbuka dan tautan halaman aktif ditandai. Jangan mengubah hak role/tautan ketika merapikan navigasi.

Toolchain proyek memerlukan Go 1.27.3; instalasi global tidak diubah. Biarkan GOTOOLCHAIN=auto agar Go mengunduh patch yang diminta. Versi ini dipilih setelah pemeriksaan keamanan; lihat [keputusan](DECISIONS.md).

Identitas pengguna di header bersama semua role memakai dropdown TailAdmin “Dropdown with Icon”: trigger menampilkan avatar, nama, role, dan chevron; menu menyediakan tautan Kata Sandi dan aksi Keluar yang tetap memakai POST/CSRF. Menu Kata Sandi tidak diduplikasi pada sidebar; tautannya membuka modal langsung melalui `/account/password?modal=1`. Akses `/account/password` tanpa parameter tetap menyediakan tombol untuk membuka modal. Gunakan elemen `<details>` agar dropdown bisa dioperasikan dengan keyboard tanpa JavaScript khusus. Pertahankan aksi sesuai route dan otorisasi yang tersedia; jangan menambahkan tautan profil palsu.

Migrasi up/status hanya menarget DATABASE_URL. Tidak ada perintah reset/down otomatis. Jangan mengedit migrasi 00001 yang sudah diterapkan. Tambahkan migrasi berikutnya; rencana rollback harus melindungi data dan mendapat persetujuan sebelum dijalankan.

## Tabel, filter, dan pencarian
Gunakan kelas bersama `.table-surface`, `.app-table`, dan `.table-toolbar` untuk tabel SSR. Rata-kan pencarian dan tombol Filter ke kanan pada baris judul tabel. Panel Filter dibuka sebagai baris inline yang menyatu di atas tabel (bukan popover); setiap perubahan kontrol langsung menerapkan parameter GET yang divalidasi handler/service, tanpa tombol Terapkan. Aksi per baris memakai menu tiga titik dan tetap mengikuti otorisasi yang sama. Semua tabel memakai 10 baris per halaman. Tabel daftar penuh yang dimuat di halaman menggunakan `data-table-paginate` dan `data-table-search` untuk pagination serta pencarian instan client-side; tabel daftar besar memakai pagination server-side dengan ukuran halaman 10 dan mempertahankan filter/pencarian GET. Tombol halaman memakai pola TailAdmin “Pagination with Text and Icon”; nomor aktif berwarna brand dan nomor lain berubah warna saat hover. Periksa total data dan pola pemuatan sebelum memilih client-side atau server-side; jangan memuat seluruh kumpulan besar hanya demi pagination client-side.

Setiap tabel diawali kolom `No.` untuk nomor urut tampilan. Daftar server-side menghitung nomor global berdasarkan halaman (10 baris per halaman); tabel client-side menomori hasil pencarian berurutan mulai dari 1 dan mempertahankan urutan itu lintas halaman. Pada pratinjau impor, `No.` adalah urutan tampilan; kolom `Baris Excel` tetap menunjukkan nomor baris sumber.

Halaman **Jadwal Saya** memakai filter Hari, Kelas, dan Mata Pelajaran di atas tabel. Opsi kelas/mapel berasal dari jadwal terbit yang server tampilkan untuk guru yang sedang masuk; filter tetap berjalan pada data terbatas tersebut dan pilihan disimpan sementara di sesi browser. Jangan mengisi opsi filter dari seluruh kelas/mapel sekolah karena halaman ini hanya boleh menampilkan penugasan guru yang sedang masuk.

## Belum selesai
Login, sesi, pengguna/role, audit akses, master siswa/guru/mapel, impor .xlsx, kalender tahun/semester, daftar kelas, penempatan awal siswa oleh Admin, wali kelas/roster terbatas, pengampu, jadwal/generator, impor jadwal XML, serta transisi akhir tahun Kurikulum tersedia. Kurikulum membuat tahun baru beserta dua semester dalam keadaan belum aktif; Admin membuat kelas untuk tahun itu; Kurikulum menjalankan transisi ketika tahun lama masih aktif, lalu mengaktifkan tahun dan semester baru. Perpindahan kelas tengah tahun Kurikulum dan pindah sekolah keluar Admin tersedia sesuai D-031. CI, backup/restore teruji, operasional akademik lanjut dan deployment juga belum tersedia.

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

Migration terbaru adalah versi 18. Migration 00018 menambah fondasi kalender bertanggal dan tanggal mulai penempatan, tanpa mengisi tanggal lama otomatis. Jalankan `make migrate-up` sebelum menjalankan binary terbaru. Migration 00017 mengarsipkan kisi jadwal yang dihapus tanpa membuang histori dan mengizinkan kisi aktif baru untuk sasaran yang sama. Tidak ada akun/password bawaan dan tidak ada akun development yang dibuat oleh pengujian.

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

Jalankan `make migrate-up` sebelum binary terbaru. Migration 00003–00006 menambah master, staging impor, kalender/kelas, dan penempatan awal; migration 00007–00018 menambah fitur akademik, jadwal, perpindahan/transisi siswa, pengarsipan kisi, serta fondasi absensi bertanggal. Migration bersifat aditif terhadap data historis; jangan mengedit migrasi yang sudah diterapkan. Data contoh pengujian hanya ada di database test. Akun Admin pengguna dan port 18743 tetap digunakan.

## Penempatan siswa oleh Admin
Pada menu Administrasi → Penempatan Siswa, pilih tahun dan kelas, cari nama/NIS/NISN, lalu pilih Tempatkan. Daftar ditampilkan 10 baris per halaman dan dapat dipaginasi. Satu siswa hanya dapat memiliki satu penempatan normal per tahun ajaran; duplikat ditolak database meskipun beberapa Admin menyimpan bersamaan. Status master siswa ditampilkan untuk pemeriksaan, tetapi aplikasi belum melarang penempatan siswa nonaktif karena PRD belum menentukan aturan tersebut.

Kurikulum dapat membaca daftar dan histori penempatan. Histori menyimpan pelaku, waktu, dan kelas tujuan awal. Penempatan awal memerlukan role Admin. Untuk akhir tahun, urutannya: Kurikulum membuat tahun tujuan (otomatis dengan dua semester, belum aktif) → Admin membuat kelas tujuan → Kurikulum membuka **Transisi Siswa** saat tahun sumber masih aktif, memilih tahun tujuan, menentukan hasil tiap siswa (naik/tidak naik beserta kelas tujuan; lulus/keluar tanpa kelas tujuan), memeriksa pratinjau, lalu mengonfirmasi seluruh batch → Kurikulum mengaktifkan tahun dan semester baru. Layanan memvalidasi ulang dan menulis penempatan baru, catatan transisi, serta histori secara atomik. Satu siswa hanya diproses sekali dari satu tahun sumber; batch yang gagal tidak menyisakan perubahan parsial. Transisi akhir tahun tidak mengedit/menghapus penempatan lama. Perpindahan tengah tahun kini tersedia sesuai D-031. Semua operasi tetap melalui layanan berotorisasi/CSRF; jangan mengubah data melalui SQL manual.

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

## Template jam dan pilihan jadwal
Acuan D-021: Jam Pelajaran menampilkan Senin–Sabtu, 12 slot, dengan input waktu dalam modal TailAdmin bersama. Data lama Minggu/nomor >12 tetap ditampilkan bila ada. Form mengirim satu hari; gunakan `SaveLessonDay`, bukan loop transaksi per baris. Baris kosong adalah no-op, bukan penghapusan. Jangan memakai slot tanpa waktu untuk entri jadwal. Jadwal menggunakan opsi assignment pada tanggal draf; dropdown guru baca-saja, server menghitung guru dan mengabaikan teacher_id kiriman. Template sumber opsi di `<template>` diekstrak ke dropdown oleh `app.js`; JavaScript diperlukan untuk alur pilihan ini. Daftar ringkasan tetap 10 baris/halaman, sedangkan input 12 slot berada bersama dalam modal.

Dropdown navigasi/filter (termasuk hari dan semester akademik) wajib memakai `data-instant-filter` dan listener di `app.js`. Jangan memakai `onchange`/event inline: CSP `script-src 'self'` memblokirnya. Uji pergantian pilihan sampai navigasi GET dan pembaruan judul/modal, bukan hanya rendering fixture.


## Pengaturan dan generate jadwal — fase 7A
Setelah migration 00010, Kurikulum membuka `/akademik/struktur-kurikulum` untuk aturan JP/pola bawaan tingkat atau khusus rombel. JP 0 dengan pola kosong berarti tidak diajarkan. `/akademik/waktu-sekolah` mengatur istirahat dan kegiatan seluruh rombel: istirahat tanpa nomor jam, kegiatan dengan nomor/waktu yang sudah didefinisikan di Jam Pelajaran. Jeda antarjam perlu ditandai eksplisit sebagai istirahat. Jangan membuat guru/mapel palsu untuk Upacara.
Di `/akademik/jadwal`, pilih Generate jadwal, periksa tanggal dan pengaturan, lalu simpan hasil ke draf. Draf berisi pelajaran perlu konfirmasi sebelum diganti; perubahan konfigurasi memerlukan generate ulang dan publikasi selalu memeriksa ulang server. Menu Edit pada baris draf mendukung koreksi dengan validasi pola/JP/bentrok; versi terbit dapat dibuka melalui Lihat versi tanpa mengubahnya menjadi draf. Pertemuan boleh dilanjutkan sesudah istirahat; jeda tidak menjadi JP. Generator memakai pengampu resmi dan batasan wajib/preferensi yang diatur di Batasan Jadwal (migration 00011). Pencarian Go dibatasi 8 detik, tidak menjamin optimal/selalu menemukan solusi, dan tidak memublikasikan otomatis.
Migrasi lama dipertahankan; versi terbit dan jurnal tidak ditulis ulang. Migration 00010 tidak menyediakan downgrade yang menghapus histori: siapkan backup dan strategi pemulihan eksplisit. Konfigurasi awal/draf development sudah disiapkan satu kali sesuai HANDOVER; jangan menjalankan ulang helper lokal atau melakukan seed/reset database.


## Mengatur batasan jadwal — fase 7B
Jalankan migration aditif 00011 sebelum binary terbaru, dengan backup database resmi. Kurikulum membuka `/akademik/batasan-jadwal`, memilih semester, Tambah batasan → Guru/Mapel → hari → waktu aktual/sepanjang hari → Tidak tersedia atau Usahakan dihindari. Waktu berbeda pada hari lain dibuat sebagai aturan terpisah. Tabel kapasitas memakai pengampu efektif hari ini; modal generator menghitung pada tanggal berlaku draf. Mapel berlaku seluruh rombel. Tidak ada seed batasan contoh.

Buka Jadwal Mingguan → Generate jadwal, periksa daftar wajib/preferensi dan kapasitas → generate ke draf → tinjau kesiapan JP dan laporan preferensi → edit atau generate ulang → Publikasikan. Konfigurasi berubah meminta generate ulang. Edit/hapus batasan mencatat histori dan tidak mengubah versi terbit. Hasil terbaik terbatas bukan jaminan optimal; tanpa hasil lengkap, draf tetap. Batasan wajib yang bertentangan dengan beban perlu diperbaiki pengguna; aplikasi tidak mengubahnya menjadi preferensi secara otomatis.


## Kisi Batasan Jadwal — fase 7C
Setelah backup database resmi dan migration 00012, muat ulang aplikasi. Kurikulum → Batasan Jadwal → semester → Guru/Mapel → nama → Atur kisi. Pilih Tersedia/Kondisional/Tidak tersedia, klik sel atau tombol hari, lalu Simpan kisi. Semua Tersedia adalah reset editor; perubahan belum berlaku hingga disimpan. Slot Upacara/belum diatur terkunci. Waktu nyata setiap sel mengikuti Jam Pelajaran hari terkait. Form memerlukan JavaScript seperti komponen jadwal proyek lainnya.
Jika masih memakai rentang lama, tinjau pemetaan dan centang persetujuan. Jika jam/blok berubah, tinjau ulang kisi yang terdampak sebelum generate. Setelah penyimpanan, generate ulang draf dan periksa laporan sebelum publikasi. Catatan waktu lama tetap tersimpan tetapi tidak mengalahkan kisi sasaran; tidak menjalankan konversi/seed helper otomatis. HTTP formulir lama masih diterima bagi sasaran yang belum mempunyai kisi; setelah memakai kisi, backend menolak penulisan format lama terhadap sasaran tersebut.

Tabel Kisi tersimpan menyediakan Edit dan Hapus. Hapus memakai POST/CSRF dan versi optimistis, mengarsipkan kisi serta histori, dan menonaktifkan aturan format waktu lama yang pernah digantikan kisi tersebut dalam transaksi yang sama agar tidak aktif kembali. Migration 00017 menambah `deleted_at` dan indeks unik hanya untuk kisi aktif; versi jadwal terbit tetap immutable. Setelah migration, kisi untuk sasaran yang sama dapat dibuat ulang.


## Impor jadwal XML aSc — fase 7D
Backup cluster resmi lalu jalankan migration 00013 sebelum binary terbaru. Kurikulum → Jadwal Mingguan → Impor XML → pilih semester/tanggal (tanggal draf dikunci jika ada) → unggah ekspor aSc Timetables 2012 XML maksimal 5 MiB → cocokkan kelas/guru/mapel/hari/nomor jam per hari → Periksa pratinjau → Simpan sebagai draf. Saran nama unik perlu ditinjau. Jam tujuan harus berurutan untuk tiap pertemuan; blok Upacara/istirahat tidak ditawarkan.

Konfirmasi menampilkan persetujuan mengganti draf, memakai waktu SIM-GURU bila berbeda dari XML, dan menyimpan draf belum lengkap bila hanya sebagian target sekolah yang tercakup. Blok pelajaran terpotong atau pola/bentrok/larangan wajib tidak sesuai harus diperbaiki, bukan dipaksakan. Perubahan data setelah pratinjau meminta pemeriksaan ulang; konfirmasi ulang tidak menggandakan entri. Publikasi tetap manual dan memeriksa seluruh kebutuhan. Unggahan sementara 30 menit/hanya pengunggah, maksimal lima aktif. Tidak menjalankan impor contoh otomatis; data ruang/kontak/siswa bukan data master dari impor ini. Cakupan format awal dan varian yang belum didukung pada PRD 6.4.


### Pilihan hasil dan kelas tujuan transisi — disetujui 23 Oktober 2026
Setiap siswa dimulai dengan **Pilih hasil**, tanpa keputusan otomatis. Kelas 7/8 memiliki hasil Naik kelas (tujuan tingkat berikutnya), Tidak naik (tujuan tingkat sama), atau Keluar (tanpa tujuan). Kelas 9 hanya memiliki Lulus (tanpa tujuan), Tidak lulus — mengulang kelas 9 (wajib kelas 9 tahun tujuan), atau Keluar (tanpa tujuan). Dropdown tujuan muncul hanya untuk naik/mengulang dan hanya menawarkan tingkat yang sesuai. Perubahan ke Lulus/Keluar mengosongkan pilihan kelas lama; pratinjau menggunakan label bahasa Indonesia dan tanda — untuk tujuan yang tidak berlaku. Server memeriksa ulang aturan yang sama. Semua keputusan siswa harus ditetapkan sebelum menjalankan batch; keputusan yang belum ditetapkan tidak dianggap naik/lulus. Tidak membuat kelas 10, tidak menghapus riwayat, dan tidak mengubah status master siswa otomatis.

## Perpindahan siswa — D-031
Kurikulum: Penempatan (baca-saja untuk penempatan awal) → Aksi → Pindah kelas. Admin: Data Siswa → Aksi → Pindah sekolah. Kedua form memakai modal terpusat/blur, ringkasan lalu konfirmasi. Riwayat perpindahan tersedia dari aksi master siswa dan daftar penempatan. Tahun/tingkat kelas tujuan harus sama, tanggal berlaku dan alasan wajib, sekolah tujuan opsional. Perubahan berlaku berdasarkan tanggal WIB, bukan waktu server browser. Migration 00015 menambah `student_class_moves`, `student_school_exits`, dan `current_student_placements`. Penempatan awal tetap tersimpan; query daftar/jumlah/roster/transisi memakai penempatan efektif. Master siswa menghitung status Pindah sekolah/tanggal terjadwal dari catatan keluar; edit identitas tidak dapat membatalkannya.

Satu perpindahan kelas mendatang diperbolehkan; catatan berikutnya harus setelah catatan terakhir. Pengaman menolak konflik tanggal, sumber kelas yang sudah berubah, duplikat keluar, serta mutasi yang bertentangan dengan transisi akhir tahun. Catatan keluar yang belum ditutup oleh penerimaan kembali efektif juga menahan perpindahan kelas baru agar histori kelas asal keluar tidak kedaluwarsa; susun perpindahan kelas sebelum mencatat keluar. Pindah sekolah terjadwal sudah dikecualikan dari calon penempatan/transisi. Pending pindah kelas menahan transisi sampai berlaku. Riwayat awal dan kejadian terpisah tetap dapat dibaca; daftar aktif bukan laporan historis per tanggal. Terima kembali siswa lama kini tersedia sesuai D-032; pembatalan/koreksi kejadian dan penerimaan pindahan baru tetap belum tersedia. Kalender belum punya tanggal awal/akhir tahun; petugas memastikan tanggal sesuai tahun. Jangan menghapus/mengubah kejadian memakai SQL manual sebagai alur operasional.

## Terima kembali siswa lama — D-032
Admin → Data Siswa → cari siswa berstatus Pindah sekolah → Aksi Terima kembali → kelas tahun aktif, tanggal kembali, alasan dan sekolah asal terakhir opsional → ringkasan/konfirmasi → Simpan penerimaan. Siswa tetap menggunakan ID/NIS/NISN lama. Tahun yang sama dibatasi tingkat asal jika tercatat; tahun berbeda tidak otomatis naik tingkat. Pilih kelas berdasarkan riwayat akademik dan mintalah Kurikulum memastikan tingkat bila diperlukan.

Migration 00016 menambah `student_school_returns`, melepas batas satu kejadian keluar per siswa dan menggantinya dengan unik siswa–tanggal, serta menambah view `outstanding_student_school_exits` dan fungsi `student_placement_class_at(placement_id,date)`. Fungsi mengembalikan NULL selama masa keluar dan sebelum penempatan baru hasil penerimaan terjadwal mulai berlaku. `current_student_placements` menggunakan fungsi tersebut; query status master, calon penempatan, perpindahan dan transisi memakai kejadian keluar yang masih terbuka. Histori keluar/kembali tidak dihapus. Penerimaan pada tahun yang sama memakai placement asli; tahun berikutnya membuat placement/histori hanya jika belum ada. Seluruhnya satu transaksi, dengan role Admin, mutation lock, pemeriksaan versi dan satu penerimaan untuk tiap kejadian keluar.

Tanggal kembali harus setelah tanggal keluar; siswa harus sudah keluar saat penerimaan dicatat. Pengiriman ganda, tingkat salah pada tahun sama, kelas tahun nonaktif, versi stale, penempatan tingkat berbeda, dan transisi tahun aktif yang sudah dijalankan ditolak. Penerimaan mendatang berstatus Kembali terjadwal, belum masuk roster hingga berlaku. Pending penerimaan menahan transisi akhir tahun. Setelah kembali efektif, siswa dapat pindah kelas/keluar lagi dan ikut transisi. Pembatalan/koreksi kejadian serta pindahan baru bukan bagian fitur ini.

## Fase 8 urutan 1 — kalender dan tanggal penempatan
Jalankan migration aditif 00018 setelah backup cluster resmi. Kurikulum → Kalender Akademik → Kalender Sekolah: pilih semester, atur rentang/hari, kemudian libur/hari sekolah khusus; aksi reset tanggal mengembalikan pola mingguan. Admin → Penempatan → Tanggal Mulai Penempatan: isi tanggal sebenarnya untuk data lama, atau koreksi dengan alasan. Form penempatan baru wajib tanggal. Jangan mengisi tanggal lama otomatis dari created_at atau nama tahun. Rekaman tanpa tanggal tetap terlihat di daftar operasional lama; konteks absensi bertanggal menolak tanggal yang belum lengkap. Pengaturan tanggal mengubah keikutsertaan roster, bukan memindahkan kelas. Fitur absensi operasional dilanjutkan sesuai PRD 7.5.

### Mengisi tanggal penempatan lama secara massal
Admin → Penempatan Siswa → Tanggal mulai penempatan → Isi tanggal massal. Pilih kelas awal/semua kelas, periksa saran tanggal awal Semester 1, isi alasan, keluarkan siswa dengan tanggal masuk berbeda, lalu Periksa pratinjau. Pilihan tetap mencakup halaman lain meskipun pencarian menyembunyikan baris. Periksa daftar lengkap, centang konfirmasi dan Simpan tanggal. Jika konfigurasi/data berubah atau lewat 10 menit, periksa ulang. Tanggal yang sudah terisi tidak ditimpa; perbaikan tanggal lama tetap melalui Atur tanggal individual. Maksimum 5.000 siswa per batch. Penempatan baru mendapat saran semester yang dapat diganti; tanpa kalender tanggal tetap kosong.

### Petugas: harian siswa, QR dan koreksi
1. Kurikulum melengkapi kalender; Admin melengkapi tanggal mulai penempatan. Akun operator harus memiliki role **Petugas Absensi**.
2. **Kartu QR Siswa → Kelola kartu**: terbitkan pertama dengan alasan/konfirmasi, lalu Cetak kartu. Untuk hilang pilih Ganti QR karena hilang; kode lama langsung tidak berlaku. Untuk rusak yang masih dipegang pilih Cetak ulang (kode sama). Catatan siswa/absensi tetap sama.
3. **Pemindai QR → Mulai kamera**: berikan izin pada HTTPS (localhost boleh untuk pengembangan); arahkan kartu. Periksa nama/kelas/waktu hasil dan tekan Scan siswa berikutnya. Pembaca USB bisa mengisi kotak kode lalu Enter. Tidak ada antrean offline; bila jaringan gagal, periksa daftar sebelum mencoba lagi.
4. **Absensi Harian**: pilih tanggal dan tahun/kelas. Status kosong tetap Belum Dicatat. Catat manual dengan alasan saat kartu/perangkat bermasalah; Sesuai D-036, Hadir wajib berjam datang sebenarnya; catatan lama tanpa jam perlu dilengkapi berdasarkan bukti. Isi Sakit/Izin/Alpa sesuai bukti. Tidak mencatat tanggal mendatang/libur.
5. **Koreksi / histori**: periksa nilai lama, isi nilai baru dan alasan, Simpan. Konflik versi meminta pemeriksaan ulang; tidak ada persetujuan Kepala Sekolah. Wali Kelas/Kesiswaan/Kepsek membaca sesuai cakupan tanpa hak mengubah.
Kamera ponsel melalui alamat IP HTTP biasa tidak memenuhi secure context; gunakan HTTPS pada deployment. Kartu dan salinan/backup database bersifat privat. Jadwal belajar tidak diperlukan untuk absensi harian; hanya kalender dan penempatan bertanggal. Absensi pelajaran menyusul.

### Membuka development dari HP satu Wi-Fi
Hentikan proses server yang memakai port 18743 terlebih dahulu, lalu jalankan `APP_HOST=0.0.0.0 make run` dari root repository. Komputer tetap dapat memakai localhost; HP memakai alamat IPv4 Wi-Fi komputer (lihat `ip -brief -4 address show`), misalnya `http://192.168.18.8:18743/`. Localhost pada HP menunjuk HP sendiri. Jika tidak tersambung, periksa apakah kedua perangkat satu jaringan dan apakah firewall/router mengisolasi klien Wi-Fi. Perintah ini hanya mengganti binding runtime; konfigurasi `.env` tetap. Kamera HP memerlukan HTTPS meskipun halaman HTTP LAN sudah dapat dibuka.

### Batas kedatangan dan isi cepat — D-036
Kesiswaan → Batas Kedatangan → pilih semester → tambah hari mingguan/pengecualian tanggal dan semua/rombel tertentu → mulai berlaku, batas HH:MM, toleransi, alasan → Simpan versi. Tidak ada jam masuk bawaan. Untuk cakupan yang sudah ada gunakan Ubah versi; versi terdahulu tersimpan dan bukti absensi tidak dihitung ulang. Prioritas tanggal+rombel, tanggal semua, mingguan rombel, mingguan semua. Pada cakupan sama pilih tanggal efektif terbaru lalu versi terakhir. Ketelitian menit; tepat di batas+toleransi masih tepat waktu, durasi terlambat dari batas asli.
Petugas → Absensi Harian → Isi cepat per kelas: H/S/I/A langsung pada daftar seluruh siswa; Tandai sisanya Hadir merupakan aksi eksplisit. Isi waktu sebenarnya (wajib untuk Hadir), gunakan jam bersama hanya jika diketahui dan sesuaikan per baris, pilih alasan kelompok, periksa ringkasan lalu Simpan. Baris tersimpan terkunci; koreksi tetap individual. Pencarian tidak menghapus pilihan, navigasi diperingatkan bila belum disimpan. Konflik meminta tinjau ulang dengan isian tetap. Catat datang sekarang pada aksi siswa hanya untuk pencatatan langsung hari ini, memakai jam server. Batas belum diatur tetap merekam kedatangan dengan label belum dapat dinilai; data lama tanpa jam menunjukkan Waktu belum lengkap. Tidak membuat waktu rekaan/sanksi otomatis.
## DEV-03C1 — fondasi penyimpanan foto selfie privat

Package `internal/teacherphoto` menyediakan `NewStore(Config)`, `Save`, `Open`, `Delete`, `CleanupTemps`, dan `Close`. Direktori storage wajib diberikan eksplisit; package belum membaca environment, tidak memiliki default path, dan belum dirangkai ke konfigurasi server. `Directory` harus absolut, kanonik, di luar `RepositoryRoot` dan semua `PublicRoots`; parent harus sudah ada, tidak berupa symlink, serta tidak writable oleh group/others. Direktori storage harus berizin `0700`; file harus `0600`. Komponen menggunakan `os.Root` agar operasi file dibatasi pada direktori yang dibuka. Untuk integrasi/deployment, siapkan path privat di luar repository, static assets, dan root Nginx, lalu kirim root yang relevan ke konfigurasi. Jangan menaruh foto di repository atau direktori publik.

`Save` saat ini menerima JPEG/PNG dengan MIME yang cocok dengan format hasil decode, membatasi input mentah 5 MiB, sisi gambar 4096 px dan total 8 juta piksel, lalu mengode ulang sebagai JPEG untuk membuang metadata. Ukuran JPEG final maksimum 102400 byte; kompresi mencoba kualitas bertingkat tanpa mengubah dimensi, lalu menolak gambar yang tetap terlalu besar. PNG diterima karena decoder/encoder Go standard library sudah cukup; WebP belum didukung dan tidak ada dependency baru. Metadata hasil berisi key acak 256-bit dalam hex, `image/jpeg`, ukuran final, dan SHA-256 byte final. Nama file dari pengguna tidak digunakan.

Publikasi file menggunakan temporary file dan hard link tanpa overwrite di direktori yang sama; file sementara dibuang setelah berhasil atau bila operasi gagal. Isi file di-`Sync` sebelum publikasi sehingga nama final hanya terlihat setelah JPEG lengkap ditulis. Entri direktori tidak di-`Sync`, jadi ini menjamin publikasi atomik terhadap pembaca berjalan, bukan ketahanan nama final saat kehilangan daya mendadak. `Save` dan `CleanupTemps` diserialkan pada instance `Store` yang sama, sehingga cleanup tidak menghapus temporary file dari simpanan aktif pada instance tersebut. Koordinasi tidak mencakup beberapa instance/proses yang membuka direktori sama; gunakan satu instance per direktori dan jangan menjalankan cleanup lintas proses bersamaan dengan penyimpanan. `CleanupTemps` hanya menghapus nama temporary valid yang lebih tua daripada umur positif yang diberikan; pemanggil masa depan harus menentukan jadwal pemanggilan. `Delete` idempoten untuk file yang sudah tidak ada. Operasi selain `Close` sesudah `Close` mengembalikan `ErrClosed`; `Close` berulang aman. Belum ada worker retensi, endpoint baca, upload, event, atau INSERT metadata.

Pada integrasi DEV-03C2, filesystem dan PostgreSQL tidak dapat menjadi satu transaksi atomik. Simpan/normalisasi file lebih dahulu, lalu masukkan event dan metadata DB dalam satu transaksi. Jika transaksi DB gagal, lakukan kompensasi dengan menghapus file yang baru dibuat; bila penghapusan gagal, catat/rekonsiliasi orphan melalui mekanisme terkontrol. Jika transaksi DB commit berhasil tetapi respons gagal, file harus dipertahankan karena metadata sudah merujuk padanya. Penghapusan retensi perlu dirancang agar kegagalan filesystem dapat dicoba ulang dan metadata tidak mengklaim foto hilang sebagai tersedia. Pemeriksaan expiry pada endpoint masa depan harus memakai tenggat dua bulan kalender WIB dari kontrak D-038.
