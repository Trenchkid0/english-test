# Server Deployment Guide

Folder ini menyediakan binary Linux (statically compiled) yang sudah membungkus backend Go dan seluruh aset frontend (`web/`):
- `dist/english-practice` : Arsitektur **x86_64 / AMD64** (Intel/AMD)
- `dist/english-practice-arm64` : Arsitektur **ARM64 / AArch64** (Oracle Cloud Ampere, AWS Graviton, Raspberry Pi, dll)

## Cara Menjalankan di Server:
1. Pastikan file memiliki izin eksekusi:
   ```bash
   # Untuk server ARM64:
   chmod +x dist/english-practice-arm64

   # Untuk server AMD64 (x86):
   chmod +x dist/english-practice
   ```
2. Pastikan file `.env` sudah ada di root project:
   ```bash
   cp .env.example .env
   nano .env
   ```
3. Jalankan migrasi dan seed database (jika setup baru):
   ```bash
   ./dist/english-practice-arm64 migrate
   ./dist/english-practice-arm64 seed
   ```
4. Jalankan aplikasi di background menggunakan PM2:
   ```bash
   # Untuk server ARM64:
   pm2 start ./dist/english-practice-arm64 --name "english-practice"

   # Untuk server AMD64:
   pm2 start ./dist/english-practice --name "english-practice"
   ```
