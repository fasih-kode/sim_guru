# ROADMAP IMPLEMENTASI SIM-GURU v1.0

**Tujuan:** panduan agen AI membangun SIM-GURU dari nol sampai siap dipakai pada satu SMP.\
**Prinsip:** sederhana untuk pengguna, ketat pada integritas data dan keamanan. Tidak ada sistem mandat/delegasi/approval rutin.

## 0. Teknologi terkunci dan inspeksi Ubuntu

- OS pengembangan: Ubuntu 26.04 LTS; backend Go; `html/template`; htmx; PostgreSQL; Tailwind CSS; TailAdmin Free HTML.
- Sumber TailAdmin: https://github.com/TailAdmin/tailadmin-free-tailwind-dashboard-template
- Alpine.js/Node/npm hanya pendukung komponen dan build; bukan backend.
- Periksa `lsb_release -a`, `go version`, `psql --version`, `node --version`, `npm --version`, `git --version`, RAM/disk, port dan status repo sebelum mengubah mesin. Jika alat belum ada, laporkan dahulu.
- Jangan mengubah versi OS, menimpa database proyek lain, atau menaruh rahasia `.env` di Git.

## 1. Aturan kerja agen AI

1. Baca PRD utama dan PRD FE sebelum menulis kode. Jadikan bab **Model kewenangan sederhana** sebagai aturan terbaru yang menggantikan mandat/delegasi lama.
2. Kerjakan fase kecil dengan tes dan review; jangan menambah fitur tanpa dasar PRD.
3. **Admin mengelola data induk siswa/guru, akun, dan role; Kurikulum mengelola penugasan akademik.** Hak mengikuti role dan penugasan aktif. Jangan buat tabel/modul/inbox mandat, delegasi, atau persetujuan digital rutin. **Jangan buat Guru Pengganti/Cadangan, penugasan sementara, atau hak sementara untuk menggantikan guru berhalangan.**
4. Kepala Sekolah hanya menerima dashboard/laporan dan terlibat pada keputusan akademik khusus. Perpindahan tengah tahun menyimpan catatan keputusan Kepala Sekolah, tanpa workflow digital.
5. Untuk koreksi jurnal/absensi gunakan alasan+histori. Untuk nilai Final gunakan pemeriksaan Kurikulum sederhana; bukan approval Kepala Sekolah.
6. Bila P0 belum ditetapkan, hentikan fitur terdampak saja. Jangan mengulang audit kontrak lama tanpa alasan konkret.
7. Gunakan migrasi naik/turun, database pengujian terpisah, tes Go, tes integrasi, otorisasi server, pemeriksaan asset frontend, dan Git diff.

## 1A. Kontrak kepemilikan data — FINAL (revisi 21 Oktober 2026)

- **Admin:** membuat, melengkapi, memperbarui, menonaktifkan data induk siswa/guru; mengelola akun dan role.
- **Kurikulum:** hanya membaca identitas siswa/guru, lalu mengelola mata pelajaran, kelas, penempatan, wali kelas, pengampu resmi, jadwal, tahun/semester dan transisi.
- Role lain hanya mengelola data operasional sesuai tugas; identitas siswa/guru baca-saja.
- Penugasan akademik **bukan** tugas Admin. Perubahan role oleh Admin dan perubahan penugasan oleh Kurikulum langsung berlaku setelah validasi, tanpa approval digital.
- Uji negatif wajib: Kurikulum tidak bisa PATCH identitas siswa/guru; Admin tidak bisa POST penempatan atau penugasan guru; guru lain tidak bisa mengubah identitas. Uji positif: perubahan identitas Admin tercermin di seluruh modul melalui ID internal stabil.

## 2. Urutan fase

| Fase | Pekerjaan | Bukti lulus |
|---|---|---|
| 0 | Baca 3 dokumen, inspeksi mesin dan repo | Catatan lingkungan aktual dan P0 fase berikut |
| 1 | Rancang modul Go, DB PostgreSQL, sesi, RBAC sederhana, audit | Rancangan sesuai stack; tanpa approval berlapis |
| 2 | Fondasi Go, config, koneksi DB, migrasi, layout TailAdmin, htmx | Server hidup; halaman SSR; fragmen htmx; DB ping; tes |
| 3 | Login, Admin pengguna dan role | Admin memberi role Kurikulum → akses modul Kurikulum aktif; audit dan akses negatif lulus |
| 4 | Admin CRUD data induk siswa/guru; Kurikulum master mata pelajaran | Validasi, unik, nonaktif, histori teruji; Kurikulum tidak dapat edit identitas |
| 5 | Tahun ajaran, semester, kelas | Aktif tunggal, dua semester, kelas unik teruji |
| 6 | Penempatan, penugasan wali kelas oleh Kurikulum, transisi | Penempatan normal unik; pratinjau; batch atomik; tidak ada approval digital rutin |
| 7 | Penugasan pengampu resmi oleh Kurikulum, jadwal, perubahan efektif dan histori | Konflik ditolak; histori utuh; tidak ada penugasan atau akses Guru Pengganti/Cadangan |
| 8 | Absensi guru QR dan siswa harian/pelajaran | QR tidak menyimpulkan mengajar; status belum dicatat aman |
| 9 | Jurnal, koreksi, anomali | Draf/Final, alasan koreksi, histori; tanpa inbox approval |
| 10 | Penilaian, remedial, finalisasi, koreksi | Rumus disetujui, guru terbatas penugasan, koreksi Final diperiksa Kurikulum |
| 11 | Laporan dan dashboard Kepala Sekolah | Hak akses dan kelengkapan benar; bukan rapor resmi |
| 12 | Penyempurnaan FE dan UX | Uji alur nyata Admin/Kurikulum/Guru/Wali/Kepala Sekolah |
| 13 | UAT, keamanan, backup/restore, performa | Tes end-to-end dan pemulihan lulus |
| 14 | Deployment dan panduan operasional | Runbook, konfigurasi, backup, pelatihan, rencana rollback |

## 3. Ketergantungan dan OPEN

- Role/penugasan mendahului operasi Guru/Wali Kelas. Master data → tahun/semester/kelas → penempatan → jadwal → absensi/jurnal/nilai → laporan.
- P0 penilaian: komponen TP, bobot, pembulatan, remedial, pengampu bersama, koreksi nilai Final. Jangan membuat rumus sendiri.
- P0 absensi: validasi QR/fallback, anomali. P0 laporan: katalog dan cakupan. P0 akademik: aturan penutupan semester dan penempatan khusus.
- P1: format ekspor, tenggat jurnal, notifikasi, atribut opsional. P1 tidak menghalangi fase aman.

## 4. Kriteria fase 3: penyederhanaan hak akses (wajib)

1. Admin membuat akun guru dan memilih role **Kurikulum**. Setelah simpan, guru dapat memakai fitur Kurikulum tanpa persetujuan Kepala Sekolah.
2. Kurikulum menetapkan guru sebagai **Wali Kelas 7A**. Hak kelas 7A langsung aktif; kelas lain tidak otomatis terbuka.
3. Kurikulum menetapkan Guru Mapel PAI untuk 7A/7B. Guru tidak dapat mengakses nilai kelas 8A.
4. Admin mengganti atau mencabut role. Hak server-side ikut berubah dan perubahan dicatat di audit.
5. Tidak ada route/menu/tabel untuk pengajuan mandat, delegasi, inbox approval, atau self-approval rutin.
6. Admin tanpa role Guru Mapel tidak bisa mengubah nilai hanya karena ia Admin.

## 5. Kriteria pengujian lintas modul

**Tes regresi penghapusan Guru Pengganti/Cadangan:** tidak ada role/menu/route/form/API/tabel baru untuk pengganti; guru yang tidak ditugaskan tidak dapat mengisi jurnal, absensi pelajaran, atau nilai meskipun guru pengampu berhalangan; ketidakhadiran tidak memicu akses sementara; pergantian pengampu **resmi** melalui Kurikulum berlaku menurut tanggal efektif dan menyimpan histori; jadwal tanpa pembelajaran tidak otomatis dibuatkan jurnal Final. Jika repository sudah memuat fitur pengganti, audit dependensi dan migrasi secara aman, jangan menghapus data produksi tanpa rencana.

| Domain | Skenario minimum |
|---|---|
| Role/penugasan | Role langsung aktif, cabut akses, multi-role, akses URL ilegal |
| Akademik | Tahun/semester aktif tunggal, penempatan normal unik, transisi atomik |
| Jadwal | Benturan, publikasi, perubahan pengampu resmi, histori; tidak ada pengganti/cadangan |
| Absensi | QR gagal, datang/pulang, harian vs pelajaran, Belum Dicatat |
| Jurnal | Draf/Final, koreksi beralasan, hanya pengampu resmi, histori; tidak ada jurnal otomatis saat guru absen |
| Nilai | Rekomendasi vs akhir, remedial, koreksi Final oleh alur Kurikulum |
| Laporan | Final/Sementara/Belum Lengkap, filter akses, riwayat |
| Operasional | Backup/restore, CSRF, sesi, tes regresi, asset TailAdmin lokal |

## 6. Pola kerja tiap iterasi

```text
FASE/FITUR:
KEPUTUSAN FINAL yang dipenuhi:
OPEN yang benar-benar menghalangi:
FILE yang diubah:
TES dijalankan dan hasil nyata:
AUDIT akses, data historis, regresi:
RISIKO tersisa:
LANGKAH berikutnya:
```

**Larangan:** jangan menyebut fitur selesai hanya karena halaman tampil; jangan mengimplementasikan approval berlapis, dan jangan meminta persetujuan pemilik proyek untuk keputusan teknis kecil yang tidak mengubah aturan bisnis.

## 7. Instruksi pembuka untuk agen AI

> Baca ketiga dokumen SIM-GURU. Mulai dengan inspeksi Ubuntu 26.04 LTS dan repo, kemudian buat rencana implementasi Fase 0–2 yang ringkas. Gunakan Go, html/template, htmx, PostgreSQL, dan TailAdmin Free HTML. Model akses sederhana: Admin mengatur role; Kurikulum mengatur penugasan akademik; hak langsung aktif; Kepala Sekolah bukan approver harian. Tidak ada Guru Pengganti atau Guru Cadangan; hanya perubahan pengampu resmi dengan histori. Jangan membuat modul mandat/delegasi atau inbox persetujuan rutin. Kerjakan bertahap, jalankan tes, dan laporkan bukti. Hanya tanyakan kebijakan P0 ketika fitur terkait benar-benar membutuhkan keputusan.
