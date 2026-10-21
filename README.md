# SIM-GURU

Sistem informasi guru untuk satu SMP. Fondasi server Go, autentikasi, pengguna/role, master siswa/guru/mapel, impor Excel, serta pengelolaan tahun ajaran/semester/kelas tersedia. Penempatan awal siswa oleh Admin kini tersedia; penugasan wali kelas/pengampu, jadwal, transisi dan operasional akademik lanjutan belum diimplementasikan.

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

Autentikasi memerlukan CSRF_AUTH_KEY lokal dan migrasi versi 6. Tidak ada akun bawaan. Buat Admin pertama melalui terminal dengan go run ./cmd/admin -username admin -name "Administrator Sekolah"; password diminta tanpa echo. Baca docs/DEVELOPMENT.md sebelum bootstrap.

Master data: Admin mengelola siswa/guru; Kurikulum membaca identitas dan mengelola mata pelajaran. Koreksi/status menyimpan alasan serta histori. Gunakan akun Admin yang sudah dibuat; jangan mengulang bootstrap pada database berisi akun.

Admin dapat memakai Impor Excel pada Data Siswa/Data Guru: unduh template .xlsx, isi sheet Data, unggah untuk pratinjau, lalu konfirmasi. Maksimal 5 MiB/500 baris. Impor menambah data baru secara atomik dan tidak menimpa identitas lama; lihat docs/DEVELOPMENT.md.

Role Kurikulum menyiapkan tahun ajaran beserta Semester 1 dan 2 serta memilih tahun/semester aktif. Admin membuat/mengoreksi kelas; Kurikulum membaca kelas dan historinya. Admin mengelola penempatan awal di Administrasi → Penempatan Siswa; satu siswa hanya dapat ditempatkan sekali per tahun ajaran. Kurikulum dapat membaca penempatan. Perpindahan, transisi, wali kelas dan pengampu belum tersedia.
