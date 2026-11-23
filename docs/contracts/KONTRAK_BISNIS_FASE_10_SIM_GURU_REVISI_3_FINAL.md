# KONTRAK BISNIS FASE 10 SIM-GURU REVISI 3 — FINAL

**Status: FINAL — DISAHKAN PRODUCT OWNER, 23 November 2026.** Pengesahan ini bukan izin implementasi, migration, reset, konversi data, UAT atau deployment. Revisi 3 menggantikan Revisi 2 sebagai acuan implementasi; Revisi 2 tetap utuh sebagai arsip.

**Dasar:** [Revisi 2 FINAL](KONTRAK_BISNIS_FASE_10_SIM_GURU_v1_0_REVISI_2_UNTUK_PENGESAHAN.md), [Opsi B](https://github.com/fasih-kode/sim_guru/pull/1#issuecomment-7087116750), [S-01–S-06](https://github.com/fasih-kode/sim_guru/pull/1#issuecomment-7087419565), [keputusan PO satu model](https://github.com/fasih-kode/sim_guru/pull/1#issuecomment-7088563971), dan [review audit supervisor](https://github.com/fasih-kode/sim_guru/pull/1#issuecomment-7088636453). Keputusan terbaru menggantikan rancangan S-01/S-02 tentang pilihan dua model per konteks: produk akhir memiliki **satu mesin penilaian aktif**. S-03–S-06 tetap sejauh selaras dengan keputusan ini. CP10-01–23 tetap CLOSED sebagai milestone historis, Fase 10 belum CLOSED dan CP10-24 susulan ON HOLD.

## 1. Tujuan dan istilah

Guru memilih semester, kelas dan mata pelajaran yang diampu, membuka satu **Buku Nilai Guru** dengan tab **Nilai TP | ASTS | ASAS**, memasukkan nilai pada tabel siswa dan menekan **Simpan**. Audit rutin dicatat otomatis tanpa formulir tambahan. Sistem menghitung NH dan rekomendasi bila syarat terpenuhi.

TP adalah Tujuan Pembelajaran; NH adalah Nilai Harian rata-rata TP yang berlaku. ASTS dan ASAS merupakan komponen ujian; **ASAS bukan Nilai Akhir/Rapor**. Rekomendasi adalah hasil bobot sebelum ditetapkan; Final adalah nilai akhir yang ditetapkan secara eksplisit dan berversi.

Satu-satunya sumber NH aktif produk adalah **nilai langsung per siswa–TP**. Guru tidak perlu membuat kegiatan Sumatif untuk mencatat nilai TP dan tidak ada remedial harian/TP baru. Schema atau arsip historis yang dipertahankan demi retensi tidak menjadi pilihan model atau mesin NH kedua dalam produk aktif.

## 2. Kewenangan dan keamanan

**BK-01.** Admin mengelola role sesuai kewenangan yang disahkan. Kurikulum mengelola kebijakan akademik dan bobot; bobot berlaku seragam seluruh sekolah per semester.

**BK-02.** Guru Mapel mengelola konteks semester–kelas–mapel sesuai penugasan resmi dan identitas aktif. Server memeriksa role, status akun/guru, konteks dan hak pada setiap pembacaan serta perubahan; tampilan saja bukan pengaman.

**BK-03.** Guru pemilik nilai atau Final yang masih aktif dapat mengoreksi catatan historis miliknya dalam batas hak historis yang sah meskipun tidak lagi mengampu; hak ini tidak secara otomatis memperluas mutasi rutin. Hak khusus CP10-23 terbatas pada sumber yang telah ditetapkan dan tidak menjadi izin umum.

**BK-04.** Pengampu baru tidak mengubah catatan asli guru sebelumnya. Pengakuan bukti atau tindakan guru tujuan memerlukan kewenangan dan ketentuan yang disahkan; tidak ada pengakuan otomatis.

**BK-05.** Jika pemilik tidak aktif, Kurikulum dapat melakukan koreksi administratif pengecualian yang sah dengan alasan dan audit. **BK-06.** Guru pengampu resmi kelas tujuan berwenang menetapkan Final terbaru siswa pindahan; koreksi Final rutin pemilik sah tidak membutuhkan persetujuan Kurikulum berlapis.

**BK-07.** Perubahan wajib dilindungi CSRF, validasi server, transaksi atomik, pencegahan duplikasi dan penolakan konfirmasi berdasarkan data yang berubah. Pembacaan histori hanya untuk pihak yang berhak.

## 3. Tujuan Pembelajaran

**TP-01.** TP dikelola per kelas–mapel–semester. **TP-02.** Guru yang sah dapat menyalin definisi saja ke kelas lain yang diampu; TP tujuan independen, tanpa menyalin nilai atau histori dan memakai kriteria tujuan.

**TP-03.** TP tanpa nilai boleh dikoreksi rutin selama struktur belum terkunci; koreksi redaksi TP yang sudah bernilai memerlukan alasan dan audit. Keterkaitan kegiatan tanpa nilai tidak otomatis mengunci TP. **TP-04.** Perubahan substansi TP yang telah digunakan adalah koreksi struktur khusus; TP yang mempunyai histori tidak dihapus permanen.

**TP-05.** Setelah Final pertama dalam konteks, penambahan TP rutin terkunci. Koreksi struktur khusus untuk TP yang telah benar-benar diajarkan dan dinilai memerlukan bukti sah, alasan, pratinjau dan konfirmasi dampak sesuai keputusan yang berlaku; tidak mengarang atau menyalin nilai. **TP-06.** Koreksi struktur tidak mengubah Final otomatis; Final terdampak ditandai Perlu Ditinjau dan diproses eksplisit bertahap. Histori tetap.

**TP-07 / S-03.** Label `TP1`, `TP2`, dan seterusnya unik dan persisten per semester–kelas–mapel, menunjuk ID TP asli; label tidak digeser atau digunakan ulang setelah koreksi/arsip. Salinan mendapat nomor konteks tujuan. Uraian lengkap tersedia saat dibutuhkan. Penetapan awal kode untuk TP existing dilakukan deterministik dan tercatat, tanpa mengganti ID.

## 4. Nilai TP dan Formatif

**PN-01.** Setiap siswa memiliki satu nilai langsung per TP yang berlaku pada konteks. Input dapat dilakukan sebagian dan bertahap; sel kosong tidak membuat nilai atau nol. Audit mencatat identitas pencatat dan waktu secara otomatis.

**PN-02 / S-04.** Nilai awal asli tidak ditimpa. Sebelum siswa bersangkutan Final, guru yang sah dapat mengisi atau mengoreksi nilai pada TP yang sudah ada langsung melalui tabel; koreksi nilai tersimpan tidak memerlukan alasan tertulis dan sistem otomatis menyimpan before/after, aktor dan waktu dalam histori append-only. Setelah siswa bersangkutan Final, **pengisian nilai awal pada sel yang sebelumnya kosong maupun koreksi nilai tersimpan** memerlukan alasan, pratinjau dampak terkini, validasi konflik token/versi dan konfirmasi khusus. Final dan snapshot lama tetap utuh, dan perubahan dapat menimbulkan Perlu Ditinjau. Penerbitan atau perubahan versi Final tetap eksplisit. Siswa lain yang belum Final tetap dapat diisi atau dikoreksi secara rutin pada TP yang sudah ada; penguncian penambahan struktur TP setelah Final pertama dalam konteks tetap berlaku. Hak pemilik/guru pengganti tidak melebar.

**PN-03 / S-05.** Formatif adalah umpan balik teks singkat opsional, boleh ditautkan pada TP tetapi tidak wajib; tidak mensyaratkan angka, status atau pembuatan kegiatan Sumatif. Formatif tidak masuk NH, rekomendasi, kelengkapan atau bukti minimal Final Pengecualian. Guru tidak wajib mengisinya ketika menyimpan nilai TP.

**PN-04.** Model aktif tidak menyediakan remedial harian/TP baru. Koreksi kesalahan pencatatan adalah koreksi administratif, bukan penilaian ulang yang menimpa nilai asli.

**PN-05 / S-06.** Penempatan normal yang terverifikasi tidak membutuhkan pengaturan ekstra. Untuk siswa pindahan/masuk ketika semester berjalan atau penempatan tidak pasti, guru berwenang menetapkan TP **Berlaku/Tidak Berlaku** berdasarkan pembelajaran/penilaian nyata dan tanggal efektif penempatan yang dapat diverifikasi; catat aktor, waktu dan alasan ringkas pengecualian. Jangan menebak tanggal dari waktu penyimpanan, mewajibkan semua TP otomatis, mengisi nol, atau mengimpor/mengakui nilai asal otomatis. Jika penempatan belum pasti, tampilkan **Perlu penetapan/verifikasi** dan cegah Final tidak sah. Kasus rutin tidak dibebani penetapan tambahan.

## 5. Skala dan NH

**NL-01.** Nilai TP, ASTS, ASAS dan remedial ujian berada pada 0–100 dengan maksimal dua angka desimal. `0` valid; kosong bukan 0. Format invalid atau nilai di luar rentang ditolak.

**NL-02.** Nilai TP berlaku adalah nilai awal langsung atau hasil koreksi administratif sah terbaru. **NL-03.** NH adalah rata-rata aritmetika tanpa bobot dari seluruh nilai TP yang berlaku bagi siswa. TP berbobot setara. Bila ada nilai TP berlaku yang kosong, NH belum lengkap; tidak ada imputasi atau rata-rata parsial yang disajikan sebagai lengkap.

**NL-04.** Bila belum ada TP yang berlaku atau penyebut tidak dapat ditentukan, NH tidak memiliki nilai numerik dan tidak dianggap lengkap ketika bobot NH positif. Ketidakpastian penempatan/keberlakuan ditampilkan, bukan ditebak. Ini tidak meniadakan aturan bobot 0% atau ketentuan Final yang sah.

**NL-05.** Tidak ada pembulatan pada perhitungan antara; tampilan TP/NH dua desimal. Perbandingan ketercapaian menggunakan nilai TP eksak sebelum pembulatan. Pembulatan rekomendasi dilakukan pada akhir sesuai BT-03.

## 6. ASTS, ASAS dan remedial

**EX-01.** Masing-masing komponen memakai satu sumber nilai awal aktif yang sah per siswa–mapel–semester; sumber berbeda antar kelas tetap terlacak dan tidak otomatis digabung. **EX-02.** Beberapa remedial sah boleh dicatat pada sumber terpilih. Nilai efektif komponen adalah maksimum nilai dasar setelah koreksi administratif sah dan seluruh remedial sah sumber tersebut.

**EX-03.** Nilai awal asli, koreksi, remedial, tanggal, pelaksana dan histori tidak dihapus atau ditimpa; koreksi kesalahan berbeda dari remedial. **EX-04.** Koreksi atau remedial setelah Final memerlukan peninjauan dan konfirmasi Final eksplisit; Final lama tidak berubah otomatis.

**EX-05–07.** Nilai asal dan remedial lintas kelas bagi siswa pindahan hanya digunakan setelah pengakuan eksplisit yang sah; asal dan kepemilikan tetap, tidak dihitung ganda. Koreksi sumber yang telah diakui tidak otomatis mengubah pengakuan; ditandai untuk ditinjau. Guru tujuan dapat melakukan remedial sesuai kewenangan tanpa mengubah asli guru asal. Perpindahan berulang tetap terlacak.

**EX-08–09.** Bila ada beberapa hasil ujian sah dari kelas berbeda, guru tujuan memilih satu sumber aktif secara eksplisit dengan alasan dan audit; hasil lain tetap histori. Maksimum hanya antara nilai dasar dan remedial sah dalam rangkaian sumber terpilih, bukan maksimum otomatis antardua ujian. Perubahan sumber setelah Final memerlukan peninjauan dan konfirmasi Final eksplisit.

## 7. Bobot, indikator dan status

**BT-01/02.** Kurikulum mengatur satu bobot NH/ASTS/ASAS yang seragam bagi sekolah per semester. Guru tidak mengubah bobot; total harus tepat 100%, komponen 0% diizinkan.

**BT-03.** Jika seluruh komponen berbobot positif lengkap: `rekomendasi = round_half_up(NH × bobot_NH + ASTS_efektif × bobot_ASTS + ASAS_efektif × bobot_ASAS)`, bobot sebagai proporsi dan pembulatan ke bilangan bulat terdekat dilakukan hanya pada akhir; tepat 0,50 dibulatkan ke atas.

**BT-04–05b.** NH positif mensyaratkan semua nilai TP yang berlaku lengkap; ASTS/ASAS positif mensyaratkan nilai sah masing-masing. Komponen 0% tidak ikut rekomendasi, status kelengkapan atau syarat Final normal, namun boleh dicatat. Nilai kosong bukan nol dan bobot tidak dinormalisasi. Komponen 0% tidak dapat menjadi satu-satunya bukti Final Pengecualian. Perubahan bobot dari 0% ke positif mengevaluasi kelengkapan aktif; **BT-06** perubahan bobot setelah Final memerlukan alasan, audit dan pemeriksaan dampak; snapshot/Final lama tetap dan terdampak ditandai Perlu Ditinjau.

**KT-01.** Kurikulum menetapkan pedoman umum; Guru Mapel menetapkan batas bawaan kelas–mapel–semester dan pengecualian per TP. **KT-02.** Indikator memakai nilai TP berlaku sebelum pembulatan dan bukan penghalang otomatis Final. **KT-03.** Perubahan kriteria diaudit; snapshot indikator dan kriteria pada setiap Final tidak ditimpa.

**FN-01–04.** Empat status: **Belum Dinilai: belum ada satu pun bukti nilai sah pada komponen mana pun yang berbobot positif.** **Belum Lengkap** bila sudah ada setidaknya satu bukti pada komponen berbobot positif tetapi syarat rekomendasi belum lengkap; **Siap Final** bila semua komponen positif lengkap dan rekomendasi tersedia; **Final** bila nilai akhir ditetapkan secara eksplisit. Status mempertimbangkan bukti NH, ASTS dan ASAS yang berbobot positif, bukan hanya ketersediaan penyebut NH. Dengan Pengecualian dan Perlu Ditinjau adalah penanda, bukan status kelima.

**FN-05.** Final normal individual atau massal untuk siswa yang memenuhi syarat; nilai berbeda dari rekomendasi memerlukan alasan. **FN-06.** Final Pengecualian hanya individual, nilai manual dan alasan wajib, dengan setidaknya satu bukti Sumatif sah pada komponen berbobot positif. Nilai TP langsung yang sah merupakan bukti Sumatif NH untuk perhitungan dan kelayakan ini ketika bobot NH positif, tanpa mensyaratkan baris kegiatan. Sel kosong atau catatan Formatif bukan bukti Sumatif; nilai komponen 0% saja tidak cukup. **FN-07.** Nilai Final bilangan bulat 0–100, setiap versi menyimpan alasan, pelaksana, waktu, sumber nilai, struktur, kriteria, bobot dan snapshot/hash. Final lama tidak diperbarui otomatis.

## 8. Koreksi, histori dan perpindahan

**KR-01.** Pemilik Final aktif dapat mengoreksi Final historis miliknya secara sah; pengecualian administratif pemilik tidak aktif oleh Kurikulum memerlukan alasan dan audit. **KR-02.** Koreksi nilai/ujian memengaruhi perhitungan aktif, tetapi tidak otomatis memperbarui Final; pemilik melakukan peninjauan dan konfirmasi versi baru atau keputusan mempertahankan sesuai hak. **KR-03.** Koreksi struktur yang sudah digunakan memerlukan alasan, audit, pratinjau dampak dan tindak lanjut bertahap; Final yang belum ditinjau tetap berlaku. **KR-04.** Konfirmasi menggunakan pemeriksaan ulang snapshot/version; perubahan sumber sejak pratinjau menghasilkan konflik tanpa simpan parsial.

**PS-01.** TP siswa pindahan tidak otomatis berlaku; gunakan penetapan PN-05. **PS-02.** Pengakuan nilai TP asal tidak otomatis disalin, dirata-rata atau disimpulkan sebagai nilai tujuan; mekanisme pengakuan nilai langsung lintas kelas menunggu keputusan khusus. **PS-03.** Final asal tetap berlaku sebagai histori hingga Final tujuan ditetapkan secara sah. **PS-04.** ASTS/ASAS asal dan beberapa sumber ujian mengikuti EX-05–09. Setiap keputusan menjaga sumber, pelaku dan asal-usul; perpindahan berulang tidak menggandakan bukti.

Susulan CP10-24 belum diimplementasikan atau diizinkan melalui kontrak ini. Ketentuan Revisi 2 tentang kegiatan Sumatif, tanggal dan susulan menjadi bahan retensi/transisi jika ada data yang perlu dipertahankan, bukan alur rutin atau mesin NH aktif kedua.

## 9. Buku Nilai Guru

Tiga tab menggunakan navigasi bersama D-047. Setelah memilih konteks berizin, guru melihat tabel siswa: nama tetap terlihat saat scroll horizontal, header dan navigasi mudah dijangkau, kolom `TP1…TPn` menaut ke TP asli dan uraian lengkap tersedia saat dibutuhkan. Satu tombol **Simpan** jelas, dengan status berhasil/error dan penanda perubahan belum disimpan.

Tabel mendukung kelas **36 siswa atau lebih** di desktop dan ponsel, dengan scroll horizontal internal; tidak memakai kartu per siswa. Pagination/filtrasi/navigasi tidak boleh menghilangkan input belum disimpan atau menyembunyikan cakupan simpan. Kesalahan sel dan konflik memperlihatkan tindakan yang diperlukan sambil mempertahankan draft. Koreksi pra-Final dilakukan pada tabel; koreksi pasca-Final melalui alur khusus. Tab ASTS/ASAS memperlihatkan asli, remedial, nilai efektif dan histori. Penggunaan keyboard, label dan pesan harus dapat diakses.

## 10. Matriks perubahan Revisi 2 → Revisi 3

| Klausul Revisi 2 | Perlakuan dalam kontrak ini |
|---|---|
| BK-01–06 | Ketentuan operasional ditulis pada §2; hak guru dan pengecualian tidak melebar. |
| TP-01–06 | Ditulis pada §3; ditambah TP-07/S-03 kode stabil. |
| PN-01/02/04/05 | Prasyarat kegiatan, multi-TP dan kewajiban setiap kegiatan diganti nilai langsung satu siswa–TP; kegiatan lama diperlakukan sebagai histori/transisi, bukan mesin aktif. |
| PN-03/06 | Diganti/dirinci oleh Formatif teks opsional dan S-06 keberlakuan pindahan pada §4. |
| NL-01/04/05 | Skala, rata-rata TP setara dan presisi dipertahankan dalam §5. |
| NL-02/03 | Remedial harian dan rata-rata kegiatan per TP tidak menjadi aturan aktif; nilai langsung/koreksi sah menjadi dasar NH. |
| EX-01–09 | Nilai asli, koreksi, remedial, pilihan sumber dan histori ditulis operasional pada §6. |
| BT-01–06, KT-01–03, FN-01–07 | Bobot, 0%, indikator, status, Final dan pengecualian ditulis pada §7; sumber NH tunggal disesuaikan. |
| KR-01–06 | Perlindungan koreksi/Final dan konflik dipertahankan dalam §4/8; tanggal kegiatan historis menjadi urusan retensi/transisi, bukan syarat input baru. |
| KR-07–11 | Susulan tetap ON HOLD; tidak menciptakan alur susulan baru melalui kontrak ini. |
| PS-01–10 | Keberlakuan S-06, perlindungan asal/Final dan pengakuan ujian ditulis pada §4/6/8. Pengakuan nilai TP langsung lintas kelas belum ditetapkan. |
| AC-01–29 | Skala, ujian, bobot, Final, akses, kriteria dan histori diserap ke AC-R3 di bawah. AC terkait kegiatan multi-TP/rata-rata kegiatan/remedial harian/susulan tidak menjadi acceptance alur aktif; pengujian retensi ditetapkan sesuai keputusan data nyata. |

## 11. Acceptance criteria

| ID | Kebutuhan yang dapat diuji |
|---|---|
| AC-R3-01 | Guru berwenang memilih konteks dan menyimpan satu nilai siswa–TP tanpa membuat kegiatan; 0, 82,50, 100 sah; kosong tetap kosong; negatif, 100,01 dan lebih dari dua desimal ditolak. |
| AC-R3-02 | Input bertahap dan koreksi pra-Final lewat tabel menghasilkan audit otomatis before/after/aktor/waktu tanpa formulir alasan; asli tetap ada. |
| AC-R3-03 | Pengisian awal sel kosong maupun koreksi nilai setelah siswa bersangkutan Final memerlukan alasan, pratinjau dampak terkini, validasi konflik token/versi dan konfirmasi khusus; Final/snapshot lama tetap. Guru sah tetap dapat mengisi/mengoreksi TP existing secara rutin bagi siswa lain yang belum Final, sementara penambahan struktur TP setelah Final pertama tetap terkunci. |
| AC-R3-04 | Kode TP stabil per konteks, menunjuk ID asli, tidak dipakai ulang; salinan mendapatkan nomor tujuan dan uraian lengkap tersedia. |
| AC-R3-05 | NH memakai hanya nilai langsung TP berlaku, dengan bobot setara, kosong tidak jadi nol, dan presisi tidak dibulatkan sebelum perhitungan akhir. Tidak ada mesin NH kegiatan aktif kedua. |
| AC-R3-06 | NH positif yang belum lengkap menahan rekomendasi; nilai TP langsung sah adalah bukti Sumatif NH untuk Final Pengecualian jika NH positif, tanpa kegiatan. Sel kosong atau Formatif bukan bukti. Belum Dinilai/Belum Lengkap mempertimbangkan semua komponen berbobot positif. NH 0% tidak menahan Final normal ketika komponen positif lain lengkap. Tidak ada nilai NH numerik jika tidak ada TP berlaku atau penyebut tak pasti. |
| AC-R3-07 | ASTS/ASAS menampilkan asli, koreksi dan remedial; nilai efektif maksimum dalam sumber aktif; nilai asli tetap, dan ASAS berbeda dari nilai rapor. |
| AC-R3-08 | Formatif teks opsional, dengan atau tanpa TP, tidak memengaruhi NH, rekomendasi, kelengkapan atau bukti Final Pengecualian. |
| AC-R3-09 | Pindahan/tanggal belum pasti tidak otomatis diwajibkan atau diberi nol; penetapan sah diaudit dan ketidakpastian menahan Final tidak sah. |
| AC-R3-10 | Kelas 36+ siswa dapat diisi di desktop/ponsel; nama/header mudah terlihat, scroll internal dan pagination mempertahankan draft. |
| AC-R3-11 | Hak role/penugasan/pemilik, CSRF, token basi, replay identik/berbeda, konkurensi dan rollback ditegakkan tanpa data parsial atau akses lintas guru. |
| AC-R3-12 | Final berversi, kriteria/bobot/sumber dan snapshot terlacak; perubahan sumber atau bobot tidak menimpa Final dan Perlu Ditinjau ditampilkan sesuai hak. |
| AC-R3-13 | Data yang wajib dipertahankan, termasuk nilai, audit, Final dan hash, tetap identik sepanjang transisi; setiap pengecualian berprovenance dan disetujui. |

## 12. UAT yang diusulkan

**UAT A — tabel nilai dan Final:** pada fixture sintetis 36+ siswa, pilih konteks, isi nilai TP 0/82,50/100 bertahap lintas halaman, simpan dan refresh pada desktop serta ponsel. Buktikan nilai tidak hilang, NH/rekomendasi, audit otomatis dan koreksi pra-Final. Finalkan siswa yang memenuhi syarat, tinjau koreksi pasca-Final dengan alasan, dan buktikan snapshot/versi sebelumnya tetap.

**UAT B — ujian, pindahan dan retensi:** buktikan ASTS/ASAS asli dan beberapa remedial, nilai efektif, NH positif/0%, penetapan siswa pindahan serta penempatan yang belum pasti. Bandingkan hash dan data yang ditetapkan wajib dipertahankan dengan baseline. Akses asing, CSRF, konflik, replay dan konkurensi dibuktikan juga dengan tes terfokus; hasil otomatis tidak dilabeli sebagai UAT browser.

## Lampiran A — Rancangan teknis nonoperatif dan gerbang transisi

Ini usulan untuk review, bukan otorisasi eksekusi.

| Klasifikasi | Komponen dan langkah |
|---|---|
| **Pertahankan** | Identitas/penempatan/penugasan, TP dan kriteria, bobot Kurikulum, ASTS/ASAS asli/koreksi/remedial, otorisasi, audit dan Final/snapshot yang wajib diretentukan. |
| **Refaktor** | Sumber NH pada `assessment_tp_recap.go`, `semester_recommendation.go`, snapshot Final baru dan UI/handler menjadi satu Buku Nilai; pertahankan status FN/BT dan peringatan penempatan. |
| **Pensiunkan bertahap** | Route/service kegiatan Sumatif dan remedial harian sebagai alur aktif setelah dependensi dan pengganti diuji. Schema dan migration lama boleh tetap fisik sebagai arsip; bukan kalkulator NH aktif kedua. Akses histori yang perlu disimpan memerlukan otorisasi dan pembaca snapshot yang kompatibel. |

**Database:** usulkan database TEST baru yang bersih dan terisolasi dengan migration baseline dan migration baru aditif serta fixture sintetis baru, setelah instruksi implementasi terpisah. Jangan mengubah migration 00001–00060 yang telah diterapkan. Usulan schema aditif mencakup pemetaan kode TP, nilai awal langsung unik siswa–TP–konteks, koreksi append-only, keberlakuan siswa/TP, Formatif teks dan snapshot baru berversi, lengkap dengan FK, constraint, idempotensi dan transaksi. Nama serta rincian schema perlu review implementasi.

**Retensi:** TEST/UAT mengandung fixture dan bukti; fixture hanya dapat dibersihkan setelah dataset tepat dan tindakan spesifik diotorisasi. Audit terakhir melaporkan TEST/UAT 60/60 dan DEV 52/60; DEV berisi data akademik dan modul lain yang belum boleh diasumsikan aman dihapus. Simpan atau karantina DEV sampai provenance jelas. Jika nilai/Final lama terbukti bukan fixture dan masih dibutuhkan, siapkan pekerjaan transisi/retensi khusus berdasarkan keputusan PO; tidak ada konversi otomatis, pembaruan snapshot/hash, atau penghapusan diam-diam. 55 versi Final UAT sintetis sendiri bukan dasar untuk mewajibkan mesin NH lama permanen.

**Backup/pemulihan:** sebelum perubahan yang kelak diizinkan, inventaris dependensi dan baseline seluruh kolom/schema; `pg_dump -Fc` privat 0700/0600, checksum, `pg_restore --list`, lalu restore penuh ke scratch terisolasi dan rekonsiliasi. Uji pemulihan, FK, audit dan hash Final sebelum serta setelah operasi. Jika gagal, STOP; restore destruktif, reset atau migration down memerlukan instruksi terpisah. Tidak ada data yang dinyatakan aman dihapus hanya dari nama lingkungan.

**Urutan checkpoint usulan:** (1) pengesahan kontrak/retensi; (2) fondasi TP langsung dan kode stabil; (3) rekap NH dan rekomendasi satu sumber; (4) integrasi ASTS/ASAS; (5) Buku Nilai responsif; (6) Formatif; (7) pensiun route/service lama dengan tes dependensi; (8) integrasi, UAT dan rehearsal pemulihan. Setiap checkpoint memerlukan review dan izin tersendiri.

## Lampiran B — Keputusan yang masih diperlukan

Butir transisi dan retensi di bawah **tidak menahan pengesahan kebijakan satu model** atau dimulainya pengembangan terisolasi pada database uji bersih setelah ada izin implementasi terpisah. Membuat database/fixture belum diizinkan oleh kontrak ini. Penghapusan fixture, konversi data nyata, transisi DEV dan deployment tetap memerlukan pemeriksaan serta izin terpisah; pengakuan nilai TP langsung lintas kelas belum menjadi fitur yang disetujui.

1. Untuk nilai/Final lama yang terbukti bukan fixture dan masih diperlukan, tetapkan retensi, tampilan histori dan perlakuan operasional/transisi sebelum refaktor diterapkan; jangan mengonversi diam-diam.
2. Tentukan dataset fixture TEST/UAT yang boleh dibersihkan hanya setelah pemetaan dan persetujuan spesifik; database uji bersih direkomendasikan sebagai jalur pertama.
3. Mekanisme pengakuan nilai TP langsung lintas kelas dan kewenangan sumber/tujuan belum ditetapkan; jangan mengimpor atau menyimpulkan nilai otomatis.
4. Penanganan konteks semester berjalan yang memiliki nilai kegiatan operasional perlu keputusan setelah provenance dipastikan; jangan memaksakan konversi atau menghapus kegiatan.

NL-04 disahkan bersama Revisi 3 oleh Product Owner pada 23 November 2026.
