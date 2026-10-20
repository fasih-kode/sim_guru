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
- make check: gofmt, go vet, dan git diff --check. Karena belum ada commit, git diff tidak mencakup file untracked; tinjau file baru secara langsung.
- make integration: koneksi PostgreSQL nyata, privilege aplikasi, migrasi Up, pengulangan Up tanpa perubahan, versi/schema, serta rollback transaksi migrasi yang sengaja gagal.
- Test integrasi membaca TEST_DATABASE_URL dari environment atau .env root. Wajib database berakhiran _test, berbeda dari DATABASE_URL, dan role tanpa superuser/CREATEDB/CREATEROLE. Tes melakukan DDL di database test; tidak memakai data produksi, tidak melakukan reset atau Down.
- npm ci/build: instalasi sesuai lockfile dan regenerasi CSS/htmx lokal. Periksa hasil generated assets sebelum menyimpulkan binary terbaru.
- npm audit dan govulncheck adalah pemeriksaan pada database advisory saat dijalankan, bukan jaminan aplikasi sepenuhnya aman.

## Cakupan saat ini
Validasi konfigurasi, error yang tidak membocorkan kredensial, halaman SSR vs fragmen, Vary, aset lokal, route/method yang ditolak, cross-origin POST, header keamanan, dan readiness 503. Integration test membutuhkan cluster proyek aktif melalui scripts/dev-db.sh start.

Pemeriksaan browser manual/otomatis melalui Playwright dilakukan untuk desktop/mobile, reload htmx, console/network, dan pesan ketika request gagal. Belum menjadi suite end-to-end yang tersimpan dalam repository.

## Strategi domain yang belum diimplementasikan
Ikuti acceptance criteria [PRD/roadmap](PRD.md): akses positif/negatif setiap role dan penugasan, histori/transaksi, konflik jadwal, pemisahan absensi/jurnal, koreksi Final, cakupan laporan, tanpa guru pengganti, UAT, keamanan dan backup/restore. Belum ada tes domain karena fitur belum ada.

Catat hasil aktual, kegagalan awal, keterbatasan, dan pekerjaan tersisa di HANDOVER setiap sesi.
