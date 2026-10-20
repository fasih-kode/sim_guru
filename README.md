# SIM-GURU

Sistem informasi guru untuk satu SMP. Status: fondasi server Go, SSR/htmx, autentikasi, pengguna/role dan audit akses tersedia. Modul akademik belum diimplementasikan.

Mulai dari [instruksi agen](AGENTS.md), [handover](docs/HANDOVER.md), dan [indeks kebutuhan](docs/PRD.md).

- [Arsitektur](docs/ARCHITECTURE.md)
- [Aturan bisnis](docs/BUSINESS_RULES.md)
- [Pengujian](docs/TESTING.md)
- [Keputusan](docs/DECISIONS.md)
- [Lingkungan pengembangan](docs/DEVELOPMENT.md)

Repository: git@github.com:fasih-kode/sim_guru.git. Commit dan push memerlukan instruksi eksplisit.

## Menjalankan lokal

```sh
scripts/dev-db.sh start
npm ci
npm run build
make migrate-up
make run
```

Buka http://127.0.0.1:18743. Kredensial database hanya di .env lokal; ikuti docs/DEVELOPMENT.md. Pemeriksaan: make test, make check, make integration, dan make build. Database integrasi memakai TEST_DATABASE_URL terpisah.

Autentikasi memerlukan CSRF_AUTH_KEY lokal dan migrasi versi 2. Tidak ada akun bawaan. Buat Admin pertama melalui terminal dengan go run ./cmd/admin -username admin -name "Administrator Sekolah"; password diminta tanpa echo. Baca docs/DEVELOPMENT.md sebelum bootstrap.
