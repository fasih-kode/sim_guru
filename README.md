# SIM-GURU

Sistem informasi guru untuk satu SMP. Status: fondasi server Go, SSR/htmx, aset lokal dan migrasi tersedia. Login serta modul bisnis belum diimplementasikan.

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

Buka http://127.0.0.1:8080. Kredensial database hanya di .env lokal; ikuti docs/DEVELOPMENT.md. Pemeriksaan: make test, make check, make integration, dan make build. Database integrasi memakai TEST_DATABASE_URL terpisah.
