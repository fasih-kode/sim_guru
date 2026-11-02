# Instruksi permanen SIM-GURU

## Awal sesi
1. Periksa `git status --short` dan perubahan pengguna; jangan menimpa perubahan yang belum di-commit.
2. Baca dokumen ini, [HANDOVER](docs/HANDOVER.md), [indeks PRD](docs/PRD.md), dan dokumentasi relevan.
3. Periksa implementasi aktual. Dokumentasi tidak otomatis benar ketika bertentangan dengan kode; laporkan perbedaan dan minta keputusan untuk konflik kebijakan.
4. Jelaskan rencana serta dampak sebelum perubahan besar. Gunakan bahasa Indonesia untuk laporan.

## Implementasi
- **Port preview D-051:** development dan UAT sintetis boleh berjalan bersamaan hanya pada pasangan tetap: DEV `http://127.0.0.1:18743` → `sim_guru_dev`; UAT `http://localhost:18744` → `sim_guru_ci_test`. `make run` tetap menjalankan DEV; `make run-uat` memuat konfigurasi privat mode 0600 di luar repository, memvalidasi target/port/kunci CSRF terpisah, memastikan versi database UAT sama dengan target migrasi source melalui pemeriksaan status (tanpa menjalankan migrasi), serta menonaktifkan storage foto. Jangan menambah port fase lain, mengalihkan `.env`, atau menjalankan UAT pada development. Migrasi setiap database dilakukan eksplisit dan terpisah dari startup; perubahan kode perlu restart proses yang memakai source itu. Hentikan proses hanya setelah mengidentifikasi pemiliknya dan gunakan SIGTERM/Ctrl+C. Cookie session/CSRF host-only dipisahkan oleh hostname `127.0.0.1` dan `localhost`; jangan menyamakan keduanya. Pengujian HTTP otomatis tetap memakai `httptest` atau mekanisme in-process dan tidak meninggalkan preview server.
- Database development `sim_guru_dev`, database test terisolasi, dan database UAT sintetis memiliki tujuan berbeda. Jangan menjalankan suite otomatis dan sesi UAT bersamaan pada database yang sama. Jangan reset atau membersihkan database/fixture, kredensial aktif, file pengguna, atau artefak sementara kecuali kepemilikan, status tidak dipakai, dan dampaknya sudah dipastikan.
- Ikuti arsitektur, stack, dan konvensi yang berlaku. Fondasi aplikasi tersedia; baca ARCHITECTURE dan kode aktual sebelum menambah modul atau API.
- **Standar FE SIM-GURU: TailAdmin Free adalah acuan visual default untuk seluruh halaman, semua role, dan komponen baru.** Sebelum membuat/mengubah FE, buka demo TailAdmin dan periksa halaman/komponen paling dekat serta source repository; jangan menunggu pengguna mengirim screenshot atau mengulang referensi. Cocokkan layout, lebar konten, jarak, tipografi, warna, border/radius, ikon, state, responsivitas, dan interaksi. Contoh: permintaan form di modal mengikuti komponen Form in Modal pada `/modals`; tabel mengikuti `/basic-tables`. Gunakan komponen/varian terdekat bila permintaan tidak menyebut detail.
- **Navigasi global (D-047):** bila kelompok menu memiliki lebih dari satu halaman terkait, sidebar hanya menampilkan link kelompok/halaman awal yang diizinkan dan submenu antarhalaman wajib memakai horizontal navigation tabs bersama dengan ikon serta underline aktif. Gunakan `internal/web/section_navigation.go`, partial `section-navigation-tabs`, dan kelas `.navigation-*`; jangan membuat markup, CSS, atau sistem tab kedua. Daftarkan kelompok, route, dan filter role di kontrak bersama, pertahankan link native dan `aria-current="page"`, satu tab aktif, parent sidebar aktif, serta route detail/tambah/edit. Sembunyikan tab yang tidak berhak tanpa mengubah otorisasi server atau menambah permission. Kelompok satu halaman tidak perlu tab; form, dialog, dan wizard bukan tab navigasi. Uji role gabungan dan tampilan desktop/ponsel. Rincian permanen ada di [UI Standards](docs/UI_STANDARDS.md).
- **Seluruh pencarian pada halaman daftar SIM-GURU wajib menggunakan Instant Search sebagai standar frontend. Pencarian dan perubahan filter memperbarui hasil secara langsung tanpa submit manual atau full-page reload ketika JavaScript tersedia.** Debounce server default 350 ms; gunakan HTMX dan pola bersama `instant-search.js`/`list-fragment`. Pencarian lokal hanya bila seluruh data relevan dalam cakupan tersedia di browser, bukan 10 baris pertama pagination SQL. Pertahankan fokus, nilai input, kombinasi filter, pagination, URL dan batas role; gunakan komponen TailAdmin Free yang konsisten. Lindungi modal dan draft/form mutasi dari swap; jangan membuat implementasi berbeda untuk fitur baru tanpa alasan. Lihat [inventaris dan kontrak](docs/INSTANT_SEARCH_AUDIT.md).
- Adaptasikan pola TailAdmin ke stack SSR/htmx dan pola akses SIM-GURU; pertahankan data, business rules, otorisasi, aksesibilitas, dan konvensi proyek. Jangan mengganti stack atau menyalin seluruh template aplikasi tanpa kebutuhan. Verifikasi hasil FE secara visual pada desktop dan ponsel, bandingkan dengan referensi; jangan menyatakan cocok bila belum diperiksa.
- Standar UI global dan pola navigasi D-047 dirinci di [UI Standards](docs/UI_STANDARDS.md); gunakan komponen tabs bersama untuk setiap kelompok baru yang memiliki submenu/lebih dari satu halaman. Ini wajib dalam definisi selesai frontend.
- Jangan meminta klarifikasi visual yang sudah dapat dijawab dengan membuka referensi. Jika referensi tidak dapat diakses, cari source resmi. Tanyakan hanya bila ada beberapa varian yang materially berbeda dan pilihan memengaruhi alur/aturan produk; jika tidak, pilih varian TailAdmin yang paling dekat dan lanjutkan.
- Admin mengelola identitas siswa/guru, akun, role, pembuatan/koreksi kelas dan penempatan awal siswa. Kurikulum menyiapkan/mengaktifkan tahun ajaran beserta dua semester, mengelola mapel, wali kelas, pengampu, jadwal, transisi kenaikan/kelulusan dan pemeriksaan koreksi nilai Final. Kurikulum membaca kelas tanpa hak membuat/mengoreksi kelas; Admin tidak otomatis mendapat hak kalender/penugasan/nilai. Lihat PRD bab 3A dan keputusan D-015. Jangan mengubah aturan bisnis tanpa persetujuan.
- Utamakan perubahan kecil, terfokus, mudah diuji; clean code, pemisahan tanggung jawab, dan penanganan error eksplisit.
- Jangan melakukan refactoring di luar cakupan tanpa alasan dan persetujuan. Perubahan arsitektur besar memerlukan persetujuan.
- Tambahkan/perbarui pengujian untuk perubahan perilaku; jalankan pengujian dan pemeriksaan statis relevan. Jangan mengklaim lulus tanpa menjalankannya.
- Jangan mengubah migration yang sudah diterapkan tanpa strategi aman. Gunakan transaksi untuk operasi atomik dan database pengujian terpisah.
- Jangan menghapus data, reset database, atau menjalankan tindakan destruktif tanpa persetujuan eksplisit dan strategi pemulihan.
- Jangan menyimpan password, token, key, atau rahasia ke dokumentasi/Git/log. Otorisasi, validasi, dan CSRF harus di server.
- Jangan commit atau push tanpa instruksi eksplisit. Jangan membatalkan pekerjaan pengguna.
- P0 OPEN hanya menahan fitur terdampak. Jangan menebak rumus nilai atau menambah guru pengganti, mandat, dan approval rutin.

## Akhir sesi
1. Tinjau diff dan file baru; periksa rahasia serta artefak yang tidak diperlukan.
2. Jalankan verifikasi relevan; laporkan yang berhasil, gagal, atau tidak dijalankan beserta alasannya.
3. Perbarui dokumentasi terdampak dan [HANDOVER](docs/HANDOVER.md) sebelum mengakhiri pekerjaan.
4. Catat perubahan, bukti verifikasi, risiko, pekerjaan wajib tersisa, dan langkah berikutnya.

## Tata kelola sumber
[PRD](docs/PRD.md) menunjuk dokumen kebutuhan otoritatif. [DECISIONS](docs/DECISIONS.md) mencatat keputusan; jangan menggandakan kebutuhan lengkap. Bedakan fakta terverifikasi, keputusan disetujui, rekomendasi belum disetujui, dan pekerjaan belum selesai. Jika sumber bertentangan, laporkan dan minta keputusan sebelum mengganti kebijakan; lanjutkan pekerjaan independen yang aman.

# Pedoman Pengembangan SIM-GURU

## Prinsip
- Prioritaskan readability, maintainability, dan testability.
- Gunakan idiom Go dan struktur sederhana.
- Hindari overengineering dan abstraksi yang tidak perlu.
- Pertahankan perilaku aplikasi saat refactoring.
- Jangan mengubah aturan bisnis tanpa persetujuan.

## Komentar
- Tulis komentar penjelasan dalam bahasa Indonesia.
- Utamakan penjelasan alasan dan batasan bisnis.
- Hindari komentar yang hanya mengulang kode.

## Pengujian
- Tambahkan regression test untuk perubahan berisiko.
- Jalankan go test ./... dan go vet ./...
- Jangan menyatakan test lulus tanpa menjalankannya.

## Teknologi
- Go, html/template, htmx, PostgreSQL.
- TailAdmin Free dan Tailwind CSS.
- Jangan mengganti stack tanpa persetujuan.

## Cara Kerja
- Kerjakan perubahan kecil dan terukur.
- Periksa diff serta dampak perubahan.
- Catat temuan yang tidak mendesak sebagai backlog.
- Jangan menjadikan urusan monitoring atau deployment
  sebagai penghalang refactoring lokal.
