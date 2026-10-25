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
3. **Admin mengelola data induk siswa/guru, akun, role, kelas dan penempatan awal; Kurikulum mengelola kalender dan penugasan akademik.** Hak mengikuti role dan penugasan aktif. Jangan buat tabel/modul/inbox mandat, delegasi, atau persetujuan digital rutin. **Jangan buat Guru Pengganti/Cadangan, penugasan sementara, atau hak sementara untuk menggantikan guru berhalangan.**
4. Kepala Sekolah hanya menerima dashboard/laporan dan terlibat pada keputusan akademik khusus. Perpindahan tengah tahun menyimpan catatan keputusan Kepala Sekolah, tanpa workflow digital.
5. Untuk koreksi jurnal/absensi gunakan alasan+histori. Untuk nilai Final gunakan pemeriksaan Kurikulum sederhana; bukan approval Kepala Sekolah.
6. Bila P0 belum ditetapkan, hentikan fitur terdampak saja. Jangan mengulang audit kontrak lama tanpa alasan konkret.
7. Gunakan migrasi naik/turun, database pengujian terpisah, tes Go, tes integrasi, otorisasi server, pemeriksaan asset frontend, dan Git diff.

## 1A. Kontrak kepemilikan data — FINAL (revisi 21 Oktober 2026)

- **Admin:** membuat, melengkapi, memperbarui, menonaktifkan data induk siswa/guru; mengelola akun/role, membuat/mengoreksi kelas dan melakukan penempatan awal siswa.
- **Kurikulum:** hanya membaca identitas siswa/guru, lalu membaca kelas/penempatan awal dan mengelola mata pelajaran, wali kelas, pengampu resmi, jadwal, persiapan tahun beserta dua semester, aktivasi tahun/semester, transisi kenaikan/kelulusan dan pemeriksaan koreksi nilai Final.
- Role lain hanya mengelola data operasional sesuai tugas; identitas siswa/guru baca-saja.
- Penugasan wali/pengampu, jadwal dan transisi **bukan** tugas Admin; kelas dan penempatan awal adalah tugas Admin. Perubahan role oleh Admin dan perubahan penugasan oleh Kurikulum langsung berlaku setelah validasi, tanpa approval digital.
- Uji negatif wajib: Kurikulum tidak bisa PATCH identitas siswa/guru; Kurikulum tanpa Admin tidak bisa POST kelas/penempatan awal; Admin tanpa Kurikulum tidak bisa POST kalender atau penugasan guru; guru lain tidak bisa mengubah identitas. Uji positif: perubahan identitas Admin tercermin di seluruh modul melalui ID internal stabil.

## 2. Urutan fase

| Fase | Pekerjaan | Bukti lulus |
|---|---|---|
| 0 | Baca 3 dokumen, inspeksi mesin dan repo | Catatan lingkungan aktual dan P0 fase berikut |
| 1 | Rancang modul Go, DB PostgreSQL, sesi, RBAC sederhana, audit | Rancangan sesuai stack; tanpa approval berlapis |
| 2 | Fondasi Go, config, koneksi DB, migrasi, layout TailAdmin, htmx | Server hidup; halaman SSR; fragmen htmx; DB ping; tes |
| 3 | Login, Admin pengguna dan role | Admin memberi role Kurikulum → akses modul Kurikulum aktif; audit dan akses negatif lulus |
| 4 | Admin CRUD data induk siswa/guru; Kurikulum master mata pelajaran | Validasi, unik, nonaktif, histori teruji; Kurikulum tidak dapat edit identitas |
| 5 | Kurikulum menyiapkan/mengaktifkan tahun beserta dua semester; Admin mengelola kelas | Aktif tunggal, dua semester, kelas unik teruji |
| 6 | Penempatan awal oleh Admin; penugasan wali kelas dan transisi akhir tahun oleh Kurikulum | Penempatan awal dan wali kelas tersedia; transisi akhir tahun tersedia dengan pratinjau per siswa, kelas tujuan, validasi, batch atomik dan histori; pindah kelas tengah tahun Kurikulum dan pindah sekolah dan penerimaan kembali siswa lama oleh Admin tersedia (D-031/D-032) |
| 7 | Pengampu tunggal per semester–kelas–mapel oleh Kurikulum → jam pelajaran per hari → jadwal mingguan manual Draf/Publikasi → perubahan efektif dan histori; salin penugasan semester sebelumnya untuk disesuaikan | Guru boleh banyak kelas/mapel; benturan waktu guru/kelas dan jadwal di luar penugasan ditolak; draf tidak tampil sebagai jadwal publik guru; histori utuh; tanpa Guru Pengganti/Cadangan |
| 7A | **Inti diterapkan (D-024/D-025):** struktur JP/pertemuan per semester, bawaan tingkat dan pengecualian rombel → istirahat/kegiatan Upacara jam ke-1 → generator draf → tinjau/edit → publikasi | Matematika 7A 2 JP dan 7B 5 JP bisa berdampingan; semester baru bisa 8 JP tanpa ubah kode; istirahat/upacara terblokir dan tampil; validasi JP/pertemuan/pengampu/bentrok; kegagalan generate jujur; snapshot/histori dan data lama utuh |
| 7B | **Diterapkan (D-026/D-027, migration 00011):** Batasan Jadwal guru/mapel per semester/hari/waktu aktual → Tidak tersedia atau Usahakan dihindari → kapasitas per guru → pencarian preferensi dan laporan draf → validasi edit/publikasi | Larangan wajib tidak dilanggar; JP tetap lengkap; preferensi pagi diusahakan pada sesedikit mungkin hari dan ketidakterpenuhannya dilaporkan; PJOK mengikuti waktu aktual; tidak mengklaim optimal/mustahil karena timeout; snapshot/histori dan versi terbit utuh |
| 7C | **Diterapkan (D-028):** kisi hari × nomor jam untuk guru/mapel, Tersedia/Kondisional/Tidak tersedia, tandai hari/reset editor/simpan | Pemetaan rentang lama dengan tinjauan, slot kegiatan terkunci, transaksi/token konflik, tinjau ulang setelah jam berubah, generator/validator dan versi terbit tetap terjaga |
| 7D | **Diterapkan (D-029):** unggah XML aSc → padanan data/hari/jam → pratinjau → simpan draf → tinjau/publikasi | Ekspor terpotong ditahan; validasi pengampu/JP/pola/bentrok/batasan, persetujuan waktu/penggantian/draf belum lengkap, transaksi dan konfirmasi idempoten, versi terbit/master tetap |
| 8 | D-033: fondasi kalender/tanggal penempatan/akses → harian siswa QR/kartu → pelajaran → guru GPS/selfie/manual/retensi → rekap | Foto maksimal 100 KB/dua bulan; kartu hilang rotasi kode; roster/jadwal per tanggal, koreksi historis, Belum Dicatat aman; implementasi bertahap dicatat HANDOVER |
| 8A | Kesiswaan/BK sederhana: catatan pembinaan dan bimbingan sesuai PRD bab 3B | Field sederhana dan dua status; Kesiswaan seluruh sekolah, BK siswa/kasus yang ditangani; baca Wali Kelas/Kepala Sekolah sesuai cakupan; tanpa poin/sanksi otomatis/approval |
| 9 | Jurnal, koreksi, anomali | Draf/Final, alasan koreksi, histori; tanpa inbox approval |
| 10 | Penilaian, remedial, finalisasi, koreksi | Rumus disetujui, guru terbatas penugasan, koreksi Final diperiksa Kurikulum |
| 11 | Laporan dan dashboard Kepala Sekolah | Hak akses dan kelengkapan benar; bukan rapor resmi |
| 12 | Penyempurnaan FE dan UX | Uji alur nyata Admin/Kurikulum/Guru/Wali/Kepala Sekolah/Kesiswaan/BK |
| 13 | UAT, keamanan, backup/restore, performa | Tes end-to-end dan pemulihan lulus |
| 14 | Deployment dan panduan operasional | Runbook, konfigurasi, backup, pelatihan, rencana rollback |

## 3. Ketergantungan dan OPEN

- Role/penugasan mendahului operasi Guru/Wali Kelas. Master data → tahun/semester/kelas → penempatan → jadwal → absensi/jurnal/nilai → laporan.
- Fase 6: penugasan Wali Kelas diterapkan lewat migration 00007, menu Kurikulum pada daftar kelas, histori pergantian, dan menu Kelas Saya dengan roster terbatas ([D-019](docs/DECISIONS.md#d-019--cakupan-penugasan-wali-kelas-melalui-role-aktif-diterapkan-22-oktober-2026)). Untuk transisi akhir tahun, Kurikulum membuat tahun/semester tujuan tetapi membiarkannya belum aktif; Admin membuat kelas tujuan; Kurikulum mentransisikan siswa sebelum mengaktifkan kalender baru. Fitur tersedia pada menu Kurikulum → Transisi Siswa dengan pratinjau per siswa, batch atomik dan histori ([D-030](docs/DECISIONS.md#d-030--kurikulum-menjalankan-transisi-siswa-tanpa-approval-kepala-sekolah-23-oktober-2026)). Pindah kelas tengah tahun Kurikulum dan pindah sekolah keluar Admin kini tersedia melalui D-031/migration 00015, dengan tanggal berlaku, ringkasan, histori dan roster efektif.
- Fase 8 urutan 1 tersedia melalui migration 00018: kalender semester/hari sekolah/pengecualian Kurikulum, tanggal awal penempatan Admin, dan konteks roster/jadwal bertanggal berotorisasi. Tanggal lama perlu ditinjau dan dilengkapi; QR/GPS/selfie/retensi belum diterapkan. Bukti verifikasi pada HANDOVER.
- Fase 8A adalah rencana yang disetujui, belum diimplementasikan. Menggunakan master/penempatan dan penugasan Wali Kelas sebagai dasar akses, serta absensi dari fase 8 sebagai sumber rekap Kesiswaan. Ketentuan otoritatif ada di [PRD bab 3B](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#3b-kesiswaan-dan-bk--final-22-oktober-2026); alasan dan batas pengembangan di [D-018](docs/DECISIONS.md#d-018--kesiswaan-dan-bk-sederhana-disetujui-22-oktober-2026). Tetapkan cara mencatat penanggung jawab BK saat fase terkait, tanpa menambahkan approval. Tidak mengubah urutan pekerjaan penugasan/jadwal yang masih tersisa.
- Fase 7 telah diimplementasikan pada 22 Oktober 2026 sesuai [PRD bab 6](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#6-penugasan-dan-jadwal--final-inti) dan [D-020](docs/DECISIONS.md#d-020--pengampu-per-semester-dan-jadwal-mingguan-sederhana-disetujui-22-oktober-2026): penugasan, salin semester sebelumnya, jam pelajaran, draf/publikasi mingguan, pemeriksaan konflik dan jadwal Guru Mapel. Build/static checks dan tes terfokus template jam/jadwal tersedia; cakupan copy semester, konkurensi assignment dan Jadwal Saya masih memerlukan pengujian khusus sebelum seluruh fase dinyatakan selesai. Pengampu bersama tetap ditunda; inti generator/pengaturan fase 7A kini diterapkan; bukti dan batas di HANDOVER.
- Fase 7A mengikuti [PRD bab 6.1](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#61-rancangan-pengembangan-jadwal-fleksibel-dan-generator) dan [D-024](docs/DECISIONS.md#d-024--rancangan-jadwal-fleksibel-per-rombel-dan-generator-22-oktober-2026). Persetujuan lanjutan D-025 mengizinkan pertemuan disambung setelah istirahat serta penerapan fitur. Migration 00010 dan solver Go dengan budget 8 detik tersedia; generate draf atomik, tinjau/edit dan validasi publikasi diterapkan. Tes mencakup pengecualian rombel, 8 JP semester berikutnya, kegagalan/timeout, penggantian draf, akses, serta pelestarian versi terbit. Ketersediaan individu guru belum dikonfigurasi; semua guru dianggap tersedia pada jam sekolah. Rincian verifikasi dan kesiapan data development ada di HANDOVER.
- Fase 7B mengikuti [PRD bab 6.2](PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#62-rancangan-batasan-jadwal-dan-preferensi) dan [D-026](docs/DECISIONS.md#d-026--batasan-wajib-dan-preferensi-jadwal-22-oktober-2026). Urutan yang direkomendasikan: rinci model/versi/cakupan dan penilaian preferensi, buat migration aditif tanpa mengubah migration yang telah diterapkan, pengaturan Kurikulum, validator wajib/kapasitas guru dan snapshot/fingerprint, lalu pencarian preferensi, laporan serta pemeriksaan edit/publikasi. Uji larangan mapel berdasarkan waktu aktual, hari guru terpilih, Jumat penuh, contoh 36 JP dengan 34 slot jika wajib versus tetap bisa memakai slot pagi jika preferensi, pertemuan melintasi istirahat, perubahan konfigurasi, kegagalan/timeout, serta pelestarian versi terbit. Implementasi fase 7B tersedia sesuai D-027; hasil aktual verifikasi dan migration development dicatat di HANDOVER.
- P0 penilaian: komponen TP, bobot, pembulatan, remedial, koreksi nilai Final. Jangan membuat rumus sendiri.
- P0 absensi: parameter teknis GPS/akurasi sesuai D-033 dan indikator anomali. Mekanisme GPS/selfie guru serta QR harian siswa telah diputuskan pada D-033. P0 laporan: katalog dan cakupan. P0 akademik: aturan penutupan semester dan penempatan khusus.
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
| Absensi | Kalender/roster/jadwal bertanggal dan akses; QR siswa/rotasi/duplikat; GPS/selfie/fallback/100 KB/retensi dua bulan; harian vs pelajaran, Belum Dicatat |
| Kesiswaan/BK (fase 8A, rencana) | Kesiswaan seluruh sekolah; BK tidak mengakses siswa/kasus di luar tanggung jawabnya; Wali Kelas hanya baca kelasnya; Kepala Sekolah baca seluruh sekolah; dua status; penolakan edit oleh pembaca dan akses tanpa hak; catatan tidak mengubah sumber absensi/identitas/nilai/jadwal |
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

Penyempurnaan fase 7 — D-021 (22 Oktober 2026): template jam Senin–Sabtu/1–12, pengisian waktu per hari secara atomik, pilihan kelas–mapel dengan guru otomatis dan dropdown rentang jam sesuai hari. Memakai data/skema yang ada; definisi lama tetap. Pengujian service/HTTP terfokus ditambahkan; rincian hasil aktual dan batas cakupan dicatat di HANDOVER/TESTING. Integrasi aSc dilewatkan sesuai arahan pengguna.

Penyempurnaan fase 6 — D-032: Terima kembali siswa lama oleh Admin, tanggal efektif/kelas/riwayat, reuse placement tahun sama, placement baru hanya bila tahun aktif belum ada, serta siklus keluar–kembali berulang. Migration 00016; roster/transisi menjaga masa jeda. Tidak mencakup pembatalan/koreksi kejadian atau penerimaan pindahan baru.

Pelengkap fondasi Fase 8 (D-034): pengisian tanggal penempatan kosong massal Admin dengan pilihan lintas halaman, pratinjau/konfirmasi, pemeriksaan konflik dan histori atomik; saran awal Semester 1 pada form massal/penempatan baru. Modul harian siswa QR tetap urutan berikutnya.

Fase 8 urutan 2 (D-035): implementasi harian siswa, kartu QR/cetak/ganti, pemindai/USB, manual dan koreksi berhistori. Rincian validasi/pemasangan dan batas pengujian mengikuti HANDOVER. Absensi per pelajaran (urutan 3), GPS/selfie/retensi guru (4), rekap gabungan (5) tetap berikutnya; jangan menganggap seluruh Fase 8 selesai.

Pelengkap urutan 2 D-036: batas kedatangan oleh Kesiswaan, Hadir berjam wajib, penilaian keterlambatan snapshot, Catat datang sekarang dan Isi cepat Petugas per kelas. Tidak menganggap absensi pelajaran/GPS/selfie/rekap lanjutan selesai.
