# Handover

## Status — 21 Oktober 2026
Fondasi sesuai PRD sudah dibuat dan diuji. Server lokal tersedia pada http://127.0.0.1:8080 selama proses sesi masih aktif. PostgreSQL proyek di loopback 55436. Belum ada login, sesi, RBAC, audit bisnis atau modul akademik; bukan aplikasi siap produksi.

## Perubahan sesi ini
- cmd/server: konfigurasi tervalidasi, pool pgx/database/sql, startup menolak DB/migrasi belum siap, timeout HTTP dan graceful shutdown.
- internal/web: SSR html/template, fragmen htmx, layout adaptasi TailAdmin, Tailwind lokal, header keamanan/CSP dan pemeriksaan origin. Route yang tersedia hanya beranda, static, liveness/readiness.
- cmd/migrate dan migrations: Goose dengan advisory lock; Up/Down SQL awal hanya namespace app. CLI up/status; tidak ada reset/down otomatis.
- Test konfigurasi, HTTP/keamanan, koneksi yang tidak membocorkan rahasia, dan integrasi migrasi PostgreSQL ditambahkan. Makefile menyediakan run/build/assets/test/check/integration/migration.
- Go module/sum serta npm lockfile dipatok; TailAdmin 2.4.0 provenance/MIT dan lisensi htmx disimpan di third_party.
- Project Memory diperbarui: README, AGENTS, PRD index, architecture, testing, development, decisions. Aturan bisnis dan kebijakan OPEN tidak diubah.

## Verifikasi aktual
- Go aktif dalam proyek: 1.27.3; Go global tetap 1.27.1. Toolchain diunduh melalui mekanisme auto Go, tidak mengganti instalasi global.
- make assets (npm ci/build): lulus. npm audit: nol kerentanan pada pemeriksaan akhir.
- make test: lulus dengan race detector; paket config/database/web memiliki tes, cmd/server dan cmd/migrate belum memiliki tes langsung.
- make check: gofmt, go vet, git diff --check lulus. Karena semua file masih untracked, pemeriksaan Git diff saja tidak mencakup perubahan; review file baru dilakukan langsung.
- make build: lulus. Binary lokal mengandung template dan aset embed.
- make migrate-up/status: development versi 1, target 1. Integrasi memakai sim_guru_test; role non-superuser/tanpa CREATEDB/CREATEROLE, Up idempotent, schema tersedia, migrasi gagal di-rollback atomik. Tidak ada Down/reset yang dijalankan.
- govulncheck v1.9.0 pada toolchain akhir: No vulnerabilities found.
- Browser Playwright: SSR/fragment reload berhasil; satu section overview setelah swap; desktop 1440 dan mobile 390 diperiksa; mobile tanpa horizontal overflow. Readiness HTTP 200, tidak ada request eksternal. Simulasi 503 menampilkan pesan gagal dan request berikutnya memulihkan tampilan. Error console saat simulasi memang berasal dari 503 yang disengaja; favicon 404 awal diperbaiki. Screenshot mobile diperiksa secara visual.
- Server lama dihentikan dengan SIGTERM dan keluar normal; binary terbaru dijalankan ulang. Cluster PostgreSQL tidak dihentikan atau diubah portnya.

## Kegagalan awal yang telah ditangani
Tes traversal semula mengharapkan 301; Go 1.27 mengembalikan redirect normalisasi 307, ekspektasi disesuaikan dan tes ulang lulus. Instalasi awal Tailwind 4.3.3 memunculkan advisory dependency build; paket akhir 4.3.0 dan pembaruan transitif melalui npm audit fix menghasilkan audit bersih. Govulncheck awal menemukan sembilan kerentanan standard library Go 1.27.1; patch proyek 1.27.3 dan pemindaian ulang lulus.

## Risiko/keterbatasan
Tidak ada CI, UAT operasional, backup/restore teruji, deployment, pengujian beban atau rollback Down. Bootstrap manual mesin baru didokumentasikan tetapi belum diuji ulang pada cluster baru. UI saat ini hanya beranda persiapan. Pemeriksaan browser sesi ini belum menjadi suite yang tersimpan. Hasil scan adalah kondisi saat pemeriksaan, bukan jaminan keamanan seluruh aplikasi. File .env dan .local tetap hanya lokal serta diabaikan Git. Pemeriksaan akhir 41 kandidat file lulus untuk kebocoran kredensial lokal, tautan file dokumentasi, dan whitespace sumber; git check-ignore mengonfirmasi .env, data cluster, node_modules dan binary dikecualikan.

Pengguna mengotorisasi commit dan push fondasi arsitektur ke origin/main pada 21 Oktober 2026, sebelum melanjutkan autentikasi. Baseline mencakup kode fondasi, pengujian, aset, sumber kebutuhan, dan Project Memory; tidak mencakup .env, data cluster, node_modules, atau binary. Tidak ada deployment atau perubahan kebijakan OPEN. Status sinkronisasi akhir harus diperiksa melalui git status -sb dan git ls-remote origin refs/heads/main. Otorisasi commit/push sesi ini tidak menjadi izin otomatis untuk pekerjaan berikutnya.

## Verifikasi sebelum baseline Git
make test, make check, make integration, dan make build dijalankan ulang dan lulus pada sesi commit/push. Pemeriksaan staged diff dan pengecualian rahasia dilakukan sebelum commit. Staged diff awal menemukan empat Markdown hard break berbentuk trailing spaces pada sumber PRD/roadmap; diubah menjadi backslash hard break tanpa mengubah isi kebijakan.

## Langkah berikutnya
1. Periksa git status, AGENTS, HANDOVER, dan kode aktual sebelum bekerja.
2. Bangun fase login/Admin pengguna-role secara bertahap mengikuti rancangan sesi/RBAC/audit di ARCHITECTURE. Tambahkan migration baru; jangan mengedit 00001 yang sudah diterapkan.
3. Pastikan tes otorisasi positif/negatif, pencabutan hak, keamanan sesi/CSRF dan audit tersedia sebelum membuka modul bisnis.
4. Putuskan P0 hanya saat fitur terkait membutuhkan. Commit/push hanya dengan instruksi eksplisit.
