# Kontrak Bisnis Jurnal Mengajar — Fase 9A

Status dokumen: kontrak bisnis FINAL D-048 dan D-050, keputusan arsitektur FINAL D-049, serta kontrak teknis Fase 9B. Catatan implementasi dan penyempurnaan UAT terbaru berada di HANDOVER.

## 1. Landasan dan status aturan

### FINAL dari PRD

- Jadwal adalah rencana; jurnal mencatat kegiatan pada occurrence jadwal resmi. Perubahan jadwal tidak menulis ulang jurnal masa lalu.
- Jurnal memuat konteks kelas, mata pelajaran, blok JP resmi, guru pengampu resmi yang berwenang, waktu resmi per JP, dan materi.
- Guru yang berwenang dapat menyimpan Draf dan memfinalisasi sendiri. Jurnal Final tidak diedit diam-diam; koreksi wajib beralasan dan menyimpan riwayat versi. Tidak ada inbox approval atau persetujuan rutin Kepala Sekolah.
- Tidak ada Guru Pengganti/Cadangan, penugasan sementara, atau jurnal atas nama orang lain. Ketidakhadiran guru tidak membuat jurnal otomatis.
- D-050 mensyaratkan bukti presensi guru yang memenuhi syarat sebelum guru memutasi Jurnal Mengajar. Gate ini adalah prasyarat operasional, bukan indikator pelanggaran dan bukan perubahan status jurnal.
- Guru di luar penugasan resmi ditolak. Pertemuan yang tidak terlaksana tidak otomatis memiliki jurnal Final.
- Materi pembelajaran maksimal 500 karakter wajib untuk Final pembelajaran terlaksana. Catatan kegiatan opsional maksimal 2.000 karakter. Draf boleh belum lengkap.
- Penanda **Tidak Terlaksana** memerlukan alasan dan tidak boleh menyatakan pembelajaran terlaksana. Ketiadaan presensi guru tidak mengisi penanda itu.
- Seluruh nomor JP dan waktu yang digunakan jurnal berasal dari snapshot jadwal resmi yang efektif pada tanggal occurrence WIB. Guru tidak menginput atau mengoreksi waktu pelaksanaan. Revisi jadwal tidak menulis ulang snapshot pada versi jurnal yang sudah ada. Revisi FINAL D-048 tanggal 1 November 2026 menggantikan ketentuan sebelumnya yang memperbolehkan input waktu aktual terpisah.
- Draf terdampak revisi ditandai **Perlu Ditinjau** dan tetap pada occurrence lama; Final mempertahankan snapshot historis.
- Satu occurrence memiliki satu jurnal lintas revisi; pemetaan ambigu tidak diterka. Guru lama yang akunnya dan role yang diperlukan masih aktif dapat membaca/mengoreksi jurnal historis miliknya; guru baru tidak mewarisi hak mutasi.
- Jurnal Saya mendukung ekspor PDF dan XLSX seluruh hasil filter. Draf diberi label. Ekspor agregat role lain tidak diberikan otomatis.

Dasar: PRD utama §6 dan §8; PRD Frontend §3.4 dan §3.6; D-020, D-033, D-048; roadmap Fase 9.

### Rekomendasi teknis — belum menjadi kebijakan FINAL

- Buat satu jurnal untuk satu pertemuan terjadwal pada tanggal WIB tertentu; konteks jadwal dan penugasan diambil server, bukan dipercaya dari form.
- Simpan identitas/konteks historis sebagai snapshot bersama referensi sumber, agar koreksi data master atau jadwal tidak mengubah jurnal lama.
- Jurnal Final bersifat append-only secara logis: koreksi membuat versi baru beralasan dan mempertahankan versi sebelumnya.
- Gunakan kontrol versi optimistis dan idempotency key untuk mencegah simpan ganda serta menolak form usang.
- Batasi v1 pada pertemuan dari jadwal resmi yang telah dipublikasikan dan berlaku pada tanggal pertemuan.

### Cutover FINAL D-049

- Jangan backfill otomatis lineage jadwal historis. Pada satu cutover bertanggal WIB yang tercatat, beri ID logical baru kepada setiap slot pada revision published yang efektif untuk tiap semester dalam cakupan. Cutover harus memilih revision efektif secara deterministik dalam transaksi dan tidak boleh memilih dua revision untuk semester/tanggal yang sama. Entry dari revision lama yang tidak aktif tetap tanpa logical identity; jangan membuat jurnal occurrence yang bergantung padanya seolah lineage telah diketahui.
- Revisi setelah cutover mempunyai record lineage per slot. Record dapat berstatus `pending` sementara keputusan belum diketahui, atau final `continue` (menghubungkan satu entry sumber dengan satu entry tujuan dan mempertahankan ID logical), `new` (menerbitkan ID baru), atau `stop` (mengakhiri ID sumber untuk occurrence setelah tanggal efektif). Pending tidak memenuhi syarat operasi jurnal dan harus diselesaikan eksplisit oleh Kurikulum. Penghentian tidak menghapus identitas atau jurnal lama. Satu logical ID hanya boleh muncul sekali pada satu revision. ID berstatus dihentikan tidak pernah dapat dipakai kembali.
- Kurikulum yang menentukan keputusan. Kemiripan kelas, mapel, guru, hari, waktu, JP, atau assignment bukan bukti. Split: satu cabang yang dipilih eksplisit boleh meneruskan ID; cabang lain mendapat ID baru. Merge: Kurikulum memilih satu lineage untuk diteruskan dan menghentikan yang lain, atau membuat ID baru; jangan gabungkan beberapa ID lama.
- Revisi jadwal, absensi, dan presensi tetap berjalan sesuai semantik existing meskipun lineage belum diputuskan. Entry yang menunggu resolusi tidak tersedia untuk operasi jurnal sampai Kurikulum menyimpan keputusan; penolakan jurnal tidak memblokir operasi domain lain.
- Gunakan daftar entry dan revisi sumber/tujuan yang terikat FK, keputusan/status, pelaku/waktu, serta alasan. Constraint unik harus mencegah satu entry sumber bercabang sebagai beberapa keputusan `continue`, satu entry tujuan menerima beberapa lineage, dan satu logical ID dipakai dua kali pada revision yang sama. Riwayat resolusi pending dipertahankan. Resolusi lineage atomik; publikasi/operasi jadwal tetap tidak bergantung padanya. Revisi historis tetap utuh.
- Jurnal baru untuk tanggal sebelum cutover tidak diperbolehkan tanpa mekanisme pemetaan historis yang diputuskan terpisah. Pengisian terlambat untuk occurrence sejak tanggal cutover tetap diperbolehkan, mengikuti syarat isi/status dan akses kontrak ini.
- Untuk occurrence, sistem memakai tanggal kalender WIB dan revision published yang efektif pada tanggal tersebut; jangan mengganti sumber dengan revision terbaru hanya karena revision tersebut aktif sekarang. Snapshot setiap versi jurnal tetap memuat sumber dan seluruh nomor/waktu JP yang digunakan saat versi disimpan.

## 2. Alur Guru Mapel

1. Guru membuka Jadwal Saya, memilih tanggal/pertemuan dari jadwal terbit, lalu memilih **Isi Jurnal**.
2. Server memverifikasi akun aktif, role Guru Mapel, identitas guru aktif yang tertaut, penugasan resmi pada tanggal pertemuan, semester/kalender, serta versi jadwal yang efektif.
3. Form menampilkan sebagai konteks baca-saja: tahun ajaran, semester, tanggal WIB, kelas, mapel, guru pengampu resmi, serta nomor dan waktu resmi setiap JP.
4. Guru mengisi kegiatan aktual dan dapat menyimpan Draf atau memfinalisasi sesuai kontrak final D-048.
5. Jurnal Draf dapat dilanjutkan oleh pelaku yang sah. Jurnal Final tampil di riwayat; koreksi dilakukan melalui **Koreksi dengan Alasan** dan tidak memerlukan persetujuan Kepala Sekolah.
6. Jika data jadwal berubah setelah form dibuka, sistem meminta tinjau ulang. Sistem tidak memindahkan isi ke pertemuan baru secara diam-diam.

Tidak ada POST atau finalisasi otomatis dari membuka link, mengisi absensi siswa, maupun mencatat presensi guru.

## 3. Sumber data dan isian aktual

| Data | Sumber yang disarankan | Perlakuan |
|---|---|---|
| ID jurnal, pelaku, waktu simpan/final/koreksi, versi | Database dan akun terautentikasi | Dibuat server; tidak dapat dipalsukan dari klien |
| Tahun ajaran, semester, tanggal WIB | Semester/kalender resmi dan tanggal pertemuan yang dipilih | Konteks terikat pada tanggal; aturan fallback tanggal sekolah ikut kalender |
| Kelas, mapel, guru, penugasan | Jadwal terbit efektif serta penugasan resmi pada tanggal itu | Ditampilkan baca-saja dan disimpan sebagai referensi + snapshot |
| Nomor JP dan waktu JP | Revision jadwal resmi efektif pada tanggal occurrence WIB | Prefill baca-saja; seluruh nomor/waktu JP disimpan dalam snapshot setiap versi |
| Materi | Isian guru | Maksimal 500 karakter; wajib sebelum Final jika pembelajaran terlaksana |
| Catatan kegiatan | Isian guru | Opsional, maksimal 2.000 karakter |
| Tidak Terlaksana | Penanda eksplisit guru | Alasan wajib; tidak dibuat otomatis oleh absensi/presensi |
| Status jurnal | Transisi server | Draf boleh belum lengkap; Final divalidasi menurut outcome terlaksana/tidak terlaksana |
| Alasan koreksi Final | Isian korektor | Wajib menurut PRD; panjang dan format batas teknis perlu diselaraskan dengan konvensi proyek |
| Versi terdahulu dan histori | Versi/event immutable | Menyimpan pelaku, waktu server WIB, alasan, dan before/after |

Kelas, mapel, guru, dan jadwal yang sudah diketahui tidak diketik ulang atau dapat dipilih bebas oleh guru.

## 4. Status dan perubahan

Transisi FINAL: `Draf → Draf` (melanjutkan), `Draf → Final` (finalisasi mandiri), dan `Final → Final versi baru` melalui koreksi beralasan. Koreksi Final dapat memperbaiki outcome dari Terlaksana menjadi Tidak Terlaksana atau sebaliknya; status jurnal tetap Final dan alasan koreksi wajib. Tidak ada penghapusan atau penimpaan versi terdahulu. Draf boleh tidak memiliki materi/catatan. Jika ditandai Tidak Terlaksana, alasan wajib saat penanda disimpan. Materi dan catatan kegiatan tetap opsional; pada outcome Tidak Terlaksana, materi hanya konteks/rencana dan tidak boleh ditafsirkan sebagai bukti materi diajarkan. Final Terlaksana memerlukan materi. Tidak ada input waktu pelaksanaan aktual untuk kedua outcome.

Status hanya menyatakan keadaan jurnal, bukan bukti kehadiran guru atau siswa. “Belum ada jurnal Final” tidak sama dengan “tidak mengajar”, “Alpa”, atau pelanggaran.

## 5. Hak akses

- Guru Mapel hanya membuat, membaca, melanjutkan, dan memfinalisasi jurnal untuk penugasan resmi yang berlaku pada tanggal pertemuan.
- Role gabungan tidak memperluas cakupan kelas/mapel. Role saja tanpa penugasan resmi tidak cukup.
- Identitas guru, akun, role, jadwal terbit, kalender, dan assignment diperiksa ulang di server pada setiap baca/mutasi.
- Guru tidak dapat mengubah konteks historis kelas/mapel/tanggal/guru melalui payload.
- Tidak ada hak untuk Guru Pengganti/Cadangan atau akses sementara karena guru tidak hadir.
- Kepala Sekolah bukan approver rutin. Hak baca detail jurnal Kepala Sekolah, Kurikulum, Wali Kelas, dan role lain belum sepenuhnya dirinci untuk jurnal; tidak boleh disimpulkan hanya dari hak laporan umum atau role.

### Akses historis FINAL D-048

Guru historis yang menjadi pemilik jurnal dapat membaca dan mengoreksi jurnalnya selama akun aktif dan role Guru Mapel yang diperlukan aktif. Pemeriksaan menggunakan identitas/pemilik historis, bukan assignment aktif saat ini. Guru pengampu baru tidak mewarisi hak atas jurnal tersebut. D-048 tidak memberi Kurikulum fallback koreksi. Tolak akses jika hubungan pemilik tidak dapat diverifikasi.

## 6. Absensi, presensi, dan anomali

- Jurnal tidak dibuat/final otomatis dari presensi guru, QR/absensi harian, atau absensi per pelajaran.
- Absensi pelajaran tetap catatan per siswa/per pertemuan; jurnal adalah catatan kegiatan mengajar. Kedua domain memiliki status dan histori sendiri.
- D-050 adalah gate mutasi. Hanya Guru Mapel yang berwenang pada occurrence dapat menyimpan/memfinalisasi/mengoreksi jurnal bila bukti presensi guru untuk tanggal WIB memenuhi syarat. Penugasan, revision efektif, JP/snapshot, lineage, dan otorisasi tetap diperiksa terpisah.
- Hari ini, Check-In yang sah cukup jika tidak ada laporan aktif atau bukti yang bertentangan. Tanggal lampau mensyaratkan Check-In dan Check-Out yang sah serta konsisten. Tidak ada Check-In, bukti hilang/tidak jelas/bertentangan, laporan Izin/Sakit aktif, atau Dinas Luar aktif (diblokir secara default) menahan mutasi. Check-Out sebelum waktu selesai occurrence berdasarkan snapshot juga menahan occurrence tersebut. Tidak ada ambang keterlambatan baru.
- Evaluasi yang sama ditampilkan ketika form dibuka dan dijalankan ulang di transaksi penyimpanan. Request HTTP langsung tidak dapat melewati gate. Pesan blokir harus menyebut langkah koreksi yang sesuai secara netral dan mempertahankan isian yang belum disimpan.
- Setelah koreksi resmi presensi/laporan memenuhi syarat, akses terbuka kembali otomatis. Jika bukti kemudian tidak lagi memenuhi syarat, jurnal, status, seluruh versi, histori dan snapshot tetap utuh; hanya mutasi baru yang diblokir. Hak baca tidak berubah.
- D-050 bukan indikator Perlu Ditinjau dan tidak menyimpulkan pelanggaran. Indikator Perlu Ditinjau yang tetap berlaku di sini adalah indikator Draf akibat revisi jadwal/lineage menurut D-048/D-049. Tidak ada indikator tambahan presensi/absensi tanpa keputusan produk baru.

### Implementasi indikator revisi jadwal (9B-04A)

Penanda yang diwajibkan D-048 untuk Draf dihitung saat dibaca, bukan disimpan sebagai status bisnis. Untuk tanggal occurrence, pembacaan memilih revision published yang efektif pada tanggal WIB tersebut, lalu memakai keputusan lineage D-049 untuk mencari slot logical yang sama. Penanda muncul bila revision efektif yang berbeda masih `pending`, slot logical tidak lagi tersedia, atau konteks slot efektif (kelas, mapel, guru/penugasan, hari, nomor JP, atau waktu JP) berbeda dari snapshot versi Draf. Jika revision baru hanya meneruskan slot dengan konteks yang sama, indikator tidak muncul.

Final tidak diberi penanda ini dan tidak berubah akibat revision; versi historis tetap membaca snapshot tersimpan. Outcome Tidak Terlaksana sendiri bukan pemicu. Jika tidak ada revision efektif yang dapat dibandingkan, indikator tidak menyimpulkan perubahan. Pemeriksaan hanya membaca jurnal milik Guru Mapel yang terautentikasi; lineage pending diberi alasan netral tanpa membuka data jurnal orang lain. Perhitungan tidak mengubah jurnal, jadwal, absensi, presensi, atau histori.

Pembacaan penanda tidak menggantikan validasi mutasi: layanan simpan mengunci operasi jurnal dan menolak pembuatan atau perubahan occurrence ketika revision published yang efektif masih berstatus lineage pending. Penolakan tidak menambah versi atau histori; setelah keputusan lineage diselesaikan, operasi dapat dicoba kembali dengan pemeriksaan otorisasi dan versi yang sama seperti alur jurnal lainnya. Ini menerapkan batas operasi D-049, bukan status atau workflow bisnis baru.

### Batas indikator lintas sumber

D-050 menggantikan usulan lama untuk membandingkan presensi sebagai indikator: bukti presensi adalah syarat mutasi Jurnal dan Absensi Pelajaran sesuai aturan eksplisit di atas. Gate tidak mengubah atau memberi kesimpulan terhadap catatan absensi siswa. Indikator Draf yang tersisa hanya perubahan revision/lineage/konteks jadwal D-048/D-049. Tidak ada perbandingan waktu aktual guru karena input waktu aktual sudah dihapus oleh revisi D-048. Ketiadaan jurnal atau absensi siswa bukan bukti ketidakhadiran atau pelanggaran.

## 7. Kasus khusus

| Kasus | Perilaku sesuai D-048 / implementasi yang direkomendasikan |
|---|---|
| Sesuai jadwal | Jurnal terikat pada occurrence tanggal dan entri jadwal terbit efektif; konteks prefilled |
| Simpan Draf dan lanjutkan | Pertahankan nilai, pelaku/waktu update, dan versi; hanya pihak sah membuka |
| Finalisasi | Validasi ulang penugasan/konteks dan versi; simpan final serta histori atomik |
| Koreksi Final | Alasan wajib, expected-version, transaksi atomik, versi terdahulu tetap terlihat |
| Pertemuan tidak terlaksana | Penanda eksplisit dengan alasan; Final harus beralasan dan tidak menyatakan pembelajaran berlangsung. Tidak dibuat otomatis |
| Jadwal terbit direvisi | Final tetap pada snapshot lama. Draf terdampak ditandai Perlu Ditinjau, tidak dipindah atau ditulis ulang |
| Pengampu berganti efektif | Otorisasi untuk occurrence mengikuti assignment efektif pada tanggal WIB; snapshot mempertahankan pengampu ketika dibuat |
| Guru membuka pertemuan orang lain | Tolak di service dan route; jangan mengandalkan filter UI |
| Kirim ganda | Idempotensi mengembalikan hasil yang sama atau menunjukkan konflik; tidak membuat jurnal/versi ganda |
| Versi usang | HTTP konflik yang aman, tampilkan versi terbaru dan minta pengguna tinjau ulang; jangan auto-submit |
| Tidak cocok dengan absensi/presensi | Tampilkan selisih netral hanya sesuai hak akses sumber; sumber tidak berubah |
| Presensi belum memenuhi D-050 | Form menunjukkan alasan spesifik; mutasi ditolak tanpa versi/histori parsial, lalu dapat dicoba kembali setelah sumber resmi dikoreksi |

## 8. Status pembuktian teknis Fase 9B-01

Keputusan bisnis inti ditetapkan D-048/D-049. Status berikut adalah pekerjaan teknis, bukan OPEN bisnis:

1. **Cutover, bukan backfill — diterapkan dan diuji:** migration 00031–00034 menambah batch/lineage tanpa menyentuh jadwal lama. Entry historis pra-cutover tetap tanpa logical ID; occurrence pra-cutover ditolak. Uji cutover/retry dan serialisasi dengan publish PASS.
2. **Resolusi lineage — diterapkan; tes split/merge dan stop/reuse masih kurang:** keputusan transition (`continue/new/stop`) menyimpan source/target, logical ID, aktor, waktu dan alasan. Konflik menolak operasi jurnal terkait tanpa memblokir publication jadwal dan domain absensi/presensi.
3. **Kunci unik dan konkurensi jurnal — diterapkan dan diuji:** unique `(logical_lesson_id, meeting_date)`, lock, idempotency key/hash dan expected-version menjaga occurrence dan versi dari duplikasi/overwrite stale.
4. **Snapshot JP — diterapkan dan diuji untuk snapshot per versi:** revision/entry serta nomor JP dan waktu tersimpan per versi. Tes perubahan jadwal/konfigurasi setelah simpan masih perlu ditambahkan.
5. **Deteksi Draf terdampak:** sudah tersedia sebagai evaluasi baca-saja sesuai D-048/D-049; Draf ditandai berdasarkan revision efektif dan lineage/konteks, tanpa mengubah status atau snapshot.
6. **Ekspor Jurnal Saya:** Fase 9B-04B FINAL dan UAT Product Owner PASS 6/6 pada 1 November 2026. PDF/XLSX menggunakan filter dan cakupan pemilik yang sama dengan daftar serta seluruh hasil di luar pagination; batas 1.000 baris dan 20 MiB diuji pada boundary, PDF Unicode/page break dan formula injection XLSX diuji. Bukti dan cakupan tes ada di HANDOVER; full integration lintas paket tidak diklaim pada final gate.
7. **Hak baca lain:** D-048 mengatur ekspor Jurnal Saya. Ekspor agregat dan akses detail raw oleh Kurikulum/Kepala Sekolah/role lain tetap memerlukan keputusan terpisah bila masuk scope; ini tidak menghalangi jalur Guru Mapel.

Tenggat dan notifikasi tetap di luar cakupan v1 sesuai roadmap.

## 9. Ekspor — FINAL D-048

Jurnal Saya menyediakan PDF untuk cetak/arsip dan XLSX untuk pengolahan. Filter awal: tahun ajaran, semester, tanggal, kelas, mapel, status. Ekspor menjalankan seluruh hasil filter yang diizinkan, bukan hanya halaman aktif. Batas ukuran boleh menolak/menawarkan ekspor bertahap, tetapi tidak boleh menghasilkan file parsial yang tampak lengkap.

PDF memuat nomor urut, tanggal WIB, kelas, mapel, nomor JP, materi, catatan, status, dan alasan Tidak Terlaksana. Teks panjang di-wrap dan page break mempertahankan header. XLSX memakai header jelas, satu baris per jurnal, tanggal WIB, nomor JP terurut, label Draf/Final dan penanda Tidak Terlaksana. Teks pengguna ditulis sebagai string, bukan formula; regression test mencakup nilai awal `=`, `+`, `-`, `@`, serta whitespace.

Untuk XLSX, implementasi menulis teks pengguna secara eksplisit dengan `SetCellStr`. PDF menggunakan `github.com/signintech/gopdf` dengan font DejaVu Sans lokal yang disertakan beserta lisensinya; dependency dan layout diuji pada implementasi Fase 9B-04B. Batas hasil/ukuran dan status pemeriksaan dicatat pada HANDOVER. Sumber: [Excelize](https://github.com/xuri/excelize), [gopdf](https://github.com/signintech/gopdf).

## 10. Acceptance criteria

- Guru aktif ber-role Guru Mapel dengan assignment resmi dan jadwal terbit yang berlaku dapat membuka konteks pertemuan miliknya; guru/role lain ditolak.
- Data kelas, mapel, guru, tanggal, semester, dan jadwal berasal dari sumber resmi dan tidak dapat diganti melalui request.
- Guru dapat menyimpan/melanjutkan Draf dan memfinalisasi sendiri tanpa approval rutin.
- Final terlaksana mensyaratkan materi maksimal 500 karakter; catatan opsional maksimal 2.000 karakter. Draf boleh belum lengkap.
- Final Tidak Terlaksana memerlukan alasan; materi/catatan opsional ditandai sebagai konteks dan bukan bukti pembelajaran berlangsung. Status tidak dipicu otomatis oleh presensi.
- Blok resmi menampilkan dan menyimpan urutan JP serta waktu tiap JP dari snapshot revision yang berlaku. Tidak ada input waktu aktual oleh guru; bukti waktu yang telanjur tersimpan pada versi lama tetap dipertahankan dan dibaca sebagai catatan historis.
- Jurnal Final tidak berubah kecuali melalui koreksi dengan alasan; versi, pelaku, waktu WIB, dan before/after dipertahankan.
- Simpan ganda tidak menggandakan jurnal atau histori; konflik versi tidak menimpa perubahan baru; satu slot+tanggal hanya mempunyai satu jurnal lintas revisi.
- Revisi jadwal/pengampu tidak mengubah snapshot jurnal terdahulu; Draf terdampak ditandai Perlu Ditinjau tanpa pemindahan otomatis.
- Guru historis yang akun dan role yang diperlukan aktif dapat membaca/mengoreksi jurnal miliknya; guru baru tidak mewarisi akses mutasi.
- Tidak ada jurnal atau absensi yang dihasilkan otomatis dari presensi/absensi lain.
- Mutasi guru pada Jurnal Mengajar mensyaratkan bukti presensi D-050 pada tanggal occurrence; GET dan pembacaan historis tidak menulis atau mengubah catatan.
- Jadwal tanpa jurnal Final tidak disimpulkan sebagai Alpa/pelanggaran.
- Akses historis dan role gabungan diuji sesuai keputusan produk yang telah ditetapkan.
- Daftar jurnal menggunakan Instant Search HTMX, batas role server, pagination yang konsisten, dan navigasi D-047; desktop serta ponsel dapat digunakan.
- PDF dan XLSX memuat semua hasil filter yang terotorisasi, memberi label Draf, memakai tanggal WIB, dan tidak mengeksekusi teks pengguna sebagai formula.
