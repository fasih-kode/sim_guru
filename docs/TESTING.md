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

Pemeriksaan browser manual/otomatis melalui Playwright dilakukan untuk desktop/mobile, reload htmx, console/network, dan pesan ketika request gagal. Belum menjadi suite end-to-end yang tersimpan dalam repository.

## Strategi domain yang belum diimplementasikan
Ikuti acceptance criteria [PRD/roadmap](PRD.md): akses positif/negatif setiap role dan penugasan, histori/transaksi, konflik jadwal, pemisahan absensi/jurnal, koreksi Final, cakupan laporan, tanpa guru pengganti, UAT, keamanan dan backup/restore. Tes akun/role sudah tersedia; tes domain akademik belum ada karena fitur belum tersedia.

Catat hasil aktual, kegagalan awal, keterbatasan, dan pekerjaan tersisa di HANDOVER setiap sesi.

## Hasil pemindaian dependensi autentikasi
Govulncheck tidak menemukan simbol rentan yang dipanggil. Mode verbose tetap mencantumkan GO-2025-3884 pada fitur TrustedOrigins Gorilla CSRF dan GO-2026-5932 pada openpgp x/crypto. Aplikasi tidak mengatur TrustedOrigins dan tidak mengimpor openpgp; pemeriksaan origin tambahan tetap aktif. Jangan menyebut seluruh dependency graph bebas advisory; evaluasi ulang bila konfigurasi/fitur berubah. Lihat [advisory CSRF](https://pkg.go.dev/vuln/GO-2025-3884) dan [advisory openpgp](https://pkg.go.dev/vuln/GO-2026-5932).
