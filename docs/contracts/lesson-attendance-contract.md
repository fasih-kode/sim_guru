# Kontrak Absensi Pelajaran — D-033, D-037, dan D-050

Status: aturan operasional existing D-033/D-037 tetap berlaku; D-050 FINAL menambahkan prasyarat presensi guru untuk mutasi oleh guru. Kontrak ini tidak memperluas hak akses Kurikulum atau role lain.

## Gate presensi guru

Guru hanya dapat mencatat atau mengoreksi Absensi Pelajaran untuk occurrence yang memang dapat diaksesnya apabila bukti presensi guru untuk tanggal occurrence WIB memenuhi D-050:

- Pada tanggal WIB hari ini, Check-In yang sah cukup bila tidak ada bukti atau laporan yang bertentangan.
- Pada tanggal lampau, Check-In dan Check-Out harus ada, sah, serta konsisten.
- Tanpa Check-In, bukti yang hilang/tidak jelas/bertentangan, laporan Izin atau Sakit aktif, dan Dinas Luar aktif (diblokir secara default) menolak mutasi.
- Check-Out yang mendahului waktu selesai occurrence menurut snapshot JP resmi menolak mutasi occurrence tersebut. Tidak ada ambang keterlambatan tambahan.
- Koreksi sumber presensi/laporan resmi yang membuat bukti memenuhi syarat membuka akses kembali secara otomatis.

Pemeriksaan dijalankan saat form/konteks dibuka dan diulang dalam transaksi backend yang sama dengan mutasi. Request HTTP langsung tidak dapat melewati pemeriksaan. Evaluator hanya membaca event presensi, histori, metadata sumber, laporan aktif, tanggal WIB, dan waktu resmi occurrence; status rekap harian saja bukan bukti.

## Batas akses dan histori

- Gate ini hanya menambah syarat untuk mutasi Guru Mapel yang sudah berhak menurut assignment, jadwal terbit, revision efektif, roster, snapshot, dan D-037. Role gabungan tidak memperluas cakupan.
- Hak koreksi administratif Kurikulum yang sudah ada menurut D-037 dipertahankan tanpa perubahan. D-050 tidak memberi Admin hak baru.
- Jika bukti berubah menjadi tidak memenuhi syarat setelah catatan tersimpan, catatan siswa, status, snapshot, versi, dan histori tidak dihapus atau diubah otomatis. Gate hanya menahan mutasi selanjutnya oleh guru.
- Hak baca historis tetap menurut kebijakan masing-masing domain; kegagalan gate tidak mengubah akses baca atau mengungkap informasi presensi kepada pihak yang tidak berwenang.
- Tidak ada pencatatan Absensi Pelajaran otomatis dari presensi guru, status jurnal, maupun QR absensi harian siswa. Gate bukan kesimpulan pelanggaran.

## Perilaku antarmuka

Jika tidak memenuhi syarat, halaman/form menampilkan alasan singkat dan langkah yang sesuai, misalnya meminta pemeriksaan/koreksi kepada petugas berwenang. Isian yang telah dimasukkan dipertahankan. Backend mengulang pemeriksaan pada saat simpan dan menolak tanpa mutasi parsial jika bukti berubah setelah form dibuka.

## Bukti verifikasi

Kasus minimum: Check-In hari ini; Check-In hari ini tidak ada; tanggal lampau lengkap/tidak lengkap; bukti tidak sah/bertentangan; laporan Izin/Sakit/Dinas Luar; Check-In terlambat yang tetap sah; Check-Out sebelum/tepat sesudah akhir JP; koreksi yang membuka atau mencabut kelayakan; akses guru lain/role gabungan; request langsung; race antara koreksi presensi dan simpan; serta pelestarian catatan dan histori lama.
