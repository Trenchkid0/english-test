# Ruang Kata

Aplikasi latihan bahasa Inggris personal untuk persiapan IELTS. Satu proses Go menyajikan frontend SaaS, autentikasi akun, bank soal, dan penyimpanan perkembangan per pengguna di MySQL.

## Yang sudah tersedia

- Level CEFR A1–C2 dan target IELTS 4.0–9.0.
- Pendaftaran mandiri, login berbasis cookie aman, logout, dan isolasi data tiap pengguna.
- Grammar, vocabulary, reading, dan fill-in-the-blank dalam format pilihan ganda.
- Pilihan 5–30 soal dan timer 5–60 menit.
- Jawaban disimpan ke MySQL segera setelah dipilih.
- Sesi yang belum selesai dapat dilanjutkan dari riwayat.
- Sesi selesai menyimpan nilai, jawaban benar, alasan, dan tip IELTS untuk review ulang.
- Jawaban salah atau kosong dapat langsung dijadikan sesi latihan ulang dari halaman hasil.
- Review pintar menjadwalkan soal kembali dalam interval 1, 3, 7, dan 14 hari.
- Dashboard pribadi merangkum nilai nyata, aktivitas mingguan, dan tipe soal terlemah.
- Vocabulary notebook menyimpan kata, arti, contoh, collocation, dan tingkat penguasaan.
- IELTS Writing Task 1/2 dengan feedback empat kriteria; mode lokal tetap tersedia tanpa API key.
- Tes diagnostik singkat memetakan rentang latihan awal A2–B2.
- Reading toolkit menyorot bukti jawaban dan menyimpan penyebab kesalahan yang dirasakan pengguna.
- Tema terang/gelap tersimpan di browser.
- Mode bank hanya memilih soal yang sudah tersimpan dan tidak memanggil AI.
- Learner dapat memilih soal acak dari bank atau membuat paket baru dengan DeepSeek; paket baru otomatis disimpan ke bank untuk dipakai lagi.
- Bank soal awal tersedia untuk A1–C2 dan diisi melalui perintah seed eksplisit.
- Admin dapat menambah bank soal dengan DeepSeek; tanpa API key, sistem memakai paket lokal dan menghindari duplikat.
- Soal dari bank yang sudah pernah masuk ke sesi seorang pengguna tidak akan dipilih lagi untuk pengguna tersebut. Saat stok unik habis, aplikasi meminta penambahan soal baru alih-alih mengulang soal lama.

## Menjalankan

Persyaratan: Go 1.22+ dan akses ke MySQL 8/MariaDB yang mendukung kolom JSON.

1. Salin `.env.example` menjadi `.env`, lalu isi koneksi database. `DEEPSEEK_API_KEY` bersifat opsional untuk menambah variasi bank soal.
2. Unduh dependency, buat schema, dan isi bank soal satu kali:

```bash
go mod download
go run . migrate
go run . seed
go run . seed-cefr

# Seed fokus B1-C2, IELTS 5.0-9.0, 6 tipe, 500 soal per kombinasi
go run . seed-ielts

# Audit kuota, explanation, dan duplikasi context reading/listening
go run . audit-ielts

# Pilot enam soal lokal, lalu seed 216.000 soal yang resumable tanpa API
go run . pilot-ai-ielts
go run . seed-ai-ielts
go run . audit-ai-ielts
```

3. Build binary aplikasi:

```bash
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/english-practice .
```

4. Untuk pemakaian normal, jalankan binary—bukan `go run .`:

```bash
./bin/english-practice
```

5. Buka `http://localhost:8080`, lalu daftar. Akun pertama otomatis menjadi admin bank soal; akun selanjutnya menjadi learner.

Perintah `migrate` membuat database `DB_NAME`, menjalankan perubahan schema, dan mencatat versi migrasi. Perintah `seed` membaca JSON dalam `data/questions/`, lalu menjamin sedikitnya 500 soal aktif pada setiap kombinasi level A1–C2, target IELTS 4.0–9.0, dan tipe soal. Konteks reading/listening dinormalisasi (huruf kecil, angka menjadi `{number}`, dan spasi dirapikan); duplikat dinonaktifkan dan kekurangannya diisi kembali. Jalankan seed hanya saat data berubah atau ketika memakai database baru. Startup server hanya membuka koneksi dan memeriksa satu penanda versi schema—tidak lagi menjalankan migrasi atau seed.

Gunakan `go run . seed --fresh` (atau `./bin/english-practice seed --fresh`) untuk menghapus hanya data hasil generator `codex_local` lalu membuat ulang tepat 500 soal per sel. Data curated dan data pengguna tidak dihapus.

User database memerlukan izin membuat database/tabel ketika menjalankan `migrate`. Setelah migrasi selesai, proses server cukup diberi izin operasional pada schema aplikasi.

## Build untuk mini server

```bash
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/english-practice .
./bin/english-practice migrate
./bin/english-practice seed
./bin/english-practice
```

Frontend sudah di-embed ke binary. Untuk database baru, sertakan direktori `data/questions` ketika menjalankan `seed`. Setelah schema dan bank soal tersedia, runtime hanya membutuhkan binary dan `.env`; tidak ada build frontend atau database lokal di RAM.

Perintah yang tersedia:

| Perintah | Kegunaan | Kapan dijalankan |
| --- | --- | --- |
| `./bin/english-practice` atau `serve` | Menjalankan HTTP server | Setiap aplikasi dimulai |
| `./bin/english-practice migrate` | Membuat/memperbarui schema | Saat instalasi atau update schema |
| `./bin/english-practice seed` | Sinkronisasi 500 soal per level–target–tipe dan audit duplikasi | Saat instalasi atau data seed berubah |
| `./bin/english-practice seed-cefr` | Seed 30.000 soal unik: 1.000 per level CEFR–skill | Saat dataset CEFR berubah |
| `./bin/english-practice audit-cefr` | Verifikasi kuota, explanation/tip, dan duplikasi konteks | Setelah seed atau pemeriksaan berkala |
| `./bin/english-practice seed-ielts` | Seed 108.000 soal: B1–C2 × Band 5.0–9.0 × 6 tipe × 500 | Saat bank IELTS terarah perlu dilengkapi |
| `./bin/english-practice audit-ielts` | Audit 216 sel, explanation, dan konteks reading/listening | Setelah `seed-ielts` |
| `./bin/english-practice pilot-ai-ielts` | Generate satu soal lokal per tipe untuk pemeriksaan awal | Sebelum seed penuh |
| `./bin/english-practice seed-ai-ielts` | Tambah 216.000 soal lokal: 1.000 per kombinasi B1–C2 × IELTS 5.0–9.0 × 6 tipe, tanpa DeepSeek | Aman dilanjutkan ulang |
| `./bin/english-practice audit-ai-ielts` | Audit kuota korpus AI dan duplikasi context | Setelah seed AI penuh |

## Cara memakai pusat belajar

Gunakan ikon buku pada navigasi untuk membuka Ringkasan, Review, Kosakata, Writing, Diagnostik, dan Reading. Admin juga melihat tab Bank soal. Review terjadwal mulai terbentuk setelah sebuah sesi dinilai. Nilai writing dan diagnostik adalah estimasi belajar pribadi, bukan hasil resmi IELTS atau sertifikat CEFR.

## Environment

| Nama | Default | Keterangan |
| --- | --- | --- |
| `APP_ADDR` | `:8080` | Alamat HTTP server |
| `DB_HOST` | `127.0.0.1` | Host MySQL |
| `DB_PORT` | `3306` | Port MySQL |
| `DB_USER` | — | User MySQL, wajib |
| `DB_PASSWORD` | — | Password MySQL, wajib |
| `DB_NAME` | `english_practice` | Nama schema |
| `SEED_DATA_DIR` | `data/questions` | Direktori JSON untuk perintah `seed` |
| `DEEPSEEK_API_KEY` | — | Opsional; dipakai admin saat menambah soal dan untuk feedback writing |
| `DEEPSEEK_BASE_URL` | `https://api.deepseek.com` | Endpoint kompatibel OpenAI |
| `DEEPSEEK_MODEL` | `deepseek-chat` | Model generator |

## Catatan keamanan

Jangan commit `.env`. API key tidak pernah dikirim ke browser. Untuk server yang dapat diakses internet, letakkan aplikasi di belakang HTTPS agar cookie sesi memakai atribut `Secure`.
