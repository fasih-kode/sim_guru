# Peta aturan bisnis

Semua rujukan berikut berasal dari [PRD utama](../PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md). Ini indeks, bukan salinan aturan otoritatif. Implementasi akun/role, master siswa/guru/mapel, kalender akademik, kelas, dan pembatasan Admin/Kurikulum memiliki tes. Penempatan normal Admin tersedia tetapi belum dicakup tes otomatis; transfer/transisi dan operasi akademik lanjut belum diimplementasikan.

| Sumber | Ketentuan terdokumentasi |
|---|---|
| Bab 2 | Satu SMP; laporan bukan rapor resmi |
| Bab 3–3A | Admin: identitas/akun/role/kelas/penempatan awal; Kurikulum: kalender/penugasan/transisi/pemeriksaan koreksi Final; akses berdasarkan role dan cakupan |
| Bab 4 | ID internal permanen, keunikan identitas sesuai ketentuan, histori dipertahankan |
| Bab 5 | Tahun/semester aktif tunggal; penempatan dan transisi historis/atomik; keputusan Kepala Sekolah untuk perpindahan tengah tahun |
| Bab 6 | Jadwal terpisah dari jurnal; tidak ada Guru Pengganti/Cadangan; perubahan pengampu resmi bertanggal efektif |
| Bab 7–8 | Absensi terpisah; kosong bukan Alpa/Hadir; jurnal Draf/Final; koreksi beralasan dan historis |
| Bab 9–11 | Rumus harus disetujui; koreksi nilai Final diperiksa Kurikulum; laporan dan audit dibatasi akses |

## OPEN
Bab 14 adalah daftar otoritatif P0/P1. Termasuk formula/remedial/pembulatan, pengampu bersama, koreksi langsung oleh Kurikulum, QR/fallback, anomali, penutupan semester, penempatan khusus, katalog/cakupan laporan. Jangan menganggapnya disepakati atau menghalangi fondasi yang independen.

## Bukti implementasi akses
internal/auth dan internal/web menerapkan multi-role Admin, perubahan role langsung pada sesi aktif, serta penolakan akses administrasi bagi non-Admin. /akademik hanya terbuka untuk Kurikulum dan belum memiliki operasi penugasan. Tes master data kini membuktikan Kurikulum tidak dapat mengubah identitas siswa/guru, role Guru tidak dapat mengakses daftar identitas tanpa cakupan, dan Admin tanpa Kurikulum tidak dapat mengelola mapel. Cakupan akses Guru/Wali berdasarkan penugasan kelas belum diimplementasikan.

## Bukti implementasi master data
- NIS/NISN boleh kosong, nilai yang diisi unik termasuk pada siswa nonaktif; koreksi nama/nomor/status mempertahankan ID dan histori.
- Admin mengelola guru dengan NIP/NUPTK/kontak opsional. Tautan satu akun-satu guru opsional tidak menyamakan status guru dengan status akun.
- Kurikulum mengelola kode/nama/status mapel; kode historis tetap dimiliki mapel yang sama. Rename tidak menghapus histori.
- Guru nonaktif tidak boleh menerima penugasan baru menurut PRD, tetapi tes penugasan belum tersedia karena modulnya belum dibuat. Jangan mengklaim aturan ini sudah ditegakkan pada modul yang belum ada.

## Bukti implementasi tahun ajaran, semester, dan kelas
- Tahun ajaran yang dibuat menyediakan tepat Semester 1 dan Semester 2. Maksimum satu tahun ajaran dan satu semester aktif; pergantian tahun mengosongkan semester aktif lama, dan semester di tahun nonaktif tidak dapat diaktifkan.
- Kelas memiliki ID tetap, tingkat SMP 7–9, dan nama unik tanpa membedakan kapitalisasi dalam tahun+tingkat. Koreksi nama/tingkat memerlukan alasan dan optimistic version; histori menyimpan perubahan serta actor. Tidak tersedia hapus atau pindah tahun.
- Kurikulum menyiapkan tahun beserta dua semester dan mengaktifkan kalender. Admin membuat/mengoreksi kelas; Kurikulum hanya membaca kelas/historinya. Admin dapat membaca pilihan tahun untuk kelas, tanpa hak membuat atau mengaktifkan kalender. Akun multi-role mendapat gabungan hak eksplisit. Jumlah penempatan pada kelas tampil pada katalog. Kelas tetap belum siap operasional sampai wali kelas tersedia.
- Penempatan awal siswa oleh Admin sudah tersedia; wali kelas, pengampu, jadwal, transisi kenaikan/kelulusan serta pemeriksaan koreksi nilai Final oleh Kurikulum telah disetujui, tetapi modulnya belum tersedia. Perpindahan tengah tahun tetap mengikuti keputusan khusus Kepala Sekolah dan pelaksanaan Kurikulum sesuai PRD; detail koreksi langsung nilai Final tetap OPEN. Lihat [D-015](DECISIONS.md#d-015--pemisahan-kelasadmin-dan-kalenderkurikulum-disetujui-21-oktober-2026).
- Penempatan normal satu per siswa/tahun oleh Admin; tahun kelas diturunkan dari kelas dan dikunci FK majemuk. Siswa yang sudah ditempatkan pada tahun tersebut tidak muncul sebagai calon kelas lain. Calon berstatus aktif maupun nonaktif ditampilkan; belum ada ketentuan bisnis untuk menolak siswa nonaktif. Penempatan mencatat actor/waktu dan histori atomik, tidak tersedia edit/hapus. Ini belum mengimplementasikan transfer atau transisi. Kurikulum dapat membaca daftar/histori.
- Tanggal kalender, penutupan tahun/semester dengan cek kewajiban, penempatan khusus, perpindahan dan transisi kenaikan/kelulusan belum tersedia dan tidak dapat disimpulkan dari status aktif.

## Tambahan kebutuhan impor — permintaan pengguna 21 Oktober 2026
Admin dapat mengimpor master siswa dan guru dari .xlsx sebelum fase tahun/semester/kelas. Aturan identitas, kepemilikan Admin, dan histori tetap berlaku. Impor menambah data setelah pratinjau/konfirmasi; tidak menimpa data atau menciptakan role/akun/penugasan. Potensi duplikat dihentikan untuk pemeriksaan manual, tanpa menyimpulkan NIP/NUPTK adalah unique key global. Pilihan teknis dan batasan ada di [DECISIONS](DECISIONS.md) dan [DEVELOPMENT](DEVELOPMENT.md).
