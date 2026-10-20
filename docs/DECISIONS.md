# Catatan keputusan

## D-001 — Pembagian kewenangan (disetujui 21 Oktober 2026)
Admin mengelola identitas siswa/guru, akun dan role. Kurikulum mengelola penugasan akademik, termasuk wali kelas dan pengampu resmi. Pengguna menyetujui penyelarasan referensi Admin yang bertentangan di roadmap/PRD frontend. Alasan: konsisten dengan PRD bab 3A. Dokumen sumber diperbaiki tanpa memindahkan atau menggandakannya.

## D-002 — Sistem Project Memory (disetujui 21 Oktober 2026)
AGENTS, indeks kebutuhan, arsitektur, aturan bisnis, testing, decisions, dan handover memakai tautan relatif ke tiga dokumen asli. Protokol awal/akhir sesi wajib menjaga konteks berdasarkan bukti aktual.

## D-003 — Ubuntu 26.04 (disetujui 21 Oktober 2026)
Pengguna secara eksplisit menyetujui Ubuntu 26.04 yang tersedia menggantikan ketentuan Ubuntu 24 untuk lingkungan development. OS tidak diubah. Stack aplikasi tetap mengikuti PRD.

## D-004 — Isolasi development (pilihan teknis dalam lingkup yang diotorisasi)
Gunakan toolchain terpasang dan cluster PostgreSQL lokal milik proyek pada loopback, port 55436, di .local/ yang diabaikan Git. Database sim_guru_dev dan sim_guru_test terpisah. Tidak mengubah cluster sistem/proyek lain. SSH yang sudah tersedia dipakai untuk remote GitHub. Tidak membuat commit/push.

## D-005 — Fondasi Go (pilihan teknis, 21 Oktober 2026)
Pengguna menginstruksikan pembangunan fondasi sesuai PRD. Gunakan net/http dan html/template bawaan, database/sql + pgx v5.11.0, godotenv v1.5.1, serta Goose v3.29.0. Tidak menambah router/ORM/framework frontend. Goose dipilih untuk migrasi SQL transaksi, riwayat versi, embed dan advisory lock PostgreSQL. Migrasi eksplisit terpisah dari startup. Migrasi pertama hanya namespace app; belum ada tabel domain.

## D-006 — Aset lokal minimal (pilihan teknis, 21 Oktober 2026)
TailAdmin Free HTML 2.4.0 commit 1bd2dc42a8467ae0281cf00bd0050da4c9c4be07 berlisensi MIT; README/license diperiksa sebelum adaptasi. Pakai shell/cards/token warna, bukan seluruh demo. Tailwind CLI menggantikan bundling Webpack karena belum ada kebutuhan JavaScript kompleks. Tailwind/CLI dipatok 4.3.0, htmx 2.0.11; npm audit fix memperbarui dependensi transitif dan audit ulang nol kerentanan. Awalnya 4.3.3 membawa advisory build dependency; lockfile akhir harus dipertahankan, jangan diperbarui otomatis tanpa pemeriksaan.

## D-007 — Patch toolchain proyek (pilihan teknis keamanan, 21 Oktober 2026)
Govulncheck pada Go 1.27.1 melaporkan sembilan kerentanan standard library yang terjangkau. go.mod dinaikkan ke 1.27.3 melalui pemilihan toolchain Go otomatis, tanpa mengganti /usr/local/go milik proyek lain. [Release history resmi](https://go.dev/doc/devel/release) mencatat patch keamanan 1.27.2 dan 1.27.3. Hasil verifikasi ulang dicatat di HANDOVER.

## D-008 — Autentikasi dan administrasi akses (pilihan teknis, 21 Oktober 2026)
Pengguna mengotorisasi implementasi autentikasi dan pengelolaan pengguna/role. Tambahkan migration 00002 tanpa mengubah 00001; delapan role mengikuti PRD. Nama tampilan akun belum menjadi data induk guru. Password bcrypt cost 12, 12 karakter minimum dan batas 72 byte; golang.org/x/crypto v0.58.0. Bootstrap CLI memakai golang.org/x/term v0.47.0 agar password tidak terlihat dan hanya bekerja saat akun kosong; tidak ada credential default.

Sesi token acak 256 bit disimpan sebagai SHA-256, batas 8 jam/idle 30 menit, cookie HttpOnly/SameSite=Lax/Secure production. Status/role dibaca per request; revokasi password/logout/nonaktif berlaku di server. Gorilla CSRF v1.7.3 dengan key lingkungan persisten digunakan bersama NewCrossOriginProtection; tidak memakai TrustedOrigins. Limiter login bounded per proses mengabaikan header forwarding yang tidak dipercaya. Pilihan ini tetap membutuhkan evaluasi deployment sebelum produksi.

Mutasi akun dan audit atomik; advisory lock menghindari race akses/bootstrap, optimistic version menghindari lost update. Minimal satu Admin aktif dilindungi sebagai pengaman operasional, bukan approval tambahan. UI hanya menonaktifkan akun, bukan menghapus histori. Audit tidak memuat credential; UI menampilkan WIB.

## D-009 — Port development 8090 (21 Oktober 2026)
Pengguna menyampaikan port 8080 dipakai aplikasi lain dan meminta penggantian. Port 8090 tersedia saat diperiksa, dipilih untuk konfigurasi lokal, contoh dan fallback agar konsisten. Restart hanya proses SIM-GURU; akun Admin yang sudah dibuat dipertahankan. APP_PORT tetap dapat dioverride melalui environment.

## D-010 — Port development 18743 menggantikan 8090 (21 Oktober 2026)
Pengguna menyatakan 8090 juga dipakai aplikasi lain. SIM-GURU dipindahkan ke 18743 setelah pemeriksaan ketersediaan; port 8080 dan 8090 tidak digunakan lagi oleh SIM-GURU. APP_PORT tetap dapat dikonfigurasi.

## Belum diputuskan
Schema siswa/guru dan akademik, cakupan penugasan aktual, kebijakan retensi audit/sesi, serta seluruh OPEN PRD. Tidak ada keputusan implisit mengenai formula penilaian atau perubahan kewenangan Admin/Kurikulum. MFA/reset mandiri dan deployment belum dibuat.
