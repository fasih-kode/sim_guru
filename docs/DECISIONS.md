# Catatan keputusan

## D-001 — Pembagian kewenangan (disetujui 21 Oktober 2026)
Catatan historis; rincian kelas dan penempatan awal diperbarui oleh D-015.
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

## D-011 — Checkpoint autentikasi (21 Oktober 2026)
Pengguna menyetujui commit/push autentikasi dan port 18743 sebelum master data. Commit 3c45810 telah di-push ke origin/main dan hash remote diverifikasi sama. Implementasi master data sesudah checkpoint tidak otomatis di-commit/push.

## D-012 — Master siswa/guru dan mapel (pilihan teknis dalam PRD, 21 Oktober 2026)
Migration 00003 menambah tabel terpisah per entitas dengan ID bigint permanen, unique/FK dan histori. internal/master menyatukan validasi/akses domain master tanpa membuat ORM atau dependency baru. SQL identifier berasal dari whitelist Kind; query nilai terparameterisasi. Advisory lock akses yang sudah ada diekspor agar mutasi role dan master tidak memakai pemeriksaan otorisasi yang kedaluwarsa.

Admin menulis siswa/guru; Kurikulum baca identitas terbatas tanpa kontak/relasi akun dan menulis mapel. Role lain tidak diberi akses daftar identitas seluruh sekolah sebelum cakupan tugas diimplementasikan. Batas panjang input adalah validasi teknis, bukan penetapan format nomor sekolah; tidak menebak panjang digit NIS/NISN atau keunikan NIP/NUPTK yang tidak disebut PRD.

Nomor siswa kosong disimpan NULL. Kode mapel trim/huruf besar dan seluruh kode historis dicadangkan melalui subject_codes. Tautan satu guru-satu akun opsional memakai FK/UNIQUE dan tidak menyinkronkan role/password/status akun. Koreksi memerlukan alasan, histori atomik dan version untuk mencegah lost update. Tidak ada hapus data atau penugasan akademik pada tahap ini.

## D-013 — Impor Excel siswa/guru (21 Oktober 2026)
Pengguna meminta fitur impor data guru dan siswa sebelum tahun ajaran/semester/kelas. Implementasi dalam batas master Admin yang sudah disepakati, dengan template .xlsx, pratinjau, konfirmasi, histori dan transaksi atomik. Tambah saja dipilih agar tidak menebak kebijakan merge/overwrite identitas. Tidak ada pembuatan akun, role atau penugasan melalui file.

Batas teknis: file 5 MiB, ZIP 20 MiB/200 entry, 500 baris data/2000 baris fisik; nomor identitas/telepon wajib teks, DOB mendukung dua sistem tanggal Excel. Staging 15 menit terikat owner, token hash-only, maksimal lima pending/akun, scrub payload setelah selesai/expiry, receipt idempotent. saveRecord dibagi dengan form agar insert/histori tidak memiliki implementasi aturan yang berbeda. Tidak ada perubahan pada migrasi 00001–00003; tambah 00004.

Excelize v2.11.0 semula terpilih dari rilis terbaru tetapi govulncheck menemukan empat kerentanan terjangkau. Dipatok ke v2.11.1-0.20261020033641-611b222fd8f7 (commit upstream 611b222fd8f7), lalu unit/integrasi dan scan diulang. Scan akhir tidak menemukan simbol rentan terjangkau; advisory tidak-terpakai lama tetap dicatat di TESTING. Alasan memakai revisi commit adalah perbaikan keamanan belum tersedia pada tag stabil yang diperiksa. Sumber: [repository resmi](https://github.com/qax-os/excelize) dan [API pembacaan workbook](https://xuri.me/excelize/en/workbook.html).

## D-014 — Tahun ajaran, semester, dan kelas (21 Oktober 2026)
Pengguna melanjutkan roadmap setelah impor Excel. Migration 00005 menambah kalender dan daftar kelas tanpa mengubah migration terdahulu. Membuat tahun ajaran otomatis membuat tepat Semester 1 dan 2 secara atomik. Partial unique index menjamin satu tahun aktif dan satu semester aktif maksimum; mengganti tahun menonaktifkan semester dari tahun sebelumnya. Semester hanya dapat diaktifkan bila tahun induknya aktif.

Pada implementasi awal Kurikulum satu-satunya role pengelola; kewenangan kelas kemudian diganti oleh D-015. Kelas terikat permanen ke tahun, tingkat SMP dibatasi 7–9, nama rombongan dibandingkan tanpa membedakan kapitalisasi dalam tingkat+tahun. ID stabil, koreksi nama/tingkat dengan alasan/version, histori transaksi. Tidak ada delete, perpindahan tahun atau data contoh development. Tanggal kalender dan status penutupan belum dibuat; penutupan menunggu pemeriksaan kewajiban yang bergantung pada penempatan/penugasan. Daftar kelas ditandai belum siap operasional.

## D-015 — Pemisahan kelas/Admin dan kalender/Kurikulum (disetujui 21 Oktober 2026)
Setelah audit, pemilik proyek menyetujui Admin membuat/mengoreksi kelas dan melakukan penempatan awal siswa. Kurikulum tetap menyiapkan tahun ajaran beserta dua semester, mengaktifkan tahun/semester, mengelola master mapel, menetapkan wali kelas dan guru pengampu, membuat jadwal, menjalankan transisi kenaikan/kelulusan serta memeriksa koreksi nilai Final. Kelas/penempatan awal dibaca Kurikulum sebagai masukan penugasan/jadwal. Perpindahan tengah tahun tetap mengikuti ketentuan khusus PRD; hasil transisi dapat mengubah penempatan sesuai proses transisi, bukan memberikan hak edit penempatan awal bebas. Penegasan pemeriksa koreksi Final tidak memutuskan hak koreksi langsung yang masih OPEN P0.

Alasan: memisahkan administrasi kelas/siswa dari kalender serta perencanaan mengajar. Menggantikan bagian kelas/penempatan awal pada ketentuan lama, termasuk D-014; tidak memberikan Admin hak akademik lain secara otomatis. Tiga dokumen sumber dan Project Memory diselaraskan dengan keputusan eksplisit ini.

Implementasi revisi membatasi tulis kelas ke Admin, baca kelas/histori ke Admin atau Kurikulum; kalender tetap Kurikulum. Admin membaca pilihan tahun untuk kelas tanpa hak kalender. Route kelas lama dipertahankan. Tidak memerlukan migration, perubahan role akun atau penulisan ulang data/histori kelas lama. Pemeriksaan role/status tetap di service/transaksi; menu bukan pengaman tunggal. Penempatan awal, wali/pengampu, jadwal, transisi dan koreksi Final belum diimplementasikan; keputusan kewenangan bukan bukti fitur selesai.

## D-016 — Penempatan normal awal oleh Admin (pilihan implementasi sesuai kebijakan disetujui, 21 Oktober 2026)
Migration 00006 bersifat aditif: tabel penempatan merekam satu siswa untuk satu tahun ajaran, ditautkan ke kelas yang wajib berada pada tahun yang sama oleh FK majemuk. Histori menyimpan actor, waktu dan alasan standar secara atomik. Service/HTTP hanya Admin membuat penempatan; Admin dan Kurikulum membaca daftar/histori. Siswa dicari lewat nama/NIS/NISN dan paginasi; status master ditampilkan. Karena PRD belum menetapkan status master siswa sebagai syarat kelayakan, UI tidak mengecualikan siswa nonaktif. Tidak tersedia edit/hapus, transfer tengah tahun, penempatan khusus, atau transisi lewat formulir ini. Duplikat ditolak constraint unik. Migration lama dan data akun/kelas tidak diubah. Implementasi mencakup penempatan normal, bukan penyelesaian OPEN penempatan khusus.

## Belum diputuskan
Detail penempatan khusus/transisi dan cakupan penugasan aktual, kebijakan retensi audit/sesi, serta seluruh OPEN PRD. Tidak ada keputusan implisit mengenai formula penilaian atau perubahan kewenangan Admin/Kurikulum. MFA/reset mandiri dan deployment belum dibuat.

## D-017 — TailAdmin sebagai standar visual FE (disetujui 21 Oktober 2026)
Pengguna menetapkan demo dan Free Tailwind Dashboard Template TailAdmin sebagai acuan visual default untuk seluruh frontend SIM-GURU, semua role, dan komponen baru. Sebelum implementasi, Codex wajib membuka demo utama, halaman/komponen relevan dan source resmi secara proaktif tanpa menunggu screenshot atau instruksi berulang. Gunakan varian terdekat; contoh form-in-modal mengikuti “Form in Modal” di halaman Modals. Cocokkan layout, lebar, spacing, tipografi, warna, ikon, state, responsivitas, dan interaksi, lalu periksa hasil visual pada desktop/ponsel.

Keputusan ini mengatur presentasi, bukan business rules atau data. Adaptasi tetap mengikuti Go SSR/htmx dan pola otorisasi SIM-GURU, tanpa penggantian stack/penyalinan aplikasi penuh. Jika suatu pilihan visual benar-benar memiliki beberapa varian yang mengubah alur produk, pilih varian terdekat yang sudah tersedia; klarifikasi hanya untuk dampak alur yang tidak dapat diinferensikan. Referensi: [demo](https://demo.tailadmin.com/), [Modals](https://demo.tailadmin.com/modals), [Basic Tables](https://demo.tailadmin.com/basic-tables), [source resmi](https://github.com/TailAdmin/tailadmin-free-tailwind-dashboard-template).
