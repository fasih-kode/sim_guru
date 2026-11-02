# PROMPT CODEX — IMPLEMENTASI FASE 10 SIM-GURU v1.1 (MIKRO-CHECKPOINT, 1–2 UAT)
## Penilaian Hasil Belajar | Kontrak Bisnis Revisi 2 DISAHKAN

> **INSTRUKSI UNTUK CODEX DESKTOP**
>
> Jalankan pekerjaan di repository lokal SIM-GURU, bukan di lingkungan produksi. Pemilik aplikasi telah mengesahkan **Kontrak Bisnis Fase 10 Revisi 2 pada 1 November 2026**. Tahap diskusi kebijakan bisnis sudah selesai. Kerjakan **HANYA SATU MIKRO-CHECKPOINT PER PERINTAH**, dibatasi **1–2 skenario UAT** yang saling terkait. Jangan mengerjakan fitur tambahan meskipun bergantung pada area yang sama. Setelah review/UAT yang diminta selesai, tetap **BERHENTI**; kerjakan checkpoint berikutnya hanya jika diminta secara eksplisit. **Alur Git terbaru:** setelah implementasi dan tes checkpoint lulus, commit serta push normal ke branch review dapat diminta sebelum review independen dan UAT. Jangan merge atau deploy tanpa instruksi eksplisit; jangan force-push. Instruksi checkpoint terbaru menentukan apakah publikasi sebelum UAT diizinkan. Jangan meminta persetujuan ulang atas keputusan FINAL.

---

## 0. Identitas, tujuan, dan status yang harus dipahami

- Proyek: **SIM-GURU v1.0**, administrasi guru dan akademik satu SMP.
- Repository lokal yang diharapkan: `/home/fasih/Projects/sim_guru`.
- Branch kerja historis: `review/sim-guru-checkpoint`; **verifikasi branch, HEAD, dan working tree terkini**. Jangan otomatis checkout/reset/clean.
- Stack harus mengikuti repo: **Go + `html/template` + HTMX + PostgreSQL + Tailwind/TailAdmin**. JS ringan yang sudah ada boleh digunakan; **jangan membuat React, SPA, framework backend, atau layanan baru**.
- Port preview yang sudah digunakan: `http://127.0.0.1:18743`, dipakai **bergantian** untuk DEV atau UAT, bukan dua instance bersamaan.
- Database historis: `sim_guru_dev` (data pengembangan nyata), `sim_guru_test` (tes otomatis), `sim_guru_ci_test` (UAT sintetis). PostgreSQL lokal historis `127.0.0.1:55436`. **Verifikasi konfigurasi nyata dan identitas database sebelum operasi tulis**; informasi ini bukan bukti bahwa proses saat ini masih sama.
- Checkpoint remote terakhir dari handover: `5e1a8a0bf54ea5a61f31ea8076e8278825c24f68` dan migration 36/36. **Hanya baseline historis, jangan asumsikan HEAD/migration masih di sana.**
- Fase 9 Jurnal Mengajar beserta patch historis telah diterima; **jangan buka ulang tanpa bug nyata**.
- Login OTP email lewat Brevo **DITUNDA** oleh pengguna karena belum memiliki domain. **Jangan mengubah autentikasi, menambah OTP, atau menjadikan Brevo/domain sebagai blocker Fase 10.**

### Sasaran Fase 10

Target akhir setelah SELURUH checkpoint kelak selesai: modul Penilaian Hasil Belajar berfungsi **end-to-end** bagi Kurikulum dan Guru Mapel: konfigurasi bobot sekolah; TP; Formatif/Sumatif Harian multi-TP; input nilai; susulan; remedial; ASTS/ASAS; indikator ketercapaian; rekomendasi nilai; status dan Finalisasi; histori/koreksi/peninjauan; dan pengakuan bukti siswa pindahan, sesuai **semua** acceptance criteria kontrak. UI akhir harus layak dipakai guru dalam daftar siswa nyata, bukan hanya endpoint/API atau form contoh. **Target akhir ini BUKAN izin mengerjakan seluruh cakupan sekarang.**

## 1. Sumber kebenaran dan urutan prioritas

**Wajib dibaca sebelum desain dan coding:**

1. `AGENTS.md` aktual dan instruksi repo lain yang relevan untuk praktik teknis, keselamatan data, dan pengujian.
2. `docs/contracts/KONTRAK_BISNIS_FASE_10_SIM_GURU_v1_0_REVISI_2_UNTUK_PENGESAHAN.md` (**kontrak bisnis Fase 10 yang SUDAH DISAHKAN pengguna**).
3. `docs/HANDOVER.md` serta `docs/DECISIONS.md`.
4. PRD dan FE PRD yang ada, terutama `PRD_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md`, `PRD_FE_SIM_GURU_v1.0_REVISI_PEMBAGIAN_TUGAS.md`, dan roadmap aktual jika berkasnya tersedia.
5. Struktur dan pola implementasi kode yang sudah berjalan (otorisasi, penugasan efektif, migrasi, SSR+HTMX, audit, validasi, testing, UI).

**Kontrak Revisi 2 yang disahkan mengungguli isi PRD/roadmap lama apabila menyangkut aturan bisnis penilaian yang sudah diperbarui** (misalnya rumus nilai, ASTS/ASAS, bobot, pengecualian, hak koreksi Final). Jangan mengikuti aturan lama yang menuntut persetujuan Kurikulum atas seluruh koreksi Final guru. Tetap patuhi ketentuan keamanan dan `AGENTS.md`; jika ada kontradiksi keamanan/otorisasi yang benar-benar tidak dapat didamaikan, jelaskan secara spesifik sebelum tindakan berisiko.

**Tugas administrasi dokumentasi di awal:** file kontrak masih bernama `...UNTUK_PENGESAHAN.md` dan mungkin memuat kalimat “belum disahkan”. **Pengguna sudah mengesahkan keseluruhan naskah pada 1 November 2026.** Perbarui **metadata status saja** menjadi `FINAL — DISAHKAN PENGGUNA, 1 November 2026` (dan catatan penunjuk pada `docs/DECISIONS.md`/`docs/HANDOVER.md` bila relevan), **tanpa mengubah pasal bisnis, formula, acceptance criteria, atau UAT**. Pertahankan path kontrak semula agar tautan tidak rusak. Ikuti aturan publikasi terbaru di awal prompt: checkpoint dapat di-commit/push sebelum UAT jika instruksi checkpoint mengizinkan, sedangkan merge/deploy tetap memerlukan instruksi terpisah.

Jika file kontrak tidak ditemukan atau isinya tidak sesuai, **jangan mengarang pengganti dan jangan lanjut membuat skema penilaian**. Laporkan lokasi yang dicari, hasilnya, dan blocker. Jika ditemukan keputusan lama bertentangan dengan kontrak yang telah disahkan, catat bahwa aturan terkini menang; tidak perlu bertanya ulang.

## 2. Aturan kerja dan larangan yang tidak bisa ditawar

1. Mulai dengan pemeriksaan **terfokus**: `git status --short`, branch/HEAD, lokasi kontrak, pola model/route/handler/template, kepemilikan data penugasan, student transfer efektif, audit, dan test helper. Jangan mengaudit ulang seluruh Fase 1–9 atau membuat proses review berkepanjangan.
2. Jika working tree memiliki perubahan sebelumnya, **jaga dan laporkan**; jangan `git reset --hard`, `git clean -fd`, stash sembarangan, atau menimpa pekerjaan yang bukan milikmu.
3. Tuliskan **daftar checkpoint ringkas untuk gambaran saja**, tetapi aktifkan **hanya satu checkpoint berisi 1–2 UAT** pada satu waktu. Daftar tersebut bukan backlog yang boleh dieksekusi otomatis.
4. **Kerjakan hanya checkpoint pertama yang tertulis pada §10** setelah verifikasi aman. Tidak perlu meminta izin untuk setiap langkah teknis *di dalam checkpoint itu*. **Wajib berhenti begitu 1–2 UAT yang dipilih siap**, saat ada risiko besar, atau keputusan bisnis sungguh-sungguh OPEN. Jangan menambah cakupan sendiri.
5. Tidak boleh melakukan reset/truncate, menimpa nilai historis, menghapus data dev, migrasi destruktif, memperbaiki histori dengan SQL ad hoc, memanipulasi waktu server, backdating, bypass hak user, atau menyisipkan fixture ke `sim_guru_dev` untuk memaksakan tes PASS.
6. Migration mengikuti pola repo, **aditif, bernomor/berversi baru**, bukan mengedit migration 00001–00036 atau migration lain yang sudah diterapkan. Sebelum menerapkan migration ke `sim_guru_dev`: audit target DB **secara nyata**, backup aman, pastikan backup terbaca, cek dampak SQL, dan uji migration pada database pengujian terlebih dulu. Jangan klaim backup teruji full restore hanya karena `pg_restore --list` lulus.
7. Jalankan tes otomatis hanya di `sim_guru_test` (dan UAT sintetis di `sim_guru_ci_test`), **bukan di database dev**. Jangan membuat DB/port baru tanpa kebutuhan nyata dan persetujuan pengguna.
8. Untuk preview `127.0.0.1:18743`, verifikasi proses yang aktif dan **database yang benar-benar terhubung**; bila mengganti DEV↔UAT, lakukan secara bergantian, laporkan mode preview yang hidup dan kembalikan ke DEV setelah UAT sesuai kebiasaan proyek. Jangan mematikan proses tak terkait.
9. Jangan ubah Fase 1–9 kecuali integrasi minimal yang benar-benar diperlukan; semua regresi pada absensi, jurnal, kelas, jadwal, penugasan, login, dan histori menjadi blocker.
10. Jangan membuat approval Kepala Sekolah, mandat/delegasi, guru pengganti/cadangan, atau pemeriksaan Kurikulum yang wajib untuk koreksi Final rutin guru. Guru pemilik yang masih aktif dapat mengoreksi Final miliknya; Kurikulum hanya koreksi administratif pengecualian bila guru pemilik tidak aktif.
11. Beri error yang jelas dalam bahasa Indonesia tanpa bocor SQL, stack trace, credential, data lintas hak, atau rahasia `.env`.
12. Dokumentasi, laporan Codex, komentar kode yang memang diperlukan, serta keterangan UI gunakan **bahasa Indonesia**; nama simbol kode tetap ikuti standar Go dan konvensi repo.

## 3. Pemetaan aturan bisnis ke implementasi — ACUAN SELURUH FASE (BUKAN TARGET SEKALIGUS)

**Kontrak asli adalah sumber utama. Bagian ini indeks implementasi, bukan pengganti kontrak.** Gunakan kode AC-01 s.d. AC-29 dan UAT-01 s.d. UAT-40 dari dokumen untuk memeriksa cakupan bertahap. **Pada checkpoint yang sedang aktif, hanya AC/UAT yang ditugaskan yang boleh menjadi target implementasi.**

### 3A. Otorisasi dan penugasan

- Admin menetapkan akun/role dan penugasan menurut alur sekolah yang sudah ada; Kurikulum mengatur **bobot tingkat sekolah per tahun pelajaran/semester**.
- Guru Mapel boleh membuat/mengubah data pada kelas, mapel, dan semester yang menjadi penugasan sah; pemeriksaan server pada **setiap mutasi** dan pembacaan sensitif.
- Riwayat nilai/Final milik guru lama tidak boleh diedit guru baru. Guru lama yang masih aktif boleh memperbaiki catatan historis miliknya dengan akses **khusus objek miliknya**, bukan akses tanpa batas ke kelas lama.
- Jika guru pemilik tidak aktif, Kurikulum dapat melakukan koreksi administratif beralasan dan diaudit. Guru pengampu tujuan berhak melakukan pengakuan nilai dan Final terbaru siswa pindahan.
- Ambil keadaan penugasan/kelas/tanggal efektif dari sumber resmi yang sudah tersedia; jangan menebak kelas dari UI, menghidupkan role lama, atau membuat akses sampingan.

### 3B. TP, kriteria, kegiatan, dan nilai dasar

- TP per **kelas × mapel × semester**; salin **definisi saja** ke kelas resmi lain yang diampu guru, tanpa menyalin kegiatan, nilai, histori, atau batas khusus TP. TP tujuan independen dan batasnya mengikuti kelas tujuan.
- Sebelum ada nilai dan belum terkunci: edit biasa; setelah ada nilai: koreksi redaksi dengan alasan+audit; perubahan substansi atau penambahan historis setelah Final: **jalur koreksi struktur** dengan preview dampak dan histori. Catatan bersejarah tidak dapat dihapus permanen.
- Komponen kegiatan fleksibel, wajib bertipe **Formatif/Sumatif**. Satu kegiatan bisa menilai **beberapa TP** dan memiliki **nilai terpisah per siswa per TP**. Formatif dipakai untuk pembelajaran saja, tidak mengubah status/NA.
- Sumatif yang berlaku wajib lengkap untuk TP yang dinilainya bila NH berbobot positif. Kegiatan Sumatif/TP rutin terkunci setelah **Final pertama** pada kelas/mapel/semester. Penambahan historis harus sesuai fakta, beralasan, melihat dampak, tidak mengubah Final otomatis.
- Skala semua nilai/ujian/remedial **0–100 dengan maksimal dua desimal**; **0 sah dan kosong berbeda dari 0**. Input `82,50` dapat dipahami sesuai format Indonesia dengan validasi server; tidak menerima tiga digit desimal, angka negatif, atau di atas 100.
- Kriteria ketercapaian: kebijakan umum Kurikulum, default Guru Mapel per kelas/mapel/semester, override per TP. Indikator berdasarkan **rata-rata TP mentah**, bukan nilai tampilan yang dibulatkan; indikator bukan penghalang Final atau remedial wajib. Snapshot lama dipertahankan dalam versi Final.

### 3C. Sumatif Harian/TP dan perhitungan

- Nilai efektif per kegiatan = `max(nilai awal, semua remedial sah yang berlaku)`; remedial beberapa kali **bukan** kegiatan Sumatif tambahan. Tidak boleh menghapus/menimpa nilai awal.
- `Nilai TP = rata-rata aritmetika nilai efektif semua kegiatan Sumatif yang relevan untuk TP` dengan bobot sama per kegiatan.
- `NH = rata-rata aritmetika seluruh TP berlaku` dengan bobot sama antarseluruh TP, **bukan** rata-rata seluruh kegiatan lintas TP.
- Satu kegiatan dapat menilai beberapa TP; nilai setiap TP perlu terhitung tepat satu kali, tanpa campur hasil TP lain.
- Simpan ketelitian perhitungan, **jangan membulatkan setiap tahap**. Nilai TP ditampilkan dua desimal, indikator bandingkan angka sebelum pembulatan. Gunakan representasi aritmetika decimal/fixed-point/rational yang sesuai pola repo untuk menghindari kesalahan pecahan ambang `.50`; jangan memakai pembulatan binary float yang tidak stabil.

### 3D. ASTS, ASAS, bobot Kurikulum

- ASTS dan ASAS masing-masing **satu sumber nilai awal yang berlaku** per siswa/mapel/semester untuk perhitungan; catatan remedial berulang dan koreksi historis terpisah. Nilai efektif masing-masing = nilai tertinggi nilai awal dan remedial sah dari **sumber yang dipilih**.
- Jika siswa memiliki lebih dari satu hasil ASTS/ASAS dari kelas berbeda: guru tujuan **memilih satu sumber secara eksplisit** dengan alasan+audit; hasil lain tetap histori. **Jangan** auto-maksimum atau rata-rata dua ujian berbeda. Jika pindah lagi, sumber bisa ditelusuri tanpa duplikasi.
- Kurikulum mengelola tepat satu konfigurasi bobot **seragam seluruh sekolah** per tahun pelajaran/semester: `BH` (NH), `BT` (ASTS), `BS` (ASAS), tiap bobot boleh 0%, total **persis 100%**. Angka seperti `60:20:20` adalah **contoh UAT**, bukan default kebijakan diam-diam. Guru tidak berhak mengubah bobot.
- Rekomendasi bila lengkap: `round_half_up(NH × BH + NT × BT + NS × BS)` dengan bobot sebagai proporsi; hasil integer 0–100, tanpa pembulatan antara.
- **Komponen dengan bobot 0%**: boleh dicatat dan diremedial serta histori tetap ada, tetapi **tidak** dihitung dalam rekomendasi, tidak memengaruhi status kelengkapan atau Final normal, dan tidak menjadi bukti minimum pengecualian. Jika NH bobot 0%, TP/Sumatif Harian yang kosong **tidak** memblokir Final normal.
- Nilai komponen positif yang kosong **bukan 0**; jangan mengalihkan/menormalisasi bobot untuk membuat rekomendasi tampak tersedia.
- Perubahan bobot setelah sudah ada Final: Kurikulum memberi alasan dan melihat **pratinjau dampak seluruh sekolah**; rekomendasi non-Final dihitung ulang, Final lama tidak berubah, snapshot bobot lama disimpan, Final terdampak diberi **Perlu Ditinjau**, tinjauan bertahap oleh guru masing-masing. Pindah 0%→positif harus mengubah syarat kelengkapan aktif (non-Final), tidak merusak Final yang ada.

### 3E. Status, Finalisasi, dan versi Final

Empat status utama **tepat**:

1. `Belum Dinilai`: belum ada Sumatif sah dari komponen dengan bobot positif.
2. `Belum Lengkap`: sudah ada Sumatif sah berbobot positif tetapi seluruh syarat rekomendasi belum lengkap.
3. `Siap Final`: semua nilai wajib dari komponen berbobot positif sudah lengkap, rekomendasi tersedia.
4. `Final`: guru menetapkan nilai akhir secara eksplisit. Boleh berpenanda `Dengan Pengecualian` dan/atau `Perlu Ditinjau` (**bukan** status kelima/keenam).

- Final normal **individual atau massal** untuk siswa yang siap. Guru boleh berbeda dari rekomendasi hanya dengan **alasan wajib**.
- Final pengecualian **individual saja**, perlu **satu nilai Sumatif sah dari komponen positif** (NH/ASTS/ASAS), **nilai akhir manual dan alasan wajib**; tidak membuat rekomendasi semu. Formatif atau nilai komponen 0% saja tidak cukup.
- Final merupakan hasil versi yang **immutable historis**. Simpan aktor, alasan, waktu, nilai, basis sumber/struktur yang relevan, snapshot bobot dan kriteria/indikator. Nilai baru harus **konfirmasi eksplisit**; tidak ada auto-update terhadap Final lama akibat nilai, remedial, bobot, TP, tanggal, pindahan, atau koreksi sumber.
- Koreksi Sumatif/Final diotorisasi berdasarkan kepemilikan; koreksi administratif Kurikulum hanya jika pemilik tak aktif. Koreksi sumber dapat mengubah rekomendasi non-Final tetapi Final lama tetap. Buat penanda `Perlu Ditinjau` atas dampak.
- Tinjauan ulang dapat **bertahap** per siswa; bulk hanya bila syarat normal terpenuhi. Jika data berubah setelah pratinjau tetapi sebelum konfirmasi, **tolak penyimpanan lama / minta refresh**, jangan menyimpan nilai berdasar snapshot stale.
- **Atomisitas:** perubahan Final baru, penunjuk versi aktif, snapshot dan audit harus berhasil/gagal dalam satu transaksi. Cegah duplicate final aktif/race serta partial histories.

### 3F. Susulan, tanggal nyata, pindahan, pengakuan dan koreksi

- Tanggal kegiatan kelas adalah tanggal pelaksanaan asli; **tanggal input berbeda**. Susulan dipakai untuk siswa belum mengikuti kegiatan, dicatat dalam **kegiatan yang sama** dengan **tanggal pelaksanaan per siswa**. Susulan menjadi nilai awal, **bukan** remedial dan **tidak** menambah bobot Sumatif.
- Untuk siswa pindahan setelah tanggal kegiatan kelas, kegiatan tersebut tidak otomatis wajib; guru harus **menugaskan susulan secara eksplisit** bila perlu. Semua tanggal individual dan keputusan harus dapat diaudit. Aturan satu slot juga berlaku ASTS/ASAS.
- Tanggal efektif penempatan resmi menentukan kegiatan kelas tujuan yang wajib. Nilai yang menjadi tidak berlaku setelah koreksi tanggal **tidak dihitung tetapi tidak dihapus dari histori**. Koreksi tanggal setelah ada nilai/final harus alasan+audit+pratinjau siswa terdampak; Final tidak berubah otomatis.
- Guru kelas tujuan secara eksplisit menilai kesetaraan substantif TP asal→TP tujuan **satu TP sumber ke satu TP tujuan per pengakuan**; ambil bukti Sumatif asal **per kegiatan** dengan bobot sama, jangan menggabungkan rata-rata TP asal sebagai satu skor.
- Setiap bukti menyimpan ID sumber asli, lintasan pengakuan, pelaksana, waktu, keputusan, nilai yang diakui, dan remedial. Pada 7A→7B→7C atau kembali 7A, **tidak double count** satu bukti dalam TP tujuan. Remedial di tujuan adalah catatan baru milik guru tujuan, tidak mengedit asal. Kembali ke kelas asal: remedial lintas kelas **perlu pengakuan ulang eksplisit**.
- Koreksi sumber **tidak** otomatis mengubah nilai yang telah diakui; tandai `Perlu Ditinjau` dan guru tujuan memutuskan. Aturan pengakuan **juga** berlaku pada ASTS/ASAS; guru memilih satu sumber jika ada beberapa ujian.
- Final semester dari kelas asal yang sudah ada tetap berlaku sampai guru tujuan menetapkan **Final terbaru**; yang lama tetap riwayat. Harus ada **satu versi Final berlaku** untuk siswa/mapel/semester; histori lintas kelas tidak dihapus.

## 4. Rancangan FE untuk keseluruhan fase — DIREALISASIKAN BERTAHAP

Gunakan komponen **TailAdmin** dan template server yang sudah ada. Ikuti struktur sidebar, template partial, flash/error/validation, gaya tabel/form, dan HTMX repo. Desain tampilan untuk guru SMP yang ingin menyelesaikan pekerjaan dengan cepat; jangan membuat wizard panjang atau form yang memaksa banyak klik.

**Akses Kurikulum** (menu sesuaikan struktur aktual):
- Pengaturan bobot per tahun pelajaran/semester: ketiga persentase, validasi real time dan server total 100%, simpan beralasan saat perubahan, tampilkan konfigurasi efektif dan histori, preview dampak lintas sekolah bila sudah ada nilai/Final.
- Koreksi administratif Final **hanya** ketika guru pemilik tidak aktif, dan panel pemantauan yang sesuai role jika sudah ada pola tersebut. Tidak membuat approval rutin.

**Akses Guru Mapel** (navigasi sesuai penugasan sah):
- Pilih **tahun pelajaran → semester → kelas → mapel** dengan default/konteks efektif sesuai praktik repo.
- **TP:** daftar, tambah, edit, salin definisi ke kelas lain yang diampu, batas ketercapaian default dan override per TP; tampilkan alasan/histori untuk perubahan sensitif.
- **Kegiatan:** buat kegiatan Formatif/Sumatif, tanggal nyata dan TP yang diukur; saat diperlukan lihat/masukkan nilai tiap siswa per TP.
- **Input nilai:** tabel siswa ramah desktop dan layar lebih sempit, input angka jelas, simpan dengan validasi dan umpan balik; bedakan `Kosong` vs `0`; jangan kehilangan data diam-diam karena refresh HTMX.
- **ASTS/ASAS:** satu slot nilai awal per siswa/mapel/semester, opsi susulan, remedial, tampilkan nilai efektif dan histori nilai awal.
- **Remedial/Susulan:** dua tindakan berbeda di UI. Remedial memerlukan nilai awal sah; susulan tidak. Tanggal dan pelaksana tercatat.
- **Rekap nilai:** tampilkan rata-rata TP (2 desimal), NH, nilai efektif ASTS/ASAS, bobot resmi Kurikulum, nilai rekomendasi jika tersedia, dan alasan ketidaklengkapan jika tidak tersedia; tampilkan indikator ketercapaian.
- **Nilai akhir:** status per siswa, konfirmasi Final satu siswa atau massal yang memenuhi syarat, pengecualian hanya individual dengan nilai manual+alasan, penanda Perlu Ditinjau, histori Final.
- **Siswa pindahan:** daftar calon bukti asal dan sumber asli, pemetaan TP, pemilihan ASTS/ASAS sumber aktif, persetujuan pengakuan eksplisit, riwayat dan remedial lintas kelas tanpa mengganti catatan guru lama.
- **Koreksi khusus:** tampilkan pratinjau siswa/nilai/Final terdampak dan alasan, setelah dilaksanakan tampilkan daftar Final yang perlu ditinjau serta proses konfirmasi bertahap.

**Kualitas UI:** label dan pesan validasi bahasa Indonesia, status mudah dibedakan tanpa hanya warna, aksesibilitas sederhana (label input, fokus, tab order), layout responsif, tidak ada tombol palsu. Jangan membuat seluruh 40 skenario menjadi menu tersendiri; susun UI mengikuti pekerjaan guru yang normal.

## 5. METODE WAJIB: MIKRO-CHECKPOINT, MAKSIMAL 1–2 UAT PER PUTARAN

**Aturan utama: SATU perintah = SATU pekerjaan kecil = SATU atau DUA UAT yang berhubungan = BERHENTI.** Tidak ada izin implisit untuk mengerjakan seluruh Irisan A, apalagi seluruh Fase 10. Kontrak yang sudah disahkan adalah arah keseluruhan, bukan daftar tugas untuk dihabiskan dalam satu sesi.

Siklus untuk setiap checkpoint:

1. **BATASI CAKUPAN:** sebutkan kode UAT yang dikerjakan (maksimal dua). Pilih implementasi minimum yang benar-benar membuat kasus tersebut dapat dicoba dari browser oleh pengguna berwenang.
2. **IMPLEMENTASI MINIMAL:** kode backend, otorisasi server, tampilan, validasi dan migration *hanya jika benar-benar diperlukan* oleh 1–2 UAT itu. Boleh membuat fondasi kecil yang wajib bagi checkpoint, tetapi jangan diam-diam menambahkan fitur tahap lain.
3. **UJI TERFOKUS:** satu jalur berhasil dan satu jalur gagal terkait UAT aktif, dengan bukti otomatis/browser yang nyata. Jangan menjalankan seluruh paket tes besar berulang kali untuk perubahan kecil.
4. **SIAP UAT → STOP:** beri alamat preview, akun/role yang sesuai (tanpa rahasia), tiga hingga enam langkah klik, hasil yang diharapkan, serta daftar fitur yang **BELUM dikerjakan**.
5. **PENGGUNA UAT:** jika FAIL, hanya perbaiki yang gagal, tes ulang, lalu berhenti lagi. Jika PASS, catat PASS untuk checkpoint tersebut **tanpa langsung mulai checkpoint berikutnya**. Tunggu instruksi eksplisit semisal 'lanjut checkpoint berikutnya'.
6. **GIT:** patuhi instruksi checkpoint terbaru. Bila checkpoint mengizinkan publikasi untuk review independen sebelum UAT, commit dan push normal ke branch review setelah tes lulus; merge dan deploy tetap dilarang tanpa instruksi terpisah.

### Checkpoint yang diizinkan SEKARANG: CP10-01 — Pengaturan bobot dasar oleh Kurikulum

**Target hanya dua skenario kontrak:**

- **UAT-11 / AC-07 (bagian otorisasi):** Guru Mapel tidak dapat mengubah bobot sekolah, termasuk melalui request langsung ke server. Kurikulum berwenang.
- **UAT-12 / AC-07 dan AC-29 (validasi jumlah):** Kurikulum mencoba menyimpan bobot yang totalnya 110% → sistem menolak dengan pesan bahasa Indonesia dan tanpa menyimpan konfigurasi salah. Sertakan **smoke test satu penyimpanan valid 100%**, misalnya 60:20:20, sebagai pembuktian alur normal; angka itu hanya data contoh UAT, **bukan bobot baku sekolah**.

Cakupan minimum CP10-01: layar/form Kurikulum mengikuti UI yang sudah ada; tiga angka NH/ASTS/ASAS untuk tahun pelajaran dan semester; validasi total 100% di server; otorisasi server; penyimpanan aman, identitas konfigurasi/aktor serta audit minimum yang relevan, dan tes. Bobot seragam tingkat sekolah. **Jangan membangun sistem revisi bobot yang memengaruhi Final pada CP ini**, karena Fase 10 belum memiliki Final; desain penyimpanan tetap memungkinkan penambahan versi/riwayat di checkpoint lanjut. Jika ada dasar audit/versi yang sudah tersedia, gunakan kembali. Jika perlu migration, aditif dan ikuti protokol DB.

**Secara tegas BELUM boleh dikerjakan pada CP10-01:** CRUD TP, kegiatan Sumatif, input nilai siswa, ASTS/ASAS nilai siswa, remedial, kalkulator nilai akhir, Final, koreksi Final, pengakuan siswa pindahan, susulan, atau perubahan bobot setelah Final. Tidak boleh menjadikan 'persiapan dependensi' sebagai alasan mengimplementasikannya sekarang.

**Setelah CP10-01 dinyatakan PASS dan pengguna memberi perintah baru**, tentukan checkpoint kecil berikutnya (maksimal 1–2 UAT yang saling terkait), kemudian ulangi siklus. Urutan selanjutnya bisa TP → kegiatan → input nilai → remedial → ASTS/ASAS → Final → perpindahan, tetapi urutan ini bukan perintah eksekusi otomatis.

Jika audit menemukan fitur bobot dasar sudah tersedia, jangan menduplikasi; verifikasi dan perbaiki yang dibutuhkan untuk UAT-11/12 saja. Jika ada kebutuhan keputusan bisnis benar-benar baru, laporkan blocker secara spesifik; jangan menafsirkan kebijakan sendiri.

## 6. Quality gate, test, dan bukti yang wajib tersedia

### Sebelum setiap mikro-checkpoint UAT (HANYA yang relevan dengan 1–2 UAT aktif)

- Unit test **hanya untuk perilaku yang disentuh** dalam checkpoint aktif (CP10-01: otorisasi dan validasi/penyimpanan bobot).
- Integration test PostgreSQL **terfokus** pada migration/constraint/role/transaksi yang benar-benar disentuh oleh checkpoint aktif; jangan membuat atau menguji modul Final/konkurensi yang belum diimplementasi. Hanya di DB tes yang terverifikasi.
- Pengujian HTTP/handler/template/HTMX hanya untuk error, otorisasi, dan perilaku UI yang sudah ditambahkan pada checkpoint aktif.
- Lint/build/test terfokus mengikuti make target repo; `go test` yang relevan, `go vet` yang relevan, `git diff --check`, frontend build bila aset berubah. Jangan klaim PASS tanpa log/hasil nyata.
- Uji browser pada mode UAT sintetis di port yang disepakati; minimal penelusuran happy path dan satu negative path untuk UI yang baru. Sebutkan **mana yang diuji di browser** versus **mana yang hanya diperiksa integration test**.

### Sebelum Fase 10 dianggap selesai

- Pemetaan **AC-01...AC-29** dari dokumen kontrak ke test otomatis atau bukti UAT yang sesuai; catat gap nyata (tidak boleh diam-diam ditandai PASS).
- Checklist manual **UAT-01...UAT-40** untuk pengguna dengan langkah singkat, hasil yang diharapkan, dan kredensial/akun uji yang sesuai (jangan taruh password/token di laporan).
- Uji regresi Fase 1–9 yang terkait, khususnya akses penugasan, siswa pindah, absensi, Jurnal, kalender/semester, dan login yang ada.
- Final gate **full integration serial dan race detector** sesuai pola repo dan kapasitas environment, setelah UAT seluruh Fase 10 PASS; jangan menjalankan paket besar yang memakan waktu setiap perubahan UI kecil. Jika tes memakan lama, laporkan paket yang sedang berjalan dan progres nyata, bukan menganggap hang tanpa bukti.
- Build aplikasi, pemeriksaan migrasi terurut, `go vet`, frontend build jika digunakan, `git diff --check`, review kode yang berubah, bukti tidak ada migrasi destruktif/data hilang, dan working tree diketahui.
- Uji concurrency/anti-stale khusus konfirmasi Final dan perubahan bobot/sumber agar tidak terjadi dua Final aktif atau audit parsial.
- Dokumentasikan status **PASS/FAIL/BELUM DIUJI** secara jujur; jangan mengklaim pengujian pengguna sebagai PASS sebelum pengguna sendiri menyatakan demikian.

## 7. Pengelolaan preview dan UAT pengguna

Pada setiap mikro-checkpoint berisi 1–2 UAT:

1. Siapkan server lokal sehingga UI **betul-betul dapat diakses** melalui `http://127.0.0.1:18743` dengan identitas DB yang sudah diverifikasi dan dilaporkan (`sim_guru_ci_test` untuk contoh sintetis, atau `sim_guru_dev` untuk penggunaan nyata yang memang disetujui pengguna).
2. Jangan menggunakan guru/siswa riil sebagai fixture tes. Buat data UAT sintetis hanya pada lingkungan yang memang diperuntukkan baginya, melalui mekanisme resmi aplikasi/fixture tes yang aman.
3. Berikan **skenario klik singkat untuk pengguna**: role login, menu, urutan tindakan, contoh nilai, hasil yang diharapkan. Jelaskan mana yang bisa dicoba sekarang dan mana yang akan hadir di irisan berikutnya.
4. **Berhenti menunggu keputusan pengguna** pada checkpoint tersebut. Commit/push sebelum UAT hanya dilakukan jika instruksi terbaru checkpoint mengizinkannya; saat UAT berlangsung jangan menerbitkan perubahan baru tanpa instruksi. Bila pengguna menemukan masalah, perbaiki bagian gagal dan jalankan tes terfokus ulang.
5. Jika pengguna menyatakan checkpoint PASS, **catat PASS, lalu STOP**. **Jangan mulai checkpoint berikutnya tanpa perintah eksplisit pengguna.** PASS satu checkpoint tidak otomatis mengizinkan merge/deploy atau checkpoint selanjutnya.
6. Setelah sesi UAT sintetis selesai, pulihkan preview ke mode DEV sesuai pola proyek dan laporkan target DB; jangan tinggalkan port salah target tanpa penjelasan.

## 8. Gerbang commit, push, merge dan deploy

**Untuk setiap checkpoint:** ikuti instruksi publikasi pada perintah terbaru. Jika commit/push ke branch review diizinkan sebelum UAT, pastikan tes dan pemeriksaan diff lulus, tinjau file staged, lalu lakukan push normal agar supervisor dapat melakukan review kode independen. Jangan force-push. Merge dan deployment tetap memerlukan instruksi eksplisit.

**Untuk final publikasi atau integrasi setelah UAT yang disyaratkan:**

1. Lakukan final gate (tes/regresi/SQL/migrasi/review diff) yang relevan; jika FAIL, perbaiki dan lakukan UAT ulang bila perilaku berubah.
2. Verifikasi `git status`, branch kerja, diff, file sensitif, dan tidak ada test fixture/credential/dump/data pribadi ikut ter-commit.
3. Buat commit yang logis, dengan pesan jelas bahasa Indonesia atau sesuai konvensi repo; catat SHA.
4. `git push` biasa ke `origin` branch review yang secara eksplisit ditetapkan untuk pekerjaan; **bukan force push**. Verifikasi remote SHA dan working tree.
5. **Tidak** merge ke `main` atau branch dasar dan **tidak** deploy production tanpa instruksi terpisah dari pengguna.

## 9. Format laporan wajib dari Codex (ringkas, faktual)

Setiap checkpoint laporkan dalam bahasa Indonesia:

1. **Status:** `AUDIT TERFOKUS`, `IMPLEMENTASI LOKAL`, `SIAP UAT`, `UAT BELUM DITERIMA`, `UAT PASS BERDASARKAN PERNYATAAN PENGGUNA`, atau `FINAL GATE` sesuai kenyataan.
2. **Acuan:** path kontrak beserta status disahkan, dan kode AC yang tercakup.
3. **Yang dikerjakan:** fitur/perilaku user yang sudah berjalan, folder/file penting yang berubah, migration baru dan target DB bila ada.
4. **Yang diuji:** perintah+hasil ringkas, skenario browser yang betul-betul dicoba, keterangan PASS/FAIL/BELUM DIUJI.
5. **Perlindungan data:** DB aktual, backup sebelum migrasi dev (bila diterapkan), dampak terhadap data lama, dan verifikasi tidak ada destructive change.
6. **Cara UAT oleh pengguna:** tepat 1–2 skenario UAT aktif, menu/role, tiga sampai enam langkah praktis, hasil yang harus dilihat, alamat preview.
7. **Yang belum dikerjakan / blocker:** sebutkan fitur sengaja ditunda dan kode UAT yang belum dikerjakan; jangan klaim seluruh Fase 10 selesai.
8. **Git:** branch, `git status`, `HEAD`; laporkan commit/push sesuai izin checkpoint terbaru serta tegaskan tidak ada merge/deploy tanpa instruksi.

## 10. Instruksi eksekusi SEKARANG — HANYA CP10-01

**Jangan melakukan seluruh Fase 10 atau bahkan seluruh Irisan A.** Di `/home/fasih/Projects/sim_guru`:

1. Verifikasi repository, branch/HEAD, kondisi working tree, kontrak yang disahkan, role Kurikulum/Guru Mapel yang telah ada, pola migration, dan target database. Audit secukupnya untuk CP10-01.
2. Perbarui **hanya metadata status pengesahan** kontrak jika dokumen masih berstatus 'untuk pengesahan'; jangan edit keputusan bisnisnya.
3. Kerjakan **hanya pengaturan bobot dasar Kurikulum**, mencakup UAT-11 (Guru tidak boleh mengubah) dan UAT-12 (total 110% ditolak) beserta satu jalur penyimpanan valid 100%. Jangan membuat fitur penilaian lain.
4. Siapkan backend dan UI lokal yang benar-benar dapat dicoba; migration aditif dan aman hanya bila perlu; tes terfokus pada DB tes terverifikasi, kemudian pratinjau browser aman.
5. Sampaikan **SIAP UAT — CP10-01**: alamat preview, identitas database, akun/role, langkah singkat UAT-11 dan UAT-12, hasil otomatis yang telah dijalankan, serta daftar yang belum dikerjakan.
6. **STOP WAJIB.** Tidak ada langkah otomatis ke CP10-02. Tunggu pengguna mencoba dan memberi tanggapan. Jika PASS, tetap tunggu perintah 'lanjut' sebelum mengerjakan checkpoint berikutnya. Jika FAIL, perbaiki hanya masalah yang terkait.

**Jangan merge atau deploy. Commit/push hanya jika instruksi checkpoint terbaru mengizinkannya; jangan klaim fitur yang belum dibangun atau UAT pengguna yang belum dilakukan sebagai PASS.**
