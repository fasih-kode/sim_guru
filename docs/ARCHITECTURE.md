# Arsitektur

## Implementasi aktual — 21 Oktober 2026
Modular monolith Go dengan SSR. Fondasi tersedia; belum ada login atau modul bisnis.

| Lokasi | Tanggung jawab |
|---|---|
| cmd/server | Composition root, koneksi DB, pemeriksaan migrasi, lifecycle HTTP dan graceful shutdown |
| cmd/migrate | Perintah eksplisit up/status; tidak menjalankan migrasi otomatis saat server mulai |
| internal/config | .env opsional, prioritas environment, validasi konfigurasi dan TLS production |
| internal/database | Pool database/sql dengan driver pgx, ping terbatas waktu, error tanpa DSN |
| internal/web | Routing net/http, template HTML, fragmen htmx, aset embed dan header keamanan |
| migrations | SQL embed, namespace app, Goose dengan advisory lock PostgreSQL |
| web/styles | Sumber Tailwind; template Go menjadi sumber pemindaian kelas |
| scripts | Lifecycle cluster lokal dan penyalinan aset htmx |

Alur: browser → middleware keamanan → router → template lengkap atau fragmen HTML. GET / menghasilkan halaman; HX-Request: true menghasilkan section overview dengan Vary: HX-Request. Aset dilayani lokal dari binary. /health/live memeriksa proses; /health/ready memeriksa DB dan kesesuaian versi migrasi tanpa menampilkan detail koneksi. Server bind loopback secara default, memiliki timeout HTTP dan shutdown 10 detik.

Database memakai namespace app; tabel versi Goose berada di public. Migrasi 00001 hanya membuat namespace, tanpa data atau tabel bisnis. SQL Up/Down ada; Down belum dijalankan dan tidak diekspos CLI karena memerlukan persetujuan/rencana rollback. Jangan mengubah migration yang telah diterapkan.

## Rancangan tahap berikutnya — belum diimplementasikan
- Domain identitas/akun/role dimiliki administrasi; akademik/penugasan oleh Kurikulum. Domain absensi, jurnal, penilaian, dan laporan mengikuti sumber kebutuhan di [PRD](PRD.md). Batas package domain final ditetapkan saat implementasi, tanpa membuat abstraksi kosong.
- Handler menerjemahkan HTTP; service menjaga kebijakan/transaksi; akses PostgreSQL memakai query terparameterisasi. Hindari dependency antardomain melalui detail HTTP/template.
- Sesi server-side persisten, token acak yang hanya hash-nya disimpan di DB; rotasi setelah login/perubahan sensitif, expiry/revokasi, cookie HttpOnly/SameSite dan Secure di production. Belum ada cookie atau akun yang dibuat.
- RBAC: role dari Admin dan cakupan penugasan Kurikulum diperiksa server pada setiap tindakan. Admin tidak otomatis mendapat hak akademik; pencabutan hak tidak bergantung pada menu/cache sesi lama.
- Form mutasi memerlukan CSRF token dan pemeriksaan origin; baseline NewCrossOriginProtection saat ini belum menggantikan kebutuhan token/session pada login dan form mendatang.
- Audit perubahan penting ditulis dalam transaksi bisnis yang sama: pelaku, waktu, objek, aksi, perubahan relevan, alasan koreksi. Jangan mencatat password/token atau data pribadi yang tidak diperlukan. Belum ada tabel audit.

## Basis UI
TailAdmin Free HTML 2.4.0 diadaptasi untuk shell/sidebar/header/card dan token warna. Provenance dan MIT license ada di [third_party](../third_party/tailadmin/SOURCE.md). Demo dan font eksternal tidak disertakan. Tailwind 4.3.0 dan htmx 2.0.11 dipatok dalam package-lock.json; tidak ada Alpine karena shell belum memerlukannya. Tidak ada SPA atau backend Node.

## Belum selesai
Login, sesi, RBAC, audit, tabel bisnis, migrasi domain, workflow operasional, CI, backup/restore dan deployment. Halaman beranda menyatakan layanan operasional belum tersedia; tidak memuat angka akademik contoh.
