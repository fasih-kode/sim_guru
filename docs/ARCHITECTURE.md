# Arsitektur

## Implementasi aktual — 21 Oktober 2026
Modular monolith Go dengan SSR. Fondasi dan autentikasi/pengelolaan pengguna-role tersedia; modul akademik belum diimplementasikan.

| Lokasi | Tanggung jawab |
|---|---|
| cmd/server | Composition root, koneksi DB, pemeriksaan migrasi, lifecycle HTTP dan graceful shutdown |
| cmd/admin | Bootstrap interaktif Admin pertama, hanya ketika belum ada akun |
| internal/auth | Validasi, bcrypt, sesi PostgreSQL, transaksi pengguna/role dan audit akses |
| cmd/migrate | Perintah eksplisit up/status; tidak menjalankan migrasi otomatis saat server mulai |
| internal/config | .env opsional, prioritas environment, validasi konfigurasi dan TLS production |
| internal/database | Pool database/sql dengan driver pgx, ping terbatas waktu, error tanpa DSN |
| internal/web | Routing net/http, template HTML, fragmen htmx, aset embed dan header keamanan |
| migrations | SQL embed, namespace app, Goose dengan advisory lock PostgreSQL |
| web/styles | Sumber Tailwind; template Go menjadi sumber pemindaian kelas |
| scripts | Lifecycle cluster lokal dan penyalinan aset htmx |

Alur: browser → middleware keamanan → router → template lengkap atau fragmen HTML. GET / menghasilkan halaman; HX-Request: true menghasilkan section overview dengan Vary: HX-Request. Aset dilayani lokal dari binary. /health/live memeriksa proses; /health/ready memeriksa DB dan kesesuaian versi migrasi tanpa menampilkan detail koneksi. Server bind loopback secara default, memiliki timeout HTTP dan shutdown 10 detik.

Database memakai namespace app; tabel versi Goose berada di public. Migration 00001 tetap utuh. Migration 00002 menambahkan users, roles, user_roles, sessions dan access_audit, dengan foreign key dan delapan role PRD. Akun belum ditautkan ke identitas guru karena master guru baru dikerjakan pada fase berikutnya; jangan membuat salinan data guru dari nama tampilan akun.

## Autentikasi dan otorisasi aktual
- Login memakai username ternormalisasi dan bcrypt cost 12. Password minimal 12 karakter, maksimal 72 byte UTF-8; tidak dipangkas diam-diam. Akun tidak ada/password salah/nonaktif memakai pesan gagal yang sama; dummy bcrypt mengurangi perbedaan waktu untuk akun tidak ada.
- Sesi memakai token acak 256 bit. Hanya SHA-256 token tersimpan di PostgreSQL. Cookie host-only, HttpOnly, SameSite=Lax, Secure pada production; token baru pada login dan sesi lama direvokasi. Batas absolut 8 jam, idle 30 menit. Logout, deaktivasi, dan perubahan/reset password mencabut sesi di server. Reaktivasi tidak menghidupkan kembali sesi lama.
- Status akun dan role diambil ulang pada setiap request terlindungi. Role bukan cakupan penugasan: Admin tidak otomatis memperoleh Kurikulum atau akses nilai. Halaman /akademik saat ini hanya konfirmasi akses role Kurikulum, bukan modul akademik selesai.
- Gorilla CSRF memvalidasi token untuk seluruh form mutasi termasuk login/logout, ditambah NewCrossOriginProtection dan batas body 32 KiB. Key signing acak 32 byte wajib di konfigurasi lokal; tidak disimpan Git. Request HTTP development ditandai plaintext untuk library, namun pemeriksaan origin dan token tetap aktif.
- Login dibatasi per proses: 10 percobaan/username dan 30/IP dalam 15 menit, maksimal sekitar 10.000 bucket. Tidak mempercayai X-Forwarded-For. Reset proses mereset limiter; deployment multi-instance/proxy memerlukan desain limiter bersama dan konfigurasi proxy yang benar sebelum produksi.
- Perubahan akun/role, revokasi sesi terkait, dan audit dilakukan dalam transaksi yang sama. Advisory transaction lock menyerialkan mutasi akun/bootstrap/login dengan perubahan password; optimistic version menolak form edit kedaluwarsa. Minimal satu Admin aktif dipertahankan agar akses pengelolaan tidak terkunci.
- Audit menyimpan actor, target ID, waktu, aksi, snapshot aman sebelum/sesudah; tidak menyimpan password/hash password/token. Login/logout/password change juga dicatat. UI Admin menampilkan 100 catatan terbaru dalam WIB. Tidak ada workflow approval.
- users tidak dihapus melalui UI; status aktif/nonaktif mempertahankan referensi dan histori. Role katalog tetap sesuai PRD; Admin memilih kombinasi role, tidak membuat definisi kewenangan baru.

## Route aktual
Publik: GET/POST /login, GET /health/live, GET /health/ready, GET /static/*.
Terautentikasi: GET / (SSR/fragmen htmx), POST /logout, GET/POST /account/password.
Admin: GET /admin/users (cari/filter/pagination 50), GET/POST /admin/users/new, GET/POST /admin/users/{id}/edit, GET /admin/audit.
Kurikulum: GET /akademik (halaman kesiapan).

## Batas domain tahap berikutnya
Handler menerjemahkan HTTP, internal/auth menangani kebijakan dan transaksi akun. Domain identitas siswa/guru, akademik/penugasan, absensi, jurnal, penilaian, dan laporan belum diimplementasikan. Ketika ditambah, gunakan service sesuai domain, SQL terparameterisasi, transaksi dan audit relevan. Guru Mapel/Wali Kelas harus diperiksa terhadap role sekaligus penugasan aktif; jangan memperluas hak dari role saja.

## Basis UI
TailAdmin Free HTML 2.4.0 diadaptasi untuk shell/sidebar/header/card dan token warna. Provenance dan MIT license ada di [third_party](../third_party/tailadmin/SOURCE.md). Demo dan font eksternal tidak disertakan. Tailwind 4.3.0 dan htmx 2.0.11 dipatok dalam package-lock.json; tidak ada Alpine karena shell belum memerlukannya. Tidak ada SPA atau backend Node.

## Belum selesai
Master siswa/guru, penugasan akademik dan seluruh workflow akademik, CI, backup/restore, deployment, MFA/reset password mandiri, serta retensi/arsip audit dan sesi belum diimplementasikan. Tidak ada data atau angka akademik contoh.
