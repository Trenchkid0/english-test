# Server Deployment Guide

File `english-practice` di folder ini adalah executable binary Linux (x86_64 / amd64, statically compiled) yang sudah membungkus backend Go dan seluruh file frontend (`web/`).

## Cara Menjalankan di Server:
1. Pastikan file memiliki izin eksekusi:
   ```bash
   chmod +x dist/english-practice
   ```
2. Pastikan file `.env` sudah ada di root project (atau buat dari `.env.example`):
   ```bash
   cp .env.example .env
   nano .env
   ```
3. Jalankan aplikasi:
   ```bash
   ./dist/english-practice
   ```
   Atau jalankan di background menggunakan PM2:
   ```bash
   pm2 start ./dist/english-practice --name "english-practice"
   ```
