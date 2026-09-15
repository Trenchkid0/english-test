# Question seed data

Direktori ini menjadi sumber data untuk perintah `go run . seed` atau
`./bin/english-practice seed`. Server tidak membaca file-file ini ketika mode
`serve` berjalan.

Dua belas file batch yang dikecualikan dari `.gitignore` adalah pengganti untuk
literal `curated_batch*.go` lama. Loader menerima nama field JSON camelCase dan
snake_case, memvalidasi empat pilihan, serta memastikan jawaban benar tersedia
sebelum data dimasukkan ke MySQL/MariaDB.

File `authentic_15000_final.json` dan `authentic_3000_exam_set.json` menjadi
sumber dasar perintah `seed-cefr`. Gabungannya berisi 18.000 soal atau 600 soal
untuk setiap kombinasi 6 level CEFR dan 5 skill. Pipeline memeriksa benturan
dengan database lalu menghasilkan tambahan 12.000 soal baru hingga hasil akhirnya
tepat 30.000 soal atau 1.000 per level–skill. Setiap record wajib memiliki
`explanation` dan `learningTip`. File `authentic_600_exam_set.json` tetap berupa
arsip generator dan tidak dibaca oleh seed aplikasi.
