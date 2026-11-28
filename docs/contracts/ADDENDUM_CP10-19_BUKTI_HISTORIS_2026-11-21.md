# Addendum kebijakan CP10-19 — bukti TP historis

**Status keputusan bisnis: FINAL.** Disetujui Product Owner pada 21 November 2026 dan dicatat supervisor pada Draft PR #1. Addendum ini melengkapi Kontrak Bisnis Fase 10 Revisi 2 FINAL tanpa mengubah teks atau keputusan historis kontrak tersebut. Persetujuan ini tidak mengesahkan rancangan teknis atau implementasi.

## Keputusan Product Owner

**K-1 — Bukti historis minimum.** Penambahan TP historis setelah Final pertama wajib didukung bukti bahwa TP telah diajarkan dan dinilai. Bukti dapat berupa jurnal pembelajaran dan dokumen Sumatif yang sah, termasuk arsip manual, selama identitas kegiatan, tanggal, kelas, mata pelajaran, guru penanggung jawab, dan sumber hasil penilaian dapat ditelusuri. Pernyataan tanpa sumber pendukung tidak cukup. Tidak ada kewajiban approval digital berlapis yang ditambahkan.

**K-2 — Nilai historis.** Transkripsi nilai historis ke database berada di luar CP10-19. Hasil pada dokumen sumber tidak otomatis menjadi nilai yang tercatat di SIM-GURU, tidak disalin, disimpulkan, atau dikonversi menjadi nilai siswa tanpa proses pencatatan sah tersendiri.

**K-3 — Relasi kegiatan lama.** CP10-19 tidak boleh menghubungkan TP baru ke kegiatan lama atau mengubah catatan asli milik guru sebelumnya. Koreksi relasi kegiatan adalah pekerjaan terpisah yang menjaga BK-04, audit, dan pemeriksaan dampak.

**K-4 — Siswa terdampak.** Kelayakan dan kewajiban siswa ditentukan berdasarkan tanggal pelaksanaan penilaian sebenarnya dan riwayat penempatan efektif. Siswa yang belum menjadi anggota kelas pada tanggal kegiatan tidak otomatis wajib mengikutinya. Data atau bukti yang ambigu berstatus belum dapat diverifikasi, bukan nilai nol atau kewajiban otomatis.

## Rancangan teknis — usulan untuk review supervisor

**Status: usulan, belum disetujui dan belum diimplementasikan.** Mikro-checkpoint yang diusulkan hanya memvalidasi kelengkapan referensi dan menampilkan pratinjau dampak penambahan satu TP historis. Pratinjau bersifat baca-saja: tidak membuat TP, tautan kegiatan, nilai, Final, atau histori keputusan. Tidak ada konfirmasi penyimpanan TP pada cakupan ini. Penambahan TP historis baru dapat diaktifkan melalui checkpoint tersendiri setelah dependensi dan desainnya disahkan.

### Masukan dan sumber bukti

- Guru memilih konteks semester/kelas/mapel dan mengisi uraian TP serta alasan. Untuk tiap bukti, usulkan jenis sumber, identitas/nomor kegiatan atau dokumen, tanggal pelaksanaan sebenarnya, kelas, mapel, guru penanggung jawab, lokasi/identitas arsip, dan referensi asal hasil penilaian. Sertakan referensi pembelajaran dan penilaian yang bersama-sama menelusuri kedua unsur K-1. Lokasi arsip cukup berupa identitas yang dapat dicari petugas; unggahan berkas tidak diwajibkan. Jangan menaruh isi nilai siswa atau rahasia dalam URL maupun log.
- Referensi jurnal/kegiatan SIM-GURU yang dipilih melalui ID sah dapat diperiksa keberadaan dan kesesuaian konteks, tanggal, serta identitas sumber menurut data yang tersedia. Kecocokan metadata tidak otomatis membuktikan bahwa kegiatan atau nilai itu memang menilai TP yang belum memiliki identitas/relasi; hubungan substansi harus tetap dinyatakan belum dapat diverifikasi otomatis.
- Referensi arsip manual dapat diperiksa kelengkapan format dan keterlacakan yang dinyatakan pengguna, tetapi keberadaan, keaslian, isi, dan keterkaitannya dengan TP tidak dapat dipastikan otomatis. Tampilkan sebagai **perlu verifikasi manual**, bukan tervalidasi oleh sistem. Teks tanpa referensi sumber tidak memenuhi K-1. Kriteria operasional bukti cukup dan cara verifikasi manual membutuhkan keputusan supervisor/Product Owner sebelum penyimpanan diaktifkan; tidak mengusulkan approval berlapis.

### Siswa dan dampak

- Baca tanggal pelaksanaan dari sumber yang dapat dibuktikan atau dari referensi arsip yang dinyatakan, dengan asal tanggal terlihat jelas. Gunakan riwayat penempatan efektif pada tanggal tersebut untuk mengelompokkan kandidat: anggota kelas, bukan anggota kelas pada tanggal itu, dan **belum dapat diverifikasi** ketika tanggal/penempatan ambigu. Ini adalah pratinjau relevansi berdasarkan data tersedia, bukan penetapan kewajiban otomatis dari klaim bukti manual. Tampilkan dasar tiap kelompok dan keterbatasannya.
- Tampilkan jumlah siswa kandidat dan Final terbaru yang berpotensi terdampak secara terpisah; sertakan identitas/versi Final yang dapat dibaca aktor berwenang. Pratinjau memperlihatkan bahwa struktur bukti Final **dapat berubah dan memerlukan peninjauan bila koreksi sah nantinya disimpan**, bukan menulis penanda atau menyatakan seluruh siswa pasti terdampak. Final lama dan snapshot tetap berlaku.
- Tampilkan NH, bobot, rekomendasi dan status **saat ini** dengan label yang jelas bila tersedia. Tanpa relasi kegiatan–TP dan nilai siswa yang sah dalam aplikasi, rata-rata TP baru, NH sesudah perubahan, serta selisih rekomendasi numerik **tidak dapat dihitung**. Jangan menyajikan nol, perkiraan, atau hasil dokumen sebagai nilai aplikasi. Jika NH 0%, ketidaklengkapan NH tidak menjadi syarat Final normal menurut BT-05, tetapi perubahan struktur/snapshot dan kebutuhan peninjauan Final harus tetap dievaluasi terpisah. Jangan menyimpulkan bahwa rekomendasi numerik berubah hanya karena struktur berubah.

### Keamanan, konsistensi, dan penolakan

- Service baca-saja memakai transaksi snapshot konsisten dan pemeriksaan server-side role Guru Mapel, identitas aktif, penugasan resmi berlaku, serta konteks; sumber di luar cakupan ditolak tanpa membocorkan data. Perilaku pengampu lama/nonaktif tetap mengikuti BK-03–05 dan memerlukan desain terpisah bila tidak tercakup hak akses pratinjau yang disahkan.
- Form pratinjau dan pagination menggunakan POST resmi ber-CSRF tanpa mutasi; input panjang disimpan dalam form, bukan query URL. Validasi field dan sumber tidak lengkap menghasilkan penjelasan; bukti ambigu tampil belum dapat diverifikasi dan tidak boleh disulap menjadi lolos verifikasi. Untuk pratinjau lintas halaman, fingerprint mengikat konteks, input, penugasan, penempatan, sumber terkait, bobot dan Final terbaru dalam urutan deterministik; hitung ulang pada snapshot yang sama dan tolak token/versi kedaluwarsa dengan 409. Detail fingerprint dan batas ukuran harus direview sebelum kode dibuat.
- Tidak ada submission yang menciptakan data pada mikro-checkpoint baca-saja; idempotensi transaksi penambahan TP dan audit append-only menjadi kewajiban desain checkpoint penyimpanan terpisah. Untuk pratinjau, request berulang tidak menghasilkan mutasi. Jangan mengubah guard rutin atau Final existing.

### UAT dan pengujian yang diusulkan

1. **UAT A — bukti dan dampak:** guru sah memasukkan referensi jurnal dan Sumatif pada konteks sintetis yang sudah memiliki Final; layar membedakan kecocokan data aplikasi dari bukti manual, menampilkan kandidat siswa/Final, status struktural potensial, nilai saat ini, serta dampak numerik yang belum dapat dihitung. Tidak ada baris akademik atau Final baru.
2. **UAT B — tanggal dan ketidakpastian:** contoh siswa sebelum/sesudah tanggal pelaksanaan dan penempatan atau referensi ambigu ditampilkan terpisah; siswa yang datang kemudian tidak otomatis wajib, dan kasus ambigu belum dapat diverifikasi. Sertakan bobot NH 0% untuk membedakan syarat Final normal dari potensi tinjauan struktur.

Regression service/HTTP pada TEST perlu memeriksa 403 lintas penugasan/role dan CSRF, validasi sumber tidak lengkap, rujukan palsu/lintas konteks, tanggal/penempatan tidak pasti, pagination dan UTF-8 panjang tanpa data sensitif pada URL, snapshot basi 409, replay baca-saja tanpa mutasi, dan fingerprint sebelum/sesudah termasuk Final/snapshot. Bandingkan bobot NH 0% dan positif tanpa menghitung nilai yang tidak memiliki relasi sumber sah. Test plan ini belum dijalankan.

### Dependensi sebelum aktivasi penambahan

1. Keputusan bentuk dan verifikasi bukti manual, termasuk cara memeriksa unsur diajarkan dan dinilai tanpa menyamakan kelengkapan metadata dengan kebenaran substansi.
2. Desain koreksi struktur dan audit append-only untuk membuat TP setelah Final, termasuk fingerprint, idempotensi, transaksi dan mekanisme per siswa untuk menentukan Final yang benar-benar perlu ditinjau. Kebutuhan migration aditif belum dapat dipastikan sebelum desain penyimpanan disahkan; migration 00052 tidak diubah.
3. Desain terpisah untuk koreksi relasi kegiatan dan, jika diperlukan, pencatatan/transkripsi nilai historis yang sah, menjaga identitas sumber, siswa, tanggal, pengampu, BK-04 dan pencegahan duplikasi. Pembuatan kegiatan Sumatif historis, koreksi tanggal, dan penilaian susulan juga terpisah.
4. Keputusan perlakuan konteks dengan penempatan tak pasti dan kelayakan aktor historis sebelum operasi mutasi diaktifkan. Jangan menyimpan TP hanya berdasarkan asumsi bahwa masalah bukti akan diselesaikan nanti.

Rancangan ini diserahkan untuk review supervisor; bukan persetujuan implementasi, UAT, migration, atau perubahan kebijakan lain.
