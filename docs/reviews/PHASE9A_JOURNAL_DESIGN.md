# Fase 9A — Audit dan Rancangan Jurnal Mengajar

Status: audit aktual, kontrak bisnis FINAL D-048, dan cutover arsitektur FINAL D-049. Baseline yang diaudit: `7c9212437275da90f9c00377dc5eca412590422d` pada `review/sim-guru-checkpoint`. Tidak ada implementasi jurnal pada baseline ini.

## 1. Ringkasan keputusan

PO menetapkan D-048 FINAL untuk field, outcome Tidak Terlaksana, akses koreksi guru historis, review Draf pada revisi, identitas occurrence lintas revisi, blok JP snapshot, indikator netral, dan ekspor PDF/XLSX. D-049 menetapkan cutover: tidak ada backfill otomatis jadwal lama; slot aktif diberi ID baru dan kesinambungan revisi berikutnya diputuskan eksplisit. Rujuk [kontrak bisnis](../contracts/teaching-journal-contract.md). Fase ini belum mengubah kode/schema.

## 2. Audit implementasi aktual

### Jadwal dan penugasan

- `internal/academic/teaching.go`: operasi Kurikulum mengelola penugasan per semester/kelas/mapel/guru dengan `effective_from`/`effective_until`, validasi guru aktif-akun aktif-role `guru_mapel`, dan `teaching_assignment_history`. Koreksi penugasan memperbarui baris tetapi mencatat before/after; karena itu referensi FK saja tidak cukup untuk mempertahankan seluruh nilai historis yang pernah dikoreksi.
- `migrations/00008_teaching_schedule.sql`: `app.weekly_timetable_revisions` memiliki semester, nomor revisi, status Draf/Published, tanggal efektif, pelaku dan waktu publikasi; entri jadwal mengacu revision, class, subject, teacher, penugasan, weekday, dan rentang JP. `migrations/00010_curriculum_generator.sql` menambah `plan_snapshot` untuk snapshot aturan/jam jadwal.
- `app.weekly_timetable_periods` menyimpan waktu per nomor JP dan hari dengan kunci revision/hari/nomor (weekday ditambahkan oleh migration 00009). Pembuatan revision menyalin konfigurasi jam ke snapshot. Entry menyimpan `period_start/period_end`; pembaca menyusun segmen waktu dari baris JP revision. Sumber ini cocok untuk snapshot jurnal; jangan membaca ulang `app.lesson_periods` yang dapat dikoreksi.
- `internal/academic/timetable.go`: `PublishedTimetableForTeacher` mensyaratkan akses `guru_mapel`, identitas guru aktif yang ditautkan, semester aktif, dan revisi terbit efektif; lalu menyaring entri untuk guru tersebut. Ini tampilan jadwal aktif, bukan API occurrence historis yang siap dipakai jurnal.
- `internal/academic/lesson_attendance_context.go`: `TeacherLessonsOnDate` memvalidasi tanggal sipil, role/identitas guru aktif, penugasan efektif pada tanggal, rentang kalender semester, dan konteks jadwal efektif. Roster/absensi dibuka melalui pemeriksaan ulang yang ketat dan fingerprint agar form stale dapat ditolak.

### Absensi pelajaran dan frontend

- `internal/web/academic.go` menangani `GET /jadwal-saya` dengan jadwal terbit untuk Guru Mapel.
- `internal/web/lesson_attendance.go` menangani `GET/POST /jadwal-saya/absensi`, detail dan koreksi absensi. Jalur ini mencatat per siswa per pertemuan; bukan jurnal kegiatan mengajar.
- `migrations/00021_lesson_attendance.sql` menyimpan entry/revision/penugasan/semester/tanggal/siswa/kelas/mapel/guru/blok/waktu/snapshot dan histori status pelajaran. Skema tersebut menyediakan contoh snapshot dan koreksi optimistic-version, tetapi tidak boleh dipakai untuk menyimpan isi jurnal atau mencampur domain.
- `internal/web/section_navigation.go`, `templates/page.html`, dan `.navigation-*` menyediakan navigasi bersama D-047. Jadwal Saya dan Absensi Pelajaran sudah merupakan kelompok navigasi Guru Mapel; rancangan Jurnal Mengajar harus memakai kontrak navigasi bersama, bukan sistem tab baru.
- Pencarian daftar harus mengikuti Instant Search bersama HTMX dengan debounce server 350 ms; hasil dibatasi otorisasi server dan pagination. Tidak ada route/template/service jurnal spesifik pada source saat audit.

## 3. Hubungan domain yang diusulkan

Pisahkan jurnal dari `student_lesson_attendance`, presensi guru, dan catatan kasus. Model logis minimal:

```text
teaching_journals (identitas occurrence dan konteks jurnal terkini)
  └── teaching_journal_versions (Draf/Final/koreksi, versi immutable)
```

Kolom logis yang perlu dipertimbangkan (bukan skema final):

- jurnal: ID permanen; `logical_lesson_id`; entri jadwal/revision; penugasan; semester/tahun; tanggal WIB; class/subject/teacher IDs; snapshot label; waktu rencana dan outcome `held/not_held`; status Draf/Final dengan flag tampilan terpisah `needs_review`; version dan metadata pelaku/waktu.
- versi: journal ID; nomor versi; status/outcome; waktu aktual bila berbeda; materi ≤500 karakter; catatan opsional ≤2.000; alasan Tidak Terlaksana; pelaku; timestamp server; alasan koreksi; snapshot versi lengkap; idempotency token.
- snapshot blok: rekomendasikan tabel detail per versi `teaching_journal_version_periods(version_id, ordinal, weekday, period_no, starts_at, ends_at)` dengan kunci `(version_id, ordinal)` dan konsistensi weekday pada occurrence. Ini mempertahankan nomor JP berurutan serta waktu setiap JP tanpa mereduksi ke rentang awal/akhir; segmen jeda diturunkan dari gap waktu snapshot. Relasi ke revision/entry tetap pada versi jurnal.
- constraint: satu journal per `(logical_lesson_id, meeting_date)`, versi monoton, dan validasi Final menurut outcome. `not_held` wajib alasan tanpa isi yang menyatakan pembelajaran terlaksana; `held` wajib materi sebelum Final.

### Kunci occurrence stabil lintas revisi — FINAL D-048/D-049

`weekly_timetable_entries.id` mengidentifikasi satu baris jadwal, bukan kesinambungan slot lintas revision. Kunci occurrence jurnal adalah `(logical_lesson_id, meeting_date)` tanggal WIB. Kemiripan atribut entry tidak menghasilkan bukti lineage.

D-049 mengubah strategi dari rancangan backfill awal menjadi cutover: tidak mengisi logical ID pada seluruh jadwal historis. Pada cutover, setiap slot dari jadwal aktif menerima ID logical baru. Revisi mendatang menambahkan keputusan lineage eksplisit: satu entry lama dilanjutkan ke satu entry baru dengan ID sama, entry baru mendapat ID baru, atau entry lama dihentikan untuk occurrence setelah tanggal efektif transisi. Tidak ada penggunaan ulang ID yang dihentikan; identitas dan jurnal historis tetap tersimpan. Split: Kurikulum memilih paling banyak satu cabang untuk meneruskan ID lama, cabang lain slot baru. Merge: pilih satu lineage yang berlanjut dan hentikan sisanya, atau keluarkan ID baru; jangan gabungkan ID lama.

Legacy pre-cutover tetap utuh dan tidak mendapat logical ID otomatis. Jurnal baru untuk tanggal pra-cutover dilarang sampai ada mekanisme pemetaan historis yang diputuskan terpisah. Pengisian terlambat untuk tanggal sejak cutover diperbolehkan selama occurrence, identitas dan isi memenuhi kontrak. Karena jadwal, absensi dan presensi existing harus terus berjalan, lineage belum selesai bukan guard global untuk publication jadwal. Entry dengan mapping unresolved tidak tersedia untuk operasi jurnal; source timetable dan attendance tetap bekerja. Resolusi occurrence memilih revision published yang efektif untuk tanggal WIB tersebut, bukan revision terbaru saat ini. Setiap versi jurnal menyimpan snapshot sendiri, termasuk blok JP, revision, kelas/mapel/guru, dan waktu resmi.

Snapshot jurnal menyimpan ID revisi, entry ID, daftar `period_no` terurut dengan waktu mulai/selesai tiap JP, segmen jeda tampilan, kelas/mapel/pengampu serta penugasan. Saat ini entri jadwal hanya memodelkan rentang JP kontinu; daftar berurutan tetap mempertahankan urutan dan dapat mewakili blok non-kontigu bila model jadwal kelak mengizinkannya. Jangan mengarang JP yang tidak ada pada revisi. Waktu aktual disimpan terpisah hanya bila berbeda.

### Schema aditif yang diterapkan lokal pada fase 9B-01

Migration 00031–00034 memakai struktur berikut; domain jadwal lama tidak diubah. Migration 31–34 baru diuji pada `sim_guru_test`, bukan database development:

1. `app.teaching_journal_cutover_batches`: batch dengan tanggal kalender WIB, waktu server dan aktor. Setiap batch mencatat snapshot cutover secara atomik; indeks unik `(cutover_batch_id, semester_id)` membatasi satu transition cutover per semester dalam batch. Retry pada tanggal dan lingkup semester aktif yang sama mengembalikan batch yang sama. Migration 00034 menghapus batas satu batch global per tanggal agar lingkup semester yang berubah pada tanggal yang sama tidak berbenturan.
2. `app.logical_lessons`: ID permanen per semester, aktor dan waktu penciptaan. Trigger menolak update/delete; FK histori memakai RESTRICT.
3. `app.teaching_journal_lineage_transitions` dan `..._decisions`: transition mengikat revision sumber/tujuan, semester, tanggal efektif dan aktor/alasan; decisions mengikat composite FK entry+revision dan logical lesson+semester. Keputusan final append-only (`cutover`, `continue`, `new`, `stop`). `pending` tidak disimpan sebagai event yang dapat berkontradiksi: view `teaching_journal_lineage_status` menghitung revision terbit pasca-cutover yang belum memiliki transition lengkap. Deferred constraint trigger memeriksa cakupan penuh semua entry sumber/tujuan sekali saat commit.
4. `app.teaching_journals`: satu occurrence `(logical_lesson_id, meeting_date)` dengan UNIQUE constraint, tanggal sebagai `date` kalender WIB, owner guru historis dan snapshot revision/entry/assignment/kelas/mapel/guru. Composite FK revision-semester, entry-revision dan logical-semester bersifat RESTRICT. `current_version_no` hanya boleh bertambah satu dan FK deferred menunjuk versi yang ikut disimpan dalam transaksi.
5. `app.teaching_journal_versions`: row immutable per versi dengan state/outcome, materi/catatan, waktu aktual, alasan tidak terlaksana/koreksi, actor, waktu server, request key dan payload hash. Unique `(journal_id, version_no)` dan `(journal_id, request_key)`.
6. `app.teaching_journal_version_periods`: satu row per JP per versi, ordinal, nomor, waktu mulai/selesai dari `weekly_timetable_periods`. PK `(version_id, ordinal)` dan unique `(version_id, period_no)` mempertahankan urutan/snapshot.
7. `app.teaching_journal_history`: event `created`, `saved_draft`, `finalized`, `corrected_final` terikat ke versi dan aktor. Append-only. Deferred trigger mewajibkan setiap versi memiliki histori serta snapshot JP, dan actor harus owner jurnal.

Constraint aktif: unique source/target per transition dan unique `(to_revision_id, logical_lesson_id)`; FK composite entry+revision memakai unique yang tersedia sejak migration 00021. Trigger menolak penggunaan ulang ID `new/cutover`, continuation yang berbeda dari lineage sumber, serta pemakaian ID sesudah `stop`. Entry pra-cutover tidak memerlukan link. Resolusi atomic lewat advisory lock yang sama dengan publikasi jadwal; pending hanya menolak operasi jurnal. Pengujian race harus memastikan operasi jadwal tetap menggunakan semantik existing.

## 4. Service dan transaksi

Fase 9B-01 telah mengimplementasikan cutover, resolusi lineage, lookup occurrence, dan penulisan versi dasar. Daftar baca, form HTTP, riwayat UI, review flag, dan ekspor masih belum dibuat.

API service konseptual:

- `JournalMeetingsOnDate(actor, date, filters)` — daftar occurrence terbit sesuai penugasan bertanggal, read-only.
- `JournalForMeeting(actor, occurrenceID)` — baca konteks/snapshot dan jurnal; tolak jika tidak memiliki cakupan.
- `SaveJournalDraft(actor, occurrenceID, expectedVersion, payload, idempotencyKey)`.
- `FinalizeJournal(actor, journalID, expectedVersion, payload, idempotencyKey)`.
- `CorrectFinalJournal(actor, journalID, expectedVersion, reason, payload, idempotencyKey)`.
- `JournalHistory(actor, journalID)` dengan otorisasi identik pada setiap detail/version.

Setiap mutasi perlu satu transaksi: validasi role+akun+identitas+penugasan pada tanggal occurrence; kunci baris jurnal/occurrence; cek version/idempotency; validasi isi; tulis current version/event dan histori; commit. Jika perubahan konteks jadwal terjadi di antara render dan POST, kembalikan konflik yang dapat ditinjau; jangan mengubah occurrence atau submit otomatis. Retensi konten jurnal mengikuti kebijakan bukti akademik dan belum perlu menghapus isi/versi; retensi khusus belum ditentukan.

## 5. Otorisasi dan akses setelah perubahan penugasan

- Hak mutasi tidak disimpulkan dari role saja. Harus cocok identitas guru aktif, akun/role, penugasan resmi bertanggal, entri jadwal terbit, semester dan kalender.
- Jangan mengizinkan guru memasukkan ID kelas/mapel/guru lain untuk memilih cakupan; server menurunkan semuanya dari occurrence.
- Setelah penugasan berubah, occurrence ke depan mengikuti penugasan efektif baru; jurnal lama tetap mempunyai pemilik dan snapshot lama.
- FINAL D-048: guru historis/pemilik membaca dan mengoreksi jurnal miliknya selama akun dan role Guru Mapel yang diperlukan aktif. Guru baru tidak mewarisi mutasi. Guard menggunakan identitas/pemilik historis, bukan penugasan saat ini. Jangan menyalin D-037 secara otomatis.
- Akses raw detail role lain tidak diperluas otomatis. Indikator hanya mengikuti hak atas sumber masing-masing. Ekspor agregat lintas guru perlu keputusan akses terpisah; Jurnal Saya masuk D-048.

## 6. Rancangan halaman dan ekspor

- **Jadwal Saya:** tautan “Isi/Lanjutkan Jurnal” per occurrence bertanggal yang sah; pisahkan status jadwal dan jurnal. Tampilkan kelas, mapel, guru, revisi, nomor JP terurut dan waktu per JP sebagai konteks resmi baca-saja.
- **Jurnal Mengajar:** kelompok D-047 dengan tab Jurnal Saya dan Riwayat Koreksi jika dipisah menjadi halaman. Daftar SSR berisi tanggal WIB, kelas, mapel, urutan JP, status Draf/Final, indikator Perlu Ditinjau, waktu update dan outcome. Instant Search/filter tahun, semester, tanggal, kelas, mapel, status; pagination mengikuti pola aplikasi.
- **Form:** konteks jadwal/penugasan baca-saja; materi maks. 500, catatan opsional maks. 2.000. Waktu aktual terpisah dan hanya diisi jika beda. Draf boleh belum lengkap. Penanda Tidak Terlaksana memerlukan alasan; saat Final alasan wajib dan form tidak menyajikan materi/waktu aktual seolah pembelajaran terlaksana. Submit POST ber-CSRF.
- **Perlu Ditinjau:** revisi terbit yang mengubah occurrence terkait memberi flag terpisah, tidak mengubah status Draf/Final, isi, pemilik, snapshot atau ID jurnal. Guru meninjau dan mengonfirmasi tindakan; tidak ada pemindahan data otomatis.
- **Detail/riwayat:** tampilkan snapshot semua nomor JP dan waktunya; waktu aktual; outcome; status; versi, alasan koreksi, pelaku dan waktu WIB. Koreksi hanya muncul untuk pemilik historis sesuai guard D-048.
- **Ekspor Jurnal Saya:** dua link/download PDF dan XLSX membawa filter saat ini ke seluruh hasil, bukan halaman aktif. PDF memuat nomor, tanggal WIB, kelas, mapel, JP, materi, catatan, outcome/status dan alasan Tidak Terlaksana; ulangi header di setiap halaman dan wrap teks. XLSX satu baris per jurnal dengan header, tanggal WIB, daftar JP terurut, materi/catatan/outcome/status dan label Draf. Role baca lain tidak otomatis dapat ekspor agregat.
- **Keamanan ekspor:** query server menerapkan cakupan pengguna/filter yang sama dengan daftar; jangan percaya filter ID kelas/mapel tanpa memvalidasi cakupannya. Gunakan Excelize yang sudah ada pada repository dengan `SetCellStr` untuk nilai pengguna. Uji string berawalan formula. PDF: rekomendasi pustaka Go `github.com/signintech/gopdf`; pin versi, gunakan font Unicode lokal yang dilisensikan dan uji wrapping/paginasi/memori sebelum adopsi. Dependency belum ditambahkan.
- **Mobile:** form satu kolom, tombol terjangkau, hasil daftar berupa kartu atau tabel responsif, histori/ekspor tidak memaksa overflow. Ikuti D-047 dan komponen TailAdmin.

## 7. Matriks skenario dan tes

1. Penugasan resmi, revisi dan kalender efektif mengizinkan Guru Mapel pemilik; guru lain, role gabungan tanpa penugasan, identitas tak sah dan URL/ID silang ditolak.
2. Draf belum lengkap dapat disimpan/dilanjutkan; materi dan catatan tervalidasi batas karakter yang disetujui.
3. Final terlaksana mensyaratkan materi ≤500; catatan ≤2.000 opsional; waktu aktual terpisah hanya bila berbeda.
4. Draf/Final Tidak Terlaksana memerlukan alasan saat marker disimpan/final; Final tidak menyatakan ada materi/waktu pelaksanaan.
5. Snapshot nomor JP/waktu per revisi tepat saat lintas istirahat; perubahan `lesson_periods` atau revisi berikutnya tidak mengubah data historis.
6. Uji lineage migration: mapping satu-ke-satu yang pasti, slot baru, kandidat ambigu, mapping duplikat, split/merge, revisi jadwal sebelum/sesudah jurnal; ambiguitas tidak diputuskan otomatis dan konflik mencegah duplikasi.
7. Finalisasi/koreksi atomik; alasan wajib; expected-version usang menghasilkan konflik aman; histori before/after tidak hilang.
8. Guru historis yang akun+role aktif dapat baca/koreksi miliknya tanpa penugasan aktif; guru baru tidak dapat mutasi jurnal lama; role lain tidak memperoleh akses/export agregat otomatis.
9. Revisi menandai Draf impacted sebagai Perlu Ditinjau tanpa mengganti snapshot/pemilik/occurrence; Final tidak berubah.
10. GET tidak membuat journal; presensi guru, absensi harian/per-pelajaran tidak membuat/mengubah journal atau outcome.
11. Anomali menampilkan perbedaan saja kepada pembaca yang berwenang pada masing-masing sumber, tanpa kesimpulan pelanggaran atau mutasi data.
12. Idempotency dan race test memastikan satu journal untuk `(logical_lesson_id, meeting_date)` serta tidak menggandakan histori.
13. Ekspor seluruh hasil filter lintas pagination, data Draf diberi label, tanggal WIB; XLSX kasus formula injection disimpan sebagai teks; PDF Unicode, wrapping, pergantian halaman, dan cakupan ekspor diuji.
14. Instant Search, filter, pagination, CSRF, keyboard, akses URL langsung, mobile dan role gabungan diuji.
15. Migration/integration hanya memakai `sim_guru_test`, bukan development/UAT.

## 8. Urutan implementasi lanjutan

1. Lengkapi tes PostgreSQL untuk FK pasangan tidak cocok, split/merge, stop/reuse, serta pemilihan revision efektif historis ketika revision yang lebih baru telah terbit.
2. Uji snapshot JP tetap stabil terhadap revisi jadwal dan perubahan konfigurasi jam; tambah regression attendance lintas modul dengan fixture jurnal.
3. Implementasikan pembaca occurrence dan daftar Jurnal Saya dengan filter/Instant Search, pagination, serta cakupan owner/penugasan yang sudah ditetapkan.
4. Tambahkan handler SSR/CSRF dan form TailAdmin untuk Draf, Final, koreksi Final, histori versi, serta indikator Perlu Ditinjau; pertahankan kontrak transaksi service.
5. Implementasikan PDF/XLSX Jurnal Saya dengan cakupan filter penuh, formula-safe XLSX dan layout PDF Unicode.
6. Tambahkan indikator anomali read-only sesuai sumber dan izin; jangan mengubah domain lain.
7. Jalankan Go/unit/integration/race/vet/make/npm checks; migration/integration hanya pada `sim_guru_test`; siapkan UAT Guru Mapel. Ekspor agregat role lain menunggu keputusan akses tersendiri.

Cutover menyelesaikan kebutuhan backfill lineage jadwal historis dengan tidak melakukan backfill. Jurnal pra-cutover ditolak sampai mekanisme pemetaan historis diputuskan terpisah; pengisian terlambat sejak cutover diperbolehkan. Uji fondasi sudah membuktikan uniqueness occurrence dan snapshot tiap versi pada fixture; cakupan edge case yang tersisa dicatat di HANDOVER.

### Pengujian PostgreSQL wajib untuk cutover dan lineage

Jalankan hanya pada `sim_guru_test`, serial dengan database bersama. Migration harus membuktikan:

1. Cutover memakai tanggal WIB yang tercatat, memilih tepat satu revision published efektif per semester dalam cakupan, memberi ID baru sekali ke setiap slot, mempertahankan entry/revision historis pra-cutover tanpa backfill, dan aman terhadap retry. Semester tanpa revision efektif tidak mendapat slot hasil tebakan.
2. Simulasikan publikasi revision serentak dengan cutover. Cutover harus melihat satu keadaan konsisten per semester (publikasi selesai sebelum snapshot atau berada sesudahnya), tanpa seed setengah revision atau menghentikan operasi jadwal lebih luas dari lock transaksi singkat yang diperlukan.
3. Constraint FK menolak lineage ke entry/revision yang tidak cocok; nullability `cutover/continue/new/stop` tervalidasi; unique source/target per transisi mencegah percabangan/penggandaan keputusan; satu logical ID tidak muncul dua kali dalam satu revision.
4. `continue` mempertahankan logical ID saat jam/hari/guru/kelas/mapel berubah hanya setelah keputusan Kurikulum eksplisit; `new` memperoleh ID berbeda walau semua atribut mirip; `stop` memensiunkan ID dan trigger/constraint menolak pemakaian ulang.
5. Split/merge: tanpa keputusan, mapping tetap pending dan jurnal terkait gagal tertutup; pemetaan eksplisit satu-ke-satu sesuai pilihan Kurikulum berhasil; tidak ada dua ID lama yang digabung. Resolusi menyimpan event audit dan tidak menghapus record pending sebelumnya.
6. Publikasi jadwal dan seluruh tes regresi jadwal/absensi tetap lulus saat lineage unresolved; absensi lama tetap terikat pada FK entry/revision serta snapshot semula.
7. Dua transaksi bersamaan membuat jurnal occurrence yang sama: unique `(logical_lesson_id, meeting_date)` menghasilkan satu journal saja; request yang kalah mendapat conflict yang aman. Simpan versi stale tidak menimpa versi terbaru.
8. Trigger kegagalan pada insert histori/version memicu rollback atomik tanpa journal/version/JP parsial. Simpan histori dan child JP satu transaksi.
9. Versi jurnal menyimpan nomor/waktu setiap JP; revisi jadwal baru dan perubahan `lesson_periods` tidak mengubah snapshot. Snapshot JP berurutan melintasi waktu istirahat.

Tes migration memakai database kosong/fixture terisolasi sesuai harness repository; fixture schedule lineage historis dibuat di `sim_guru_test`, bukan `sim_guru_dev`. Tidak menggunakan SQLite untuk menguji constraint PostgreSQL.
