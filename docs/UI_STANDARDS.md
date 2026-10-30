# Standar UI SIM-GURU

Standar ini menerapkan keputusan final D-047 untuk semua role, modul yang ada, dan fitur frontend berikutnya. TailAdmin Free menjadi acuan visual; implementasi mengikuti pola SSR `html/template`, link biasa, dan htmx SIM-GURU.

## Sidebar

- Tampilkan Beranda dan menu/kelompok utama yang memang boleh diakses pengguna.
- Kelompok yang memiliki beberapa halaman tidak menampilkan daftar submenu dropdown. Klik parent membuka halaman pertama yang diizinkan.
- Kelompok satu halaman tetap berupa link langsung tanpa tab.
- Gabungan role tidak boleh menggandakan kelompok atau link. Parent tetap aktif pada halaman anak, termasuk detail, tambah, dan edit.
- Menu adalah kemudahan navigasi saja; handler server tetap memeriksa otorisasi.

## Horizontal navigation tabs

- Gunakan navigasi horizontal TailAdmin dengan ikon outline konsisten, label, teks biru dan underline biru pada link aktif.
- Setiap item adalah link ke route yang sudah ada atau route modul yang memang sedang dibangun. Navigasi URL biasa harus mendukung deep link, refresh, Back, dan Forward.
- Satu kelompok hanya menandai satu link aktif. Gunakan `aria-current="page"` hanya pada link yang mewakili halaman saat ini. Detail/tambah/edit mempertahankan tab induk yang tepat.
- Jangan memakai `role="tablist"`/`role="tab"`: ini adalah navigasi antarhalaman, bukan widget tab-panel. Link native tetap dapat difokuskan dan diaktifkan dengan keyboard; sediakan focus-visible yang jelas.
- Jangan tampilkan tab yang tidak berhak diakses. Role gabungan menghasilkan gabungan link yang sah tanpa duplikasi. Perubahan navigasi tidak memberi permission baru.

## Responsivitas dan aksesibilitas

- Desktop menampilkan tab dalam satu baris dengan jarak dan ukuran sentuh yang wajar.
- Pada layar sempit, bar tab dapat digeser horizontal; gunakan lebar maksimum kontainer dan cegah overflow pada halaman induk. Teks tetap satu baris dan ikon tidak menyusut/bertumpuk.
- Pastikan fokus keyboard terlihat, ikon dekoratif memiliki `aria-hidden="true"`, nama navigasi dapat dipahami, dan kontras state aktif/nonaktif memadai. Sidebar drawer tetap menggunakan kontrol aksesibel yang tersedia.

## Implementasi bersama

Komponen resmi berada di `internal/web/section_navigation.go`: kontrak `navigationTab`, pemetaan route/role ke kelompok dan item aktif, serta daftar tab yang diizinkan. Layout bersama memakai partial `section-navigation-tabs` di `internal/web/templates/page.html`; styling berada di `web/styles/app.css` dan dibangun ke aset embed `internal/web/static/app.css`. Perilaku scroll tab aktif pada navigasi biasa ada di `internal/web/static/app.js`.

Modul baru yang memiliki beberapa halaman mendaftarkan kelompok, tab, ikon, route, dan filter role pada kontrak bersama tersebut. Hindari pengulangan markup/CSS pada template fitur. Pemilihan tab bukan pemeriksaan keamanan dan tidak menggantikan middleware/otorisasi handler.

## Pengecualian

- Halaman tunggal tidak memerlukan tab.
- Form, dialog, wizard, filter, dan panel dalam satu halaman bukan navigasi tab.
- Banyak route dapat dibagi ke beberapa kelompok berdasarkan pekerjaan; jangan menambahkan tab bertingkat.
- Perubahan visual besar di luar navigasi tetap mengikuti proses desain dan keputusan produk yang berlaku.

## Definisi selesai navigasi fitur

1. Identifikasi role serta route yang benar-benar diizinkan dari handler dan service.
2. Tentukan kelompok utama dan halaman awal yang diizinkan.
3. Daftarkan semua tab yang sah pada komponen bersama; deduplikasi role gabungan.
4. Pastikan parent, active tab, route detail/tambah/edit, `aria-current`, dan perilaku keyboard benar.
5. Jalankan tes navigasi/otorisasi, build, dan `git diff --check`; periksa desktop dan ponsel.
