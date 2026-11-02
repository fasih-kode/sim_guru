# KONTRAK BISNIS FASE 10 — PENILAIAN HASIL BELAJAR
## SIM-GURU v1.0 | Revisi 2 — FINAL

**Tanggal:** 1 November 2026

**Status:** FINAL — DISAHKAN PENGGUNA, 1 November 2026.

**Lingkup:** Kebijakan bisnis dan acceptance criteria; **bukan** instruksi implementasi, migration, atau perubahan kode.

**Dasar:** Keputusan bisnis eksplisit dalam diskusi Fase 10, termasuk perluasan ASTS/ASAS.
**Aturan pengesahan:** Keputusan FINAL di dokumen ini tidak boleh diubah diam-diam oleh Codex.

---

## 1. Tujuan dan prinsip

Fase 10 mengelola Tujuan Pembelajaran (TP), Formatif, Sumatif Harian/TP, ASTS (Asesmen Sumatif Tengah Semester), ASAS (Asesmen Sumatif Akhir Semester), remedial, indikator ketercapaian, rekomendasi nilai akhir, finalisasi, koreksi, siswa pindahan, dan audit historis.

Prinsip tetap: akses menurut penugasan resmi; pekerjaan rutin tidak membutuhkan persetujuan berlapis; tidak ada mutasi diam-diam terhadap Final; nilai kosong berbeda dari nol; data historis tidak dihapus; perubahan sensitif memiliki alasan, audit, dan pemeriksaan dampak.

## 2. Otorisasi [FINAL]

- **BK-01** Admin menetapkan role dan penugasan guru sesuai keputusan sekolah; role Kurikulum mengelola kebijakan akademik dan bobot.
- **BK-02** Guru Mapel hanya mengelola kelas, mapel, dan semester sesuai penugasan sah. Pemeriksaan hak wajib berada di server, bukan hanya di UI.
- **BK-03** Guru pemilik nilai/Final boleh mengoreksi catatan historis miliknya bila masih aktif dan mempunyai hak akses terbatas yang sah, sekalipun tidak lagi mengampu kelas tersebut.
- **BK-04** Guru pengampu baru tidak boleh mengedit catatan asli milik guru sebelumnya; ia dapat mengakui bukti secara eksplisit dan menilai remedial baru sesuai kewenangannya.
- **BK-05** Jika guru pemilik sudah tidak aktif, Kurikulum boleh melakukan koreksi administratif pengecualian dengan alasan dan audit.
- **BK-06** Guru Mapel pengampu resmi kelas tujuan berwenang menetapkan Final terbaru siswa pindahan. Tidak ada kewajiban persetujuan Kurikulum pada koreksi Final rutin oleh guru pemilik.

## 3. TP dan pengelolaannya [FINAL]

- **TP-01** TP dikelola per kelas, mata pelajaran, dan semester.
- **TP-02** Guru dapat menyalin **definisi TP saja** ke kelas lain yang sah diampunya. Setelah salin, TP independen; kegiatan/nilai/histori tidak disalin; batas ketercapaian memakai konfigurasi kelas tujuan, bukan pengecualian kelas asal.
- **TP-03** TP tanpa nilai boleh diedit rutin selama tidak melewati penguncian struktur. Setelah berisi nilai, perbaikan **redaksi** wajib beralasan dan diaudit.
- **TP-04** Perubahan **substansi** TP yang sudah digunakan adalah koreksi struktur khusus, bukan edit biasa. TP yang sudah memiliki histori tidak boleh dihapus permanen.
- **TP-05** Sesudah siswa pertama menjadi Final pada kombinasi kelas/mapel/semester, penambahan TP rutin dikunci. TP lama yang sudah diajarkan dan dinilai namun terlupa dicatat hanya dapat ditambahkan melalui koreksi struktur khusus, beralasan dan dengan pemeriksaan dampak.
- **TP-06** Koreksi struktur tidak otomatis mengubah Final. Final terdampak ditandai **Perlu Ditinjau**, diproses ulang secara eksplisit per siswa atau massal hanya untuk siswa yang memenuhi syarat normal. Histori Final lama tetap.

## 4. Kegiatan Formatif dan Sumatif Harian/TP [FINAL]

- **PN-01** Bentuk kegiatan fleksibel (tugas, praktik, proyek, tes, hafalan, dan lain-lain). Setiap kegiatan wajib berjenis **Formatif** atau **Sumatif**.
- **PN-02** Satu kegiatan boleh menilai beberapa TP, tetapi nilai siswa dicatat **terpisah untuk setiap TP**.
- **PN-03** Formatif untuk pemantauan dan umpan balik; tidak dihitung dalam rekomendasi nilai semester dan tidak memengaruhi empat status penilaian akhir.
- **PN-04** Setiap kegiatan Sumatif yang berlaku bagi siswa wajib dilengkapi nilai pada TP yang dinilainya; tidak ada Sumatif opsional/pengayaan sebagai cara menghindari kelengkapan.
- **PN-05** Kegiatan Sumatif dikunci dari penambahan rutin setelah Final pertama. Kegiatan yang telah benar-benar berlangsung tetapi terlupa dicatat hanya dapat dimasukkan lewat koreksi historis beralasan dengan tinjauan dampak.
- **PN-06** Kewajiban Sumatif kelas tujuan untuk siswa pindahan ditentukan oleh **tanggal pelaksanaan nyata** kegiatan dan **tanggal efektif** penempatannya; kegiatan sebelum tanggal mulai penempatan tidak otomatis wajib.

## 5. Skala, nilai efektif, dan perhitungan Harian/TP [FINAL]

- **NL-01** Formatif, Sumatif Harian/TP, ASTS, ASAS, dan remedial menggunakan nilai **0 s.d. 100**, maksimal **dua angka desimal**; nilai 0 sah, kosong bukan 0.
- **NL-02** Nilai efektif satu kegiatan Sumatif adalah maksimum dari nilai asli dan seluruh remedial sah yang berlaku. Remedial tidak menambah jumlah kegiatan/bobot.
- **NL-03** Nilai TP = rata-rata aritmetika **tanpa bobot** atas nilai efektif semua kegiatan Sumatif yang relevan untuk TP tersebut.
- **NL-04** Nilai Harian/TP (NH) = rata-rata aritmetika **tanpa bobot** atas seluruh TP yang berlaku. TP memiliki bobot setara, terlepas dari jumlah kegiatan Sumatif pada tiap TP.
- **NL-05** Tidak ada pembulatan pada perhitungan antara. Rata-rata TP ditampilkan dua desimal. Perbandingan ketercapaian menggunakan nilai TP **sebelum pembulatan**.

## 6. ASTS, ASAS, remedial, dan histori [FINAL]

- **EX-01** ASTS dan ASAS masing-masing menggunakan **satu sumber nilai awal yang berlaku** per siswa, mata pelajaran, dan semester untuk perhitungan komponen, tanpa komponen ujian internal di SIM-GURU. Dalam kasus perpindahan, bila terdapat beberapa hasil ASTS/ASAS yang sah dari kelas berbeda, seluruh bukti asal dipertahankan; guru tujuan memilih **satu sumber aktif secara eksplisit** (lihat EX-08).
- **EX-02** ASTS dan ASAS boleh memiliki **beberapa remedial**; nilai efektif komponen = nilai tertinggi antara nilai awal dan seluruh remedial sah.
- **EX-03** Nilai awal, semua remedial, tanggal, pelaksana dan riwayat tidak boleh terhapus/tertindih. Koreksi kesalahan input dibedakan dari hasil remedial.
- **EX-04** Remedial atau koreksi ASTS/ASAS setelah Final tidak otomatis mengubah Final; memerlukan peninjauan dan konfirmasi ulang dengan versi histori.
- **EX-05** ASTS/ASAS dari kelas asal siswa pindahan ditampilkan kepada guru tujuan dan **hanya digunakan setelah pengakuan eksplisit**. Sumber asli, upaya remedial, dan asal kepemilikan tetap utuh. Bukti yang sama tidak boleh dihitung ganda.
- **EX-06** Koreksi sumber yang sudah diakui tidak mengubah pengakuan secara otomatis: guru tujuan mendapat penanda **Perlu Ditinjau**, lalu mengambil keputusan eksplisit. Guru tujuan boleh melaksanakan remedial sendiri tanpa mengedit nilai asli guru asal.
- **EX-07** Jika siswa pindah berulang atau kembali ke kelas asal, asal-usul nilai tetap dapat ditelusuri; hasil remedial dari kelas lain diakui ulang secara eksplisit oleh guru kini.
- **EX-08** Bila siswa memiliki **dua atau lebih hasil ASTS atau ASAS dari kelas berbeda** dalam mata pelajaran dan semester sama, guru tujuan **memilih secara eksplisit satu sumber nilai yang berlaku** untuk komponen tersebut, beserta alasan dan audit. Hasil lain tetap menjadi histori; tidak dirata-ratakan dan tidak otomatis diambil yang tertinggi.
- **EX-09** Aturan **nilai tertinggi** hanya berlaku antara nilai awal dan **remedial sah dalam rangkaian sumber yang dipilih**. Perpindahan pilihan sumber setelah Final memerlukan peninjauan dan konfirmasi Final eksplisit; versi lama tetap.

## 7. Bobot Kurikulum dan nilai rekomendasi [FINAL]

- **BT-01** Kurikulum menetapkan satu konfigurasi bobot yang **seragam untuk seluruh sekolah per tahun pelajaran dan semester** untuk komponen: Harian/TP (NH), ASTS (NT), ASAS (NS).
- **BT-02** Guru Mapel tidak dapat mengubah bobot. Jumlah ketiga bobot harus **tepat 100%**. Angka bobot spesifik tidak ditetapkan di dokumen ini; 60:20:20 hanyalah ilustrasi.
- **BT-03** Nilai rekomendasi semester, bila seluruh komponen berbobot positif lengkap:

  `Rekomendasi = round_half_up(NH × BH + NT × BT + NS × BS)`

  Bobot berupa proporsi (contoh 20% = 0,20), dan `round_half_up` berarti dibulatkan ke bilangan bulat terdekat, pecahan tepat 0,50 ke atas. Perhitungan antara tidak dibulatkan.
- **BT-04** Syarat rekomendasi: seluruh kegiatan Sumatif Harian/TP yang wajib **untuk komponen dengan bobot positif** lengkap, dan setiap TP yang berlaku memiliki sedikitnya satu bukti sah apabila NH berbobot positif; ASTS dan ASAS wajib memiliki nilai sah jika bobotnya masing-masing **lebih dari 0%**.
- **BT-05** Nilai komponen kosong **bukan nol**, tidak boleh diimputasi 0 atau mengalihkan/menormalisasi bobot ke komponen lain. **Komponen berbobot 0% tidak masuk rekomendasi, tidak menentukan status penilaian akhir, dan tidak menjadi syarat kelengkapan Final normal.**
- **BT-05a** Nilai, remedial, koreksi, dan histori komponen berbobot 0% **tetap boleh dicatat** dalam administrasi pembelajaran. Bobot 0% bukan perintah menghapus kegiatan asesmen, bukan pula larangan bagi guru melaksanakannya.
- **BT-05b** Komponen berbobot 0% **tidak dapat menjadi satu-satunya bukti** Finalisasi Pengecualian. Bila Kurikulum menaikkan bobotnya dari 0% menjadi positif, sistem mengevaluasi ulang kelengkapan dan rekomendasi non-Final; hasil Final lama hanya diberi penanda dampak, tidak otomatis berubah.
- **BT-06** Kurikulum boleh **mengoreksi bobot setelah ada Final** dengan alasan, audit, serta pemeriksaan dampak seluruh sekolah. Rekomendasi yang belum Final dihitung ulang; Final lama tidak berubah otomatis, bobot snapshot lama tetap; Final terdampak diberi penanda **Perlu Ditinjau** dan guru melakukan konfirmasi ulang bertahap.

> **Penegasan keputusan:** Jika NH berbobot 0%, ketidaklengkapan TP/Sumatif Harian tidak menghalangi rekomendasi dan Final normal. Pencatatan nilai maupun kegiatan pembelajaran tetap tersedia, tetapi tidak menjadi syarat kelengkapan nilai akhir untuk semester dengan konfigurasi tersebut.

## 8. Indikator ketercapaian TP [FINAL]

- **KT-01** Kurikulum menetapkan pedoman umum. Guru Mapel menetapkan satu batas ketercapaian bawaan per kelas/mapel/semester dan dapat memberi pengecualian per TP.
- **KT-02** Indikator dihitung dari **rata-rata TP sebelum pembulatan**. Tidak mewajibkan remedial dan bukan penghalang Final otomatis.
- **KT-03** Perubahan kriteria yang belum Final memperbarui indikator; perubahan diaudit. Versi Final menyimpan snapshot kriteria dan indikator semula dan tidak ditimpa.

## 9. Status, Final normal, dan pengecualian [FINAL]

Empat status utama: **Belum Dinilai**, **Belum Lengkap**, **Siap Final**, **Final**.

- **FN-01** Belum Dinilai: belum ada satu pun nilai Sumatif sah dari komponen berbobot positif (termasuk NH/ASTS/ASAS).
- **FN-02** Belum Lengkap: sudah ada setidaknya satu nilai Sumatif sah dari komponen berbobot positif, tetapi syarat rekomendasi belum lengkap.
- **FN-03** Siap Final: semua nilai wajib dari komponen berbobot positif lengkap sehingga tersedia rekomendasi.
- **FN-04** Final: nilai akhir secara eksplisit ditetapkan guru; boleh berpenanda **Dengan Pengecualian** dan/atau **Perlu Ditinjau** (keduanya bukan status kelima).
- **FN-05** Final normal dapat individual atau massal bagi siswa yang memenuhi seluruh syarat; guru dapat berbeda dari rekomendasi dengan alasan wajib.
- **FN-06** Finalisasi pengecualian **hanya individual**, dengan **nilai akhir manual, alasan wajib**, dan **sedikitnya satu nilai Sumatif sah** pada NH, ASTS, atau ASAS **yang berbobot positif**. Tidak ada rekomendasi semu ketika komponen wajib belum lengkap. Tidak boleh Final bila semua Sumatif sah dari komponen berbobot positif kosong; hanya Formatif tidak cukup.
- **FN-07** Final harus dapat dipertahankan versi per versi berikut nilai, alasan, pelaksana, waktu, struktur/kriteria/bobot yang menjadi dasarnya.

## 10. Koreksi dan penguncian [FINAL]

- **KR-01** Guru pemilik Final dapat langsung mengoreksi Final historis yang sah tanpa persetujuan Kurikulum selama masih aktif. Kurikulum hanya untuk koreksi administratif pengecualian ketika pemilik tidak aktif.
- **KR-02** Koreksi nilai Sumatif/ASTS/ASAS dapat mengubah perhitungan aktif dan rekomendasi belum Final, tetapi **tidak** memperbarui Final otomatis. Guru harus mengonfirmasi secara eksplisit Final baru atau mempertahankan nilai lama dengan alasan.
- **KR-03** Koreksi struktur TP/Sumatif yang telah digunakan harus memiliki alasan, peninjauan dampak, histori, serta konfirmasi ulang Final **bertahap**. Final yang belum ditinjau tetap berlaku.
- **KR-04** Koreksi tanggal kegiatan Sumatif setelah memiliki nilai atau setelah penguncian struktur: alasan, audit, dan pemeriksaan dampak. Tanggal riil pelaksanaan berbeda dari tanggal input.
- **KR-05** Nilai yang menjadi **tidak berlaku** bagi siswa setelah koreksi tanggal **dikeluarkan dari perhitungan**, tetapi disimpan sebagai histori. Penilaian susulan sah harus dicatat dengan fakta tanggal pelaksanaannya.
- **KR-06** Sistem harus mencegah pembaruan Final berdasar data yang sudah berubah di sela peninjauan dan konfirmasi (validasi ulang sebelum simpan).
- **KR-07** **Penilaian susulan** dilaksanakan untuk siswa yang **belum mengikuti** kegiatan; dicatat pada **kegiatan Sumatif yang sama**, dengan tanggal pelaksanaan nyata **per siswa**. Tanggal utama kegiatan kelas tidak diubah, dan satu kegiatan hanya dihitung satu kali untuk TP yang dinilai.
- **KR-08** Susulan berbeda dari **remedial**: susulan menghasilkan nilai awal kegiatan yang belum diikuti, sedangkan remedial adalah upaya setelah sudah ada nilai awal. Susulan juga berbeda dari koreksi administratif atas input salah.
- **KR-09** Bagi siswa pindahan yang masuk **setelah** tanggal kegiatan utama, susulan tidak otomatis diwajibkan atau diikutsertakan. Guru tujuan **menetapkan keikutsertaan susulan secara eksplisit**; nilai susulan yang sah dihitung sekali meskipun kegiatan utama sebelum tanggal efektif penempatan.
- **KR-10** Tanggal pelaksanaan individual dan fakta penugasan susulan harus dapat ditelusuri. Susulan atau pembetulan historis yang memengaruhi Final/penguncian mengikuti koreksi khusus, audit, dan peninjauan Final; tidak boleh mengubah Final diam-diam.
- **KR-11** Prinsip satu slot, nilai awal, tanggal pelaksanaan nyata, dan histori susulan **berlaku juga untuk ASTS/ASAS**; tidak menciptakan komponen ujian tambahan.

## 11. Perpindahan siswa [FINAL]

- **PS-01** Guru tujuan meninjau secara eksplisit kesetaraan substantif TP asal dan TP tujuan; satu TP asal dipetakan ke satu TP tujuan untuk satu keputusan pengakuan.
- **PS-02** Bukti Sumatif individual yang diakui digabung dengan Sumatif tujuan **tanpa bobot berbeda**. Rata-rata TP asal tidak dianggap satu skor gabungan. Satu bukti tidak boleh terhitung dua kali dalam TP yang sama.
- **PS-03** Penilaian tujuan yang terlaksana sebelum tanggal efektif masuk kelas tujuan tidak otomatis wajib.
- **PS-04** Koreksi nilai sumber tidak otomatis memutakhirkan pengakuan; guru tujuan mendapat penanda **Perlu Ditinjau** dan memutuskan tindak lanjut eksplisit.
- **PS-05** Guru tujuan boleh memberi remedial baru terhadap Sumatif yang diakui; hasil tersimpan di bawah pelaksana baru, terhubung ke bukti asli, bukan mengubah nilai guru asal.
- **PS-06** Jika pindah berulang, semua bukti dapat ditelusuri hingga sumber asli tanpa penggandaan. Ketika kembali ke kelas asal, guru asal secara eksplisit meninjau remedial yang dibuat di kelas lain.
- **PS-07** Jika siswa sudah memiliki Final di kelas asal, Final tetap berlaku sampai guru kelas tujuan menetapkan Final terbaru. Final terdahulu menjadi histori, bukan dihapus.
- **PS-08** ASTS/ASAS asal juga memerlukan pengakuan eksplisit guru tujuan (aturan EX-05), tidak boleh terhitung ganda.
- **PS-09** Jika siswa benar-benar mengikuti ASTS/ASAS di lebih dari satu kelas selama semester sama, guru tujuan menetapkan **satu sumber aktif** (EX-08); bukti lain tetap menjadi histori. Tidak ada nilai tertinggi otomatis antardua ujian yang berbeda.
- **PS-10** Susulan kegiatan yang berlangsung sebelum siswa pindah hanya dapat dimasukkan berdasarkan **penugasan susulan eksplisit** dan tanggal pelaksanaan per siswa (KR-07 sampai KR-10).

## 12. Matriks validasi/acceptance criteria

| ID | Perilaku yang wajib diterima |
|---|---|
| AC-01 | Nilai 0 dan 100 valid; 82,50 valid; nilai kosong berbeda dari nol; -1/100,01/82,567 ditolak. |
| AC-02 | Formatif saja tidak mengubah status akhir. |
| AC-03 | Satu kegiatan menilai multi-TP dengan nilai per TP per siswa. |
| AC-04 | Rata-rata kegiatan Sumatif dalam TP tanpa bobot; rata-rata seluruh TP tanpa bobot. |
| AC-05 | Remedial berulang memakai maksimum, tidak menambah bobot atau menimpa sumber. |
| AC-06 | ASTS dan ASAS masing-masing satu nilai awal + histori remedial, satu nilai efektif per komponen. |
| AC-07 | Hanya Kurikulum mengatur bobot; total persis 100%; satu konfigurasi untuk seluruh sekolah per semester. |
| AC-08 | Hanya komponen berbobot positif yang wajib lengkap untuk rekomendasi; tidak ada substitusi nol atau normalisasi bobot. |
| AC-09 | Rekomendasi akhir menggunakan bobot efektif dan round-half-up pada akhir saja. |
| AC-10 | Finalisasi pengecualian hanya individual, minimal satu Sumatif sah berbobot positif, manual + alasan. |
| AC-11 | Nilai Formatif dan komponen berbobot 0% bukan dasar Finalisasi Pengecualian. |
| AC-12 | Final lama tidak berubah akibat remedial, koreksi nilai/tanggal, pengakuan, perubahan TP, atau pembobotan. |
| AC-13 | Versi Final dan snapshot struktur/bobot/kriteria bisa diaudit. |
| AC-14 | Penambahan TP dan kegiatan Sumatif rutin ditolak setelah Final pertama; koreksi historis memiliki jalur khusus. |
| AC-15 | Koreksi struktur menampilkan dampak dan mendukung konfirmasi Final bertahap. |
| AC-16 | Penyalinan TP hanya definisi; TP tujuan independen dan memakai batas tujuan. |
| AC-17 | Penugasan resmi ditegakkan pada server dan histori guru lain dilindungi. |
| AC-18 | Pengakuan Sumatif, ASTS, dan ASAS siswa pindahan selalu eksplisit. |
| AC-19 | Perpindahan ganda tidak menggandakan sumber nilai; tanggal efektif menentukan kewajiban Sumatif. |
| AC-20 | Koreksi sumber tidak mengubah pengakuan otomatis. |
| AC-21 | Koreksi tanggal dapat membuat nilai tidak berlaku, historinya tetap dan tidak dihitung. |
| AC-22 | Konfigurasi bobot yang dikoreksi setelah Final memperbarui rekomendasi non-Final, menandai Final terdampak, mempertahankan versi lama. |
| AC-23 | Sistem menolak konfirmasi Final berbasis hasil peninjauan yang sudah kedaluwarsa akibat perubahan data. |
| AC-24 | Siswa yang memiliki dua hasil ASTS/ASAS dari kelas berbeda hanya menggunakan satu sumber pilihan guru tujuan; hasil lain tetap histori; nilai tertinggi otomatis hanya berlaku untuk remedial sumber terpilih. |
| AC-25 | Susulan satu kegiatan dicatat dengan tanggal pelaksanaan individual; kegiatan asal tidak digandakan dan bobot tidak bertambah. |
| AC-26 | Penugasan susulan untuk siswa yang baru pindah sesudah kegiatan utama harus eksplisit, bukan wajib otomatis. |
| AC-27 | NH/ASTS/ASAS berbobot 0% tidak ikut rekomendasi, status kelengkapan atau syarat Final normal, tetapi nilainya dan remedial tetap dapat dicatat. |
| AC-28 | Nilai sah hanya pada komponen 0% tidak mencukupi Finalisasi Pengecualian; perubahan bobot 0% ke positif mengevaluasi kelengkapan aktif, Final tetap historis. |
| AC-29 | Konfigurasi bobot wajib total tepat 100%; komponen 0% diizinkan, dan komponen positif lain wajib sesuai kelengkapan. |

## 13. Rancangan skenario UAT pengguna (belum dieksekusi)

| ID | Skenario | Hasil yang diharapkan |
|---|---|---|
| UAT-01 | Input 0, 82,50, 100, 100,01 | Tiga pertama diterima; terakhir ditolak. |
| UAT-02 | Hanya nilai Formatif | Belum Dinilai. |
| UAT-03 | Satu kegiatan Sumatif menilai dua TP | Nilai TP per siswa independen. |
| UAT-04 | TP memiliki nilai 80,100,60 | Rata-rata TP = 80,00. |
| UAT-05 | Dua TP 80 dan 90; TP lain kosong | Tidak ada rekomendasi otomatis bila NH berbobot positif. |
| UAT-06 | Tes awal 60, remedial 75,85 | Nilai efektif 85, bukti awal tersimpan. |
| UAT-07 | TP rerata 74,995 dan batas 75 | Bandingkan tanpa membulatkan 74,995 terlebih dulu. |
| UAT-08 | Rekomendasi raw 82,50 | Menjadi 83. |
| UAT-09 | ASTS awal 65, remedial 75,85 | ASTS efektif 85, histori tiga nilai. |
| UAT-10 | ASAS awal 80, remedial 70 | ASAS efektif 80. |
| UAT-11 | Guru mencoba mengubah bobot | Akses ditolak. |
| UAT-12 | Kurikulum mengatur total bobot 110% | Ditolak. |
| UAT-13 | Bobot 60:20:20, NH 80, ASTS 70, ASAS 90 | Rekomendasi 80. |
| UAT-14 | Bobot ASAS positif tetapi ASAS kosong | Belum Lengkap, tanpa rekomendasi. |
| UAT-15 | Hanya ASTS sah, bobot positif | Pengecualian individual dengan manual+alasan diperbolehkan. |
| UAT-16 | Semua nilai Sumatif kosong | Pengecualian ditolak. |
| UAT-17 | Hanya nilai komponen berbobot 0% | Pengecualian ditolak. |
| UAT-18 | Final dibuat lalu bobot berubah 60:20:20 menjadi 50:20:30 | Rekomendasi aktif diperbarui, Final lama tetap, Perlu Ditinjau. |
| UAT-19 | Guru mengonfirmasi Final terbaru | Versi baru menjadi aktif; versi lama tetap audit. |
| UAT-20 | Tambah TP rutin setelah Final pertama | Ditolak; koreksi khusus tetap tersedia untuk bukti historis yang sah. |
| UAT-21 | Koreksi struktur memengaruhi 25 Final | Bisa konfirmasi satu per satu; lainnya tetap Final lama. |
| UAT-22 | Salin TP dari 7A ke 7B | Hanya definisi; kriteria 7B dipakai. |
| UAT-23 | Siswa pindah tanggal 15/9, Sumatif tujuan tanggal 10/9 | Tidak otomatis wajib. |
| UAT-24 | 7A→7B→7C, nilai asal pernah diakui di 7B | Di 7C sumber asli dihitung sekali saja. |
| UAT-25 | Nilai asal 80 dikoreksi 90 | Pengakuan tujuan tetap 80 hingga konfirmasi guru tujuan. |
| UAT-26 | Siswa kembali ke 7A setelah remedial di 7B | Perlu pengakuan eksplisit hasil remedial lintas kelas. |
| UAT-27 | ASTS 7A = 82, siswa pindah 7B | Menjadi komponen 7B hanya sesudah guru 7B mengakui. |
| UAT-28 | ASAS yang telah diakui diremedial di kelas tujuan | Remedial baru tercatat tanpa mengubah skor asal. |
| UAT-29 | Koreksi tanggal 20/9 menjadi 10/9 ketika siswa masuk 15/9 | Nilai siswa yang tidak berlaku tak ikut hitung, histori dan Final tetap. |
| UAT-30 | Guru tidak berwenang mengirim perubahan langsung ke endpoint | Ditolak pada server tanpa perubahan data. |
| UAT-31 | Data berubah sesudah guru membuka pratinjau Final | Konfirmasi lama ditolak atau diwajibkan memuat ulang pratinjau. |
| UAT-32 | ASTS 7A=82 dan ASTS 7B=90, guru 7B memilih sumber 7A | Nilai efektif ASTS=82 bila tidak ada remedial; bukti 90 tetap histori, tidak otomatis dipilih. |
| UAT-33 | ASTS sumber terpilih 82 mempunyai remedial sah 95 | Nilai efektif ASTS=95, bukan maksimum dua ujian berbeda tanpa pengakuan. |
| UAT-34 | Tes kelas 7B tanggal 10/9; Ahmad pindah 15/9; guru menetapkan susulan Ahmad 20/9 nilai 85 | Tercatat satu kegiatan, tanggal individual 20/9; nilai dihitung sekali setelah penugasan eksplisit. |
| UAT-35 | Siswa kelas aktif tidak hadir tes dan mengikuti susulan | Tetap satu kegiatan, nilai awal dari susulan, tidak diklasifikasikan remedial. |
| UAT-36 | Bobot NH 70%, ASTS 0%, ASAS 30%; nilai NH/ASAS lengkap, ASTS kosong | Rekomendasi tersedia dan status Siap Final; tidak ada pemaksaan nilai ASTS. |
| UAT-37 | Bobot NH 0%, ASTS 40%, ASAS 60%; NH/TP kosong, ASTS/ASAS lengkap | Rekomendasi tersedia dan status Siap Final; NH/TP masih boleh dicatat terpisah. |
| UAT-38 | Bobot ASTS 0%, nilai dan remedial ASTS dicatat | Histori tersimpan, tetapi nilai tidak memengaruhi rekomendasi atau status. |
| UAT-39 | Hanya ASTS yang bernilai sah namun bobot ASTS 0% | Tidak boleh Finalisasi Pengecualian tanpa bukti Sumatif berbobot positif. |
| UAT-40 | Kurikulum mengubah bobot ASTS dari 0% menjadi positif, nilai ASTS siswa kosong | Siswa yang belum Final menjadi Belum Lengkap; Final lama tidak berubah otomatis. |

## 14. Rekap penyelesaian isu dan batas desain teknis

Isu bisnis pada draf sebelumnya telah diputuskan secara eksplisit:

| Isu draf awal | Keputusan FINAL | Rujukan |
|---|---|---|
| Dua hasil ASTS/ASAS dari kelas berbeda | Guru tujuan memilih satu sumber aktif secara eksplisit; hasil lainnya histori; aturan nilai tertinggi hanya untuk remedial sah dari sumber terpilih | EX-08/EX-09; PS-09 |
| Susulan pada tanggal berbeda | Tetap satu kegiatan, ada tanggal nyata per siswa; siswa pindahan yang tidak otomatis wajib ditugaskan susulan secara eksplisit | KR-07 s.d. KR-11 |
| Komponen berbobot 0% termasuk NH | Tidak masuk rekomendasi, tidak menghambat Final normal, tidak menjadi bukti minimum pengecualian; nilai dan remedial tetap dapat dicatat | BT-05 s.d. BT-05b |

**Tidak ada isu bisnis OPEN yang sengaja ditinggalkan dari daftar sebelumnya.** Dokumen ini **masih memerlukan pengesahan pengguna sebagai satu naskah** sebelum dijadikan sumber instruksi implementasi. Jika saat desain teknis ditemukan kebutuhan untuk menambah atau mengubah kewajiban akademik, jangan putuskan sendiri: kembalikan sebagai satu keputusan bisnis terfokus.

Detail teknis yang **bukan perubahan kebijakan** dan wajib diuji: cara menyimpan tanggal pelaksanaan per siswa dan identitas sumber penilaian, validasi penugasan resmi di server, mekanisme atomik perubahan Final, pemeriksaan dampak Kurikulum lintas kelas, penanda Perlu Ditinjau, dan pencegahan konfirmasi berbasis data kedaluwarsa.

---

## 15. Gerbang pekerjaan dan larangan implementasi prematur

1. Minta pengesahan pengguna atas naskah kontrak bisnis Revisi 2 yang telah dikonsolidasikan.
2. Baru susun prompt Codex lengkap berdasarkan dokumen yang sudah disahkan.
3. Codex melakukan audit repositori dan implementasi **lokal**, dengan pengujian sesuai acceptance criteria.
4. Pengguna melakukan UAT langsung dan memberikan keputusan penerimaan.
5. Setelah lulus gerbang akhir, baru commit dan push sesuai alur proyek.

**Dokumen ini adalah artefak perumusan kontrak bisnis dalam percakapan; belum ada instruksi mengubah kode, skema database, migration, atau repository.**
