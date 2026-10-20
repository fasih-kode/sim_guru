# Handover

## Status — 21 Oktober 2026
Autentikasi dan pengelolaan pengguna/role sudah dibuat dan diuji pada baseline fondasi ff3e79b. Branch main; pengguna kini menyetujui checkpoint commit/push autentikasi dan port 18743 sebelum implementasi master data. Hasil sinkronisasi diverifikasi melalui Git pada sesi checkpoint.

Server development berjalan pada http://127.0.0.1:18743/login. Port 8080 dan 8090 dicadangkan pengguna untuk aplikasi lain; SIM-GURU kini memakai 18743. Database development sim_guru_dev pada 55436 sudah migrasi versi 2; pengguna sudah membuat akun Admin dan satu Admin aktif telah diverifikasi. Jangan mengulang bootstrap atau mengganti akun yang ada. Tidak ada test account yang dibuat di development.

## Perubahan
- Migration 00002 menambah users, roles, user_roles, sessions, access_audit. Migration 00001 tidak diubah. Delapan role sesuai PRD.
- internal/auth menangani validasi, bcrypt cost 12, login/logout, token sesi hash-only, expiry 8 jam/idle 30 menit, administrasi akun/multi-role, revokasi, optimistic version dan audit atomik.
- internal/web menambah login, ganti password, halaman Admin daftar/cari/filter/pagination/tambah/edit akun, reset password, status aktif, multi-role, serta 100 audit akses terakhir dalam WIB. Halaman Akademik hanya bukti akses role Kurikulum; belum ada operasi akademik.
- CSRF token seluruh form mutasi, origin protection, batas body, limiter login bounded per IP/username, cookie HttpOnly/SameSite/Secure production. Status dan role dibaca ulang setiap request; perubahan role langsung berlaku tanpa login ulang.
- cmd/admin menyediakan bootstrap interaktif hanya pada tabel akun kosong. .env lokal ditambah CSRF_AUTH_KEY acak tanpa ditampilkan; .env.example hanya placeholder. Tidak ada password/key/token dalam source atau dokumentasi.
- Dokumentasi architecture/business rules/testing/development/decisions/PRD index dan README diperbarui; kebutuhan dan kebijakan OPEN tidak diubah.

## Verifikasi aktual
- make test: lulus dengan race detector (auth/config/database/web).
- make check: gofmt, go vet, git diff --check lulus. File baru ditinjau langsung karena belum tracked.
- make integration: lulus, termasuk HTTP dengan PostgreSQL nyata, sekitar 56 detik pada run akhir. Memakai TEST_DATABASE_URL terpisah berakhiran _test dan role non-superuser/tanpa CREATEDB/CREATEROLE.
- Bukti integrasi: login valid/salah/akun tidak ada/nonaktif, SQL injection username, XSS nama tampilan, CSRF/origin negatif, akses Admin/non-Admin, multi-role grant/revoke pada sesi aktif, form kedaluwarsa dan dua edit concurrent (satu pemenang/satu konflik), deaktivasi/reaktivasi, rotasi token, idle/absolute expiry, logout, ganti/reset password dan revokasi, filter/pencarian, audit tanpa credential, SSR/fragmen/Vary.
- Keberhasilan bootstrap dan proteksi Admin terakhir diperiksa pada run pertama ketika tabel akun test masih kosong; penolakan bootstrap diulang pada run berikutnya. Tes menjaga data test sebelumnya, sehingga dua skenario pertama bersyarat pada kondisi awal tersebut.
- npm run build, make build dan npm audit lulus; npm audit nol kerentanan.
- govulncheck v1.9.0: nol simbol rentan terjangkau. Mode verbose mencantumkan advisory TrustedOrigins Gorilla CSRF (fitur tidak digunakan) dan openpgp x/crypto (tidak diimpor). Jangan mengklaim seluruh dependency graph tanpa advisory; lihat TESTING.
- CLI bootstrap dengan stdin nonterminal ditolak sebagaimana diharapkan (exit 1); proses input password terminal belum diuji manual end-to-end. Logika bootstrap diuji melalui integrasi.
- Playwright pada server test terpisah 8081: login Admin, tambah akun multi-role, cari akun, edit/cabut role berhasil; form mobile lebar 390 tanpa horizontal overflow dan tanpa pageerror. Screenshot diperiksa visual. Fixture browser dinonaktifkan, sesinya dicabut, file credential sementara dihapus dan server test dihentikan.
- Pada sesi autentikasi sebelumnya, server development 8080 dijalankan ulang dengan binary terbaru; /login dan /health/ready HTTP 200. Pada saat itu jumlah akun development 0; kini pengguna sudah membuat Admin. .env tetap berizin 0600.
- Review akhir diff dan 48 file sumber/dokumentasi: git diff --check lulus; migration 00001 tanpa perubahan; tidak ditemukan kredensial lokal/private key atau tautan file dokumentasi rusak.

## Risiko dan batasan
Akun belum terhubung ke master guru; nama tampilan adalah data akun. Belum ada modul siswa/guru, penugasan kelas/mapel, nilai atau workflow akademik. Jangan menganggap role Guru/Wali saja memberi cakupan kelas. Limiter masih per proses dan tidak menggunakan header proxy; deployment memerlukan rancangan proxy/rate limiting yang tepat. Tidak ada MFA, reset mandiri, CI, backup/restore teruji, retensi audit/sesi atau deployment production. Test fixture acak disimpan hanya di database test; tidak dilakukan reset/Down/penghapusan data.

## Langkah berikutnya
1. Periksa Git status, AGENTS, HANDOVER dan kode aktual. Jangan menimpa perubahan pengguna.
2. Gunakan Admin yang sudah dibuat pengguna melalui http://127.0.0.1:18743/login. Jangan kirim password melalui chat/argumen/pipe.
3. Setelah review, commit/push hanya jika diinstruksikan. Tidak ada commit/push pada sesi ini.
4. Lanjutkan fase master siswa/guru oleh Admin dan mapel oleh Kurikulum dengan migration baru, ID stabil dan tes akses negatif. Penugasan akademik tetap hanya Kurikulum.
5. Putuskan OPEN P0 hanya saat fitur terkait memerlukannya; perbarui dokumentasi dan handover sesuai bukti.

## Perubahan port development — 21 Oktober 2026
APP_PORT lokal, .env.example, fallback konfigurasi, README dan DEVELOPMENT diubah ke 8090. Port diperiksa tersedia sebelum dipakai. Hanya proses SIM-GURU yang diverifikasi melalui cwd/executable dihentikan dengan SIGTERM lalu dimulai ulang; proses aplikasi lain tidak dihentikan.

Verifikasi: go test -race ./internal/config lulus (termasuk override APP_PORT), make build lulus, git diff --check lulus, /login dan /health/ready di 8090 HTTP 200. Fingerprint seluruh baris akun sebelum/sesudah restart identik; satu Admin aktif. Database dan password tidak diubah. Tidak ada commit/push. Tes autentikasi penuh tidak diulang karena perubahan hanya konfigurasi port dan dokumentasi.

## Koreksi port terbaru — 21 Oktober 2026
Pengguna juga mencadangkan 8090 untuk aplikasi lain. Konfigurasi lokal, fallback, .env.example, README dan DEVELOPMENT kini memakai 18743, yang tersedia saat pemeriksaan. Proses SIM-GURU di 8090 dihentikan setelah cwd/executable diverifikasi; server dijalankan ulang di 18743. Tes konfigurasi dengan race detector, build dan git diff --check lulus; /login dan /health/ready HTTP 200. Tidak ada operasi perubahan database, akun atau password. Tidak ada commit/push. Catatan port 8090 di atas adalah riwayat, bukan konfigurasi aktif.

## Checkpoint sebelum master data
Pengguna mengotorisasi commit/push autentikasi dan port 18743. make test, make check, make integration dan make build diperiksa ulang; file lokal rahasia dikecualikan. Pekerjaan berikutnya adalah master siswa/guru Admin dan mapel Kurikulum.
