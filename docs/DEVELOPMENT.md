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

Toolchain proyek memerlukan Go 1.27.3; instalasi global tidak diubah. Biarkan GOTOOLCHAIN=auto agar Go mengunduh patch yang diminta. Versi ini dipilih setelah pemeriksaan keamanan; lihat [keputusan](DECISIONS.md).

Migrasi up/status hanya menarget DATABASE_URL. Tidak ada perintah reset/down otomatis. Jangan mengedit migrasi 00001 yang sudah diterapkan. Tambahkan migrasi berikutnya; rencana rollback harus melindungi data dan mendapat persetujuan sebelum dijalankan.

## Belum selesai
Login, sesi, pengguna/role dan audit akses tersedia. Belum ada master siswa/guru dan modul akademik, CI, backup/restore teruji atau deployment. Kebijakan OPEN PRD belum berubah.

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

Migrasi terbaru adalah versi 2. Jalankan make migrate-up sebelum menjalankan server versi ini. Tidak ada akun/password bawaan dan tidak ada akun development yang dibuat oleh pengujian.

Buat Admin pertama melalui terminal sendiri:

```sh
go run ./cmd/admin -username admin -name "Administrator Sekolah"
```

Password diminta dua kali tanpa echo, lalu konfirmasi BUAT. Bootstrap hanya berhasil jika tabel akun kosong; tidak mengubah password/role akun yang sudah ada. Jangan memberikan password melalui argumen, chat, environment, pipe, atau source code. Setelah bootstrap, buka http://127.0.0.1:18743/login lalu buat akun personel melalui Pengguna dan role.

Perubahan/reset password dan penonaktifan mencabut seluruh sesi target. Ganti password sendiri memerlukan password saat ini dan mengarahkan kembali ke login. Role baru langsung tampil pada sesi aktif tanpa approval; Guru Mapel/Wali Kelas belum memiliki fitur operasional sebelum modul penugasan dibuat.

Production belum dideploy: diperlukan HTTPS/reverse proxy yang benar, role DB least privilege terpisah dari migrator, prosedur backup/restore, limiter lintas instance bila relevan, serta pemeriksaan keamanan operasional. Cookie Secure hanya aktif ketika APP_ENV=production; HTTP loopback development memakai cookie non-Secure agar browser dapat mengirimkannya.
