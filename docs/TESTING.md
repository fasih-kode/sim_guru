# Pengujian

## Perintah yang tersedia
Jalankan dari root proyek. Go 1.27.3 dipilih melalui go.mod; GOTOOLCHAIN=auto dapat mengunduh toolchain terverifikasi.

```sh
make test
make check
make build
make integration
npm ci
npm run build
npm audit
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

- make test: unit/HTTP tests dengan race detector; tidak membutuhkan database.
- make check: gofmt, go vet, dan git diff --check. git diff tidak mencakup file untracked; tinjau file baru secara langsung.
- make integration: koneksi PostgreSQL nyata, privilege aplikasi, migrasi Up, pengulangan Up tanpa perubahan, versi/schema, serta rollback transaksi migrasi yang sengaja gagal, serta alur HTTP akun/role terhadap database nyata.
- Test integrasi membaca TEST_DATABASE_URL dari environment atau .env root. Wajib database berakhiran _test, berbeda dari DATABASE_URL, dan role tanpa superuser/CREATEDB/CREATEROLE. Tes melakukan DDL dan membuat fixture akun bernama test_* di database test; tidak memakai data produksi, tidak melakukan reset atau Down. Fixture memakai password acak dan histori dipertahankan. Tes HTTP bersifat satu proses, sekitar satu menit dengan race detector. Proteksi Admin terakhir dan keberhasilan bootstrap diperiksa saat database akun pertama kali kosong; penolakan bootstrap diperiksa setiap run.
- npm ci/build: instalasi sesuai lockfile dan regenerasi CSS/htmx lokal. Periksa hasil generated assets sebelum menyimpulkan binary terbaru.
- npm audit dan govulncheck adalah pemeriksaan pada database advisory saat dijalankan, bukan jaminan aplikasi sepenuhnya aman.

## Cakupan saat ini
Unit: konfigurasi/CSRF key, validasi akun/role/password, bcrypt, limiter, atribut cookie production/development, header keamanan, route publik, CSRF dan origin negatif, readiness 503. Integrasi: login valid/salah/tidak ada/nonaktif, SQL injection username, XSS nama tampilan, akses Admin/non-Admin, multi-role dan grant/revoke tanpa login ulang, versi edit kedaluwarsa, konflik edit concurrent, penonaktifan/reaktivasi, rotasi/expiry/idle/logout, ganti/reset password dan revokasi sesi, pencarian/filter, audit tanpa kredensial, SSR/fragmen htmx serta Vary. Integration test membutuhkan cluster proyek aktif melalui scripts/dev-db.sh start.

Pemeriksaan browser manual/otomatis melalui Playwright dilakukan untuk desktop/mobile, reload htmx, console/network, pesan gagal, dan workflow tambah/koreksi siswa, histori, guru, mapel serta tampilan Kurikulum baca-saja. Belum menjadi suite end-to-end yang tersimpan dalam repository.

## Strategi domain yang belum diimplementasikan
Ikuti acceptance criteria [PRD/roadmap](PRD.md): akses positif/negatif setiap role dan penugasan, histori/transaksi, konflik jadwal, pemisahan absensi/jurnal, koreksi Final, cakupan laporan, tanpa guru pengganti, UAT, keamanan dan backup/restore. Kalender/kelas serta akun/role dan master siswa/guru/mapel sudah memiliki tes; penempatan dan operasi akademik lanjut belum tersedia.

Catat hasil aktual, kegagalan awal, keterbatasan, dan pekerjaan tersisa di HANDOVER setiap sesi.

## Hasil pemindaian dependensi autentikasi
Govulncheck tidak menemukan simbol rentan yang dipanggil. Mode verbose tetap mencantumkan GO-2025-3884 pada fitur TrustedOrigins Gorilla CSRF dan GO-2026-5932 pada openpgp x/crypto. Aplikasi tidak mengatur TrustedOrigins dan tidak mengimpor openpgp; pemeriksaan origin tambahan tetap aktif. Jangan menyebut seluruh dependency graph bebas advisory; evaluasi ulang bila konfigurasi/fitur berubah. Lihat [advisory CSRF](https://pkg.go.dev/vuln/GO-2025-3884) dan [advisory openpgp](https://pkg.go.dev/vuln/GO-2026-5932).

## Cakupan master data
make integration kini mencakup internal/master dan internal/web/master_integration_test.go. internal/testutil (hanya build tag integration) memvalidasi database _test, role DB non-superuser/tanpa CREATEDB/CREATEROLE, menerapkan migrasi dan membuat akun fixture acak. Test data tidak dibuat pada database development atau produksi; fixture/histori test dipertahankan tanpa reset/Down.

Unit menguji tanggal kalender/masa depan, atribut wajib/opsional, email, batas panjang, normalisasi kode, alasan koreksi dan penolakan kind tidak valid. Integrasi layanan menguji ID tetap, NIS/NISN duplikat, kontak baca-saja disamarkan, pemisahan status guru/akun, satu akun-satu guru, kode mapel historis, rollback saat konflik, histori dan dua edit concurrent. Integrasi HTTP menguji CSRF/403 lintas-role, data form tetap saat validasi gagal, XSS, pencarian/filter, detail/histori, kode mapel dan PATCH tidak didukung.

Versi migrasi target saat ini 6. Run integrasi sebelum modul kalender membutuhkan sekitar 85 detik untuk paket web dengan race detector; tes kalender baru ada di `internal/academic` dan `internal/web/academic_integration_test.go`.

## Pengujian impor Excel
Unit parser mencakup template asli, nol awal/NIP panjang, tanggal Excel dan epoch 1904, tanggal palsu, identifier numerik, formula, status tidak sah, header/kolom salah, workbook kosong, baris kosong dengan nomor baris asli, row/file limit, archive rusak dan macro. Template siswa/guru dibuat dengan Artifact Tool, dirender setiap sheet dan diperiksa visual; tidak berisi data contoh yang dapat terimpor tidak sengaja.

Integrasi impor PostgreSQL membuktikan preview tidak menulis master, owner isolation, role negatif/revokasi Admin sebelum confirm, kandidat duplikat, konflik setelah preview tanpa data parsial, expiry/scrub payload, receipt/replay concurrent, histori dan tidak membuat tautan akun. Integrasi HTTP memakai multipart nyata termasuk file valid >32 KiB, download template, CSRF, format salah, duplikat, status preview/konfirmasi, siswa/guru dan expiry. Unit body limit memastikan request terlalu besar ditolak sebelum parsing.

Setelah perubahan kecil pada cleanup multipart/deduplikasi pesan, tes fokus dijalankan ulang dengan go test -race -tags=integration -run TestImport ./internal/master ./internal/web. Suite lengkap dijalankan ulang setelah memperbarui Excelize ke revisi perbaikan keamanan; jangan memakai v2.11.0 untuk upload tak terpercaya. govulncheck masih perlu diulang pada pembaruan dependensi.

## Pengujian kalender akademik dan kelas
Unit menguji validasi nama tahun ajaran, tingkat SMP 7–9, batas panjang dan alasan koreksi. Integrasi layanan memeriksa dua semester otomatis, batas satu tahun/semester aktif, dependensi semester pada tahun aktif, kalender khusus Kurikulum, tulis kelas khusus Admin, baca kelas/histori bersama, duplikasi lintas kapitalisasi, ID/version, konflik dan histori. Regresi kelas lama yang dibuat Kurikulum membuktikan Admin dapat melanjutkan koreksi tanpa mengganti ID atau actor histori lama. Tes juga memeriksa multi-role, pencabutan Admin, akun nonaktif dan role lain ditolak. Tes HTTP memeriksa login, menu/form baca-saja Kurikulum, Admin tambah/edit kelas dan histori dengan tautan kembali yang dapat dibuka, CSRF, GET/POST langsung terlarang, kalender tetap menolak Admin, multi-role tanpa menu kelas duplikat serta pencabutan role pada sesi aktif. Request terlarang diperiksa tidak mengubah versi/nama/histori kelas. Cakupan placement belum memiliki tes otomatis service/HTTP pada suite ini. Acceptance minimum yang wajib ditambahkan adalah unik siswa/tahun, kelas pada tahun yang sesuai, histori/actor atomik, role Admin/Kurikulum, duplikat concurrent dan input CSRF. Hasil build terbaru tercatat di [HANDOVER](HANDOVER.md).
