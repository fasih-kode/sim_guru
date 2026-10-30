# Indeks kebutuhan produk

Sumber otoritatif tetap berada di root:
- [PRD utama](../PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md) — kebutuhan dan aturan produk.
- [PRD frontend](../PRD_FE_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md) — antarmuka; mengikuti kebijakan PRD utama.
- [Roadmap](../ROADMAP_IMPLEMENTASI_SIM_GURU_REVISI_PEMBAGIAN_TUGAS.md) — urutan implementasi; bukan bukti fitur selesai.

Keputusan bertanda FINAL adalah keputusan terdokumentasi; OPEN belum diputuskan; RANCANGAN adalah rancangan teknis. Status di bawah merangkum jalur implementasi utama dalam repository, bukan bukti bahwa seluruh tes atau UAT telah lulus.

- **IMPLEMENTED:** fondasi aplikasi, login/sesi dan administrasi akses; master siswa/guru/mapel dan pegawai non-guru beserta impor Excel siswa/guru; kalender, kelas, penempatan, transisi/perpindahan siswa; wali/pengampu dan jadwal; absensi harian siswa; serta alur absensi per pelajaran dan kehadiran guru GPS/selfie, pengelolaan laporan, mesin status/rekap guru, dan Instant Search sebagaimana tercatat pada HANDOVER DEV-03G1–G3-UX-01. Status implementasi tidak dengan sendirinya membuktikan kesiapan operasional di semua sekolah/perangkat.
- **PARTIAL:** Fase 8 dan operasional akademik secara keseluruhan. Rekap guru berbasis jadwal tersedia dan UAT Product Owner DEV-03G3/UX-01 tercatat PASS. Rekap belum mencakup sumber tugas wajib nonmengajar yang belum tersedia; persentase final belum ditetapkan. Cakupan UAT sekolah/perangkat dan pengujian otomatis tetap mengikuti bukti masing-masing di HANDOVER dan TESTING.
- **PLANNED:** jurnal mengajar, penilaian, dan rekap akademik yang belum dibangun; lihat kebutuhan otoritatif dan roadmap.
- **UNKNOWN:** kondisi deployment di luar repository serta hasil UAT/penerimaan operasional sekolah; dokumentasi kode tidak membuktikan hal tersebut.

Penyelarasan kewenangan dan Ubuntu 26.04 disetujui pengguna pada 21 Oktober 2026; lihat [keputusan](DECISIONS.md). Kelas/penempatan awal milik Admin; persiapan kalender, mapel, wali/pengampu, jadwal, transisi dan pemeriksaan koreksi Final tetap Kurikulum. Penempatan normal Admin dan transisi akhir tahun Kurikulum kini tersedia. Perpindahan kelas tengah tahun Kurikulum dan pindah sekolah keluar Admin tersedia sesuai D-031. Lihat D-015 dan D-030; jangan menyamakan kewenangan yang disetujui dengan fitur yang sudah tersedia.

Tambahan permintaan pengguna 21 Oktober 2026: impor Excel siswa/guru sebelum tahun/semester/kelas. Kebutuhan tambahan dan pilihan implementasinya dicatat pada D-013 di [DECISIONS](DECISIONS.md); tiga dokumen kebutuhan asli tetap dipertahankan.

Tambahan disetujui 22 Oktober 2026: pembinaan Kesiswaan dan bimbingan BK sederhana masuk rencana v1 sesuai D-018. Cakupan baca catatan pembinaan Kesiswaan oleh Wali Kelas kemudian ditetapkan FINAL melalui D-042: penempatan dan penugasan aktif saat pembacaan, akses ditolak bila tidak jelas, serta snapshot kejadian tidak mewariskan akses historis. Ketentuan otoritatif ada di [PRD utama bab 3B](../PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#3b-kesiswaan-dan-bk--final-22-oktober-2026), [DECISIONS](DECISIONS.md), dan roadmap fase 8A. Ini kebutuhan yang disetujui untuk pengembangan berikutnya, bukan fitur yang sudah tersedia.

Panduan penugasan pengampu dan jadwal sederhana disetujui 22 Oktober 2026: [PRD utama bab 6](../PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#6-penugasan-dan-jadwal--final-inti), PRD frontend 3.4, roadmap fase 7 dan [D-020](DECISIONS.md#d-020--pengampu-per-semester-dan-jadwal-mingguan-sederhana-disetujui-22-oktober-2026). Implementasi tersedia di `internal/academic`, `internal/web`, template akademik, dan migration 00008. Pengampu bersama yang sebelumnya diizinkan kini ditunda di luar cakupan v1; pengampu tunggal tidak membatasi jumlah kelas/mapel yang dapat diajar seorang guru. Pengujian khusus alur ini masih perlu dibuat dan dijalankan.

Penyederhanaan pengisian jam/jadwal disetujui 22 Oktober 2026: template Senin–Sabtu/jam 1–12, simpan per hari, dan kelas–mapel dengan guru otomatis sesuai tanggal draf. Lihat PRD bab 6, PRD frontend 3.4 dan [D-021](DECISIONS.md#d-021--template-jam-harian-dan-pilihan-jadwal-mengikuti-penugasan-disetujui-22-oktober-2026). Impor aSc dilewatkan pada D-021; kemudian diterapkan sesuai D-029 setelah sampel XML diperiksa. Data lama tetap dipertahankan.

Rancangan lanjutan 22 Oktober 2026: [PRD bab 6.1](../PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#61-rancangan-pengembangan-jadwal-fleksibel-dan-generator) dan [D-024](DECISIONS.md#d-024--rancangan-jadwal-fleksibel-per-rombel-dan-generator-22-oktober-2026) menjadi panduan JP/pertemuan fleksibel dengan pengecualian rombel, istirahat eksplisit, Upacara pada jam ke-1, dan generate → draf → tinjau/edit → publikasi. Inti kebutuhan ini kini diterapkan sesuai persetujuan lanjutan D-025, dilengkapi batasan guru/mapel fase 7B; lihat HANDOVER untuk bukti verifikasi dan status data development. Rincian antarmuka pada PRD frontend 3.4; urutan pekerjaan pada fase 7A roadmap.

Rancangan berikutnya yang disetujui untuk pencatatan: [PRD bab 6.2](../PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#62-rancangan-batasan-jadwal-dan-preferensi) dan [D-026](DECISIONS.md#d-026--batasan-wajib-dan-preferensi-jadwal-22-oktober-2026), batas guru/mapel berdasarkan waktu aktual per hari dengan jenis Tidak tersedia atau Usahakan dihindari, kapasitas per guru dan laporan preferensi. Preferensi pagi diusahakan pada sesedikit mungkin hari tanpa mengurangi JP atau melanggar syarat wajib. **Diterapkan sesuai persetujuan lanjutan D-027/migration 00011**; lihat FE 3.4, roadmap fase 7B, dan HANDOVER. Penyempurnaan ini tidak mengubah status generator inti D-025 yang sudah tersedia.


Editor Batasan Jadwal diperbarui sesuai D-028: [PRD bab 6.3](../PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#63-kisi-ketersediaan-guru-dan-mapel--diterapkan-22-oktober-2026), kisi hari × nomor jam untuk Guru/Mapel, status Tersedia/Kondisional/Tidak tersedia. Kisi menjadi aturan aktif setelah ditinjau/disimpan, dengan catatan waktu lama dan jadwal terbit tetap tersimpan. Migration 00012; rincian FE 3.4 dan verifikasi HANDOVER.


Impor XML aSc disetujui dan diterapkan pada 23 Oktober 2026, D-029/migration 00013. [PRD bab 6.4](../PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md#64-impor-jadwal-xml-asc--23-oktober-2026) mengatur pemetaan per unggahan, pratinjau, blok terpotong, draf belum lengkap, waktu sekolah dan pelestarian data. Ini menggantikan penundaan impor aSc sebelumnya, tanpa mengganti generator/manual yang sudah tersedia. Verifikasi aktual pada HANDOVER.

Perpindahan siswa D-031 (23 Oktober 2026): PRD bab 5 mengatur kewenangan, tanggal berlaku WIB, histori dan pengaruh ke roster/transisi. Implementasi migration 00015; verifikasi aktual pada HANDOVER.

Penerimaan kembali siswa lama oleh Admin disetujui dan diterapkan sesuai D-032/migration 00016: PRD bab 5 mengatur identitas tetap, kelas tahun aktif, siklus keluar–kembali berulang, jeda dan status bertanggal. Bukti verifikasi terbaru pada HANDOVER.

Fase 8 diperbarui melalui D-033 (24 Oktober 2026): guru GPS/selfie, harian siswa kartu QR, dan pelajaran terpisah. Ketentuan foto 100 KB/dua bulan dan kartu hilang ada di PRD utama bab 7.1–7.5. Pengguna mengotorisasi dokumentasi lengkap serta penerapan urutan pertama (kalender/tanggal penempatan/akses), bukan seluruh modul sekaligus.

D-039 (28 Oktober 2026) memperbarui tata kelola kehadiran guru: role Pegawai TU dan TU Pengelola Kehadiran Guru memisahkan kewenangan; Admin mengelola role; Petugas Absensi Siswa tidak mendapat akses; guru hanya mengirim permintaan koreksi tanpa mengubah catatan. Kontrak status, laporan Izin/Sakit/Dinas Luar, dan kewenangan tercatat di [DECISIONS](DECISIONS.md#d-039--tata-kelola-kehadiran-guru-oleh-tu-28-oktober-2026) serta PRD utama §7.2. DEV-03G2 menambahkan laporan Izin/Sakit/Dinas Luar rentang tanggal, koreksi/pembatalan TU dan histori. DEV-03G3 dan UX-01 telah melewati UAT Product Owner dan final gate publikasi menurut HANDOVER; rekap tetap terbatas pada sumber jadwal yang tersedia. Master pegawai non-guru tersedia melalui DEV-03G4-A. Kontrak kehadiran mandiri TU mengikuti [D-041](DECISIONS.md#d-041--kehadiran-mandiri-pegawai-tu-29-oktober-2026) dan PRD utama §7.7; implementasi, hasil tes, migration, serta kesiapan UAT aktual dicatat di HANDOVER. Tugas nonmengajar belum terintegrasi karena belum ada sumber penugasan.

D-034 melengkapi fondasi Fase 8 dengan tanggal penempatan massal Admin: lihat pelengkap pada PRD utama, FE dan DECISIONS. Bukan implementasi modul pencatatan absensi.

Fase 8 urutan 2 (D-035): harian siswa, kartu QR/cetak/ganti, manual, koreksi dan histori tersedia. Acuan PRD utama 7.3/pelengkap D-035, FE, DECISIONS dan HANDOVER; urutan 3–5 belum dianggap selesai.

D-036 (24 Oktober 2026) mengganti penundaan keterlambatan dan jam Hadir opsional: Kesiswaan mengatur batas/toleransi bertanggal/rombel, penilaian tersimpan sebagai snapshot, Petugas memakai jam Hadir wajib dan Isi cepat per kelas. Lihat PRD utama 7.6, FE dan DECISIONS; status verifikasi/pemasangan pada HANDOVER.
