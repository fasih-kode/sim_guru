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

## Belum diputuskan
Schema domain, detail implementasi sesi/login, dan seluruh kebijakan OPEN PRD. Rancangan keamanan tahap berikut ada di ARCHITECTURE, belum fitur selesai. Tidak ada keputusan implisit untuk formula penilaian.
