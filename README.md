# Agora - Üniversite Kampüs Platformu ve Bilgi Sistemi

Ankara Üniversitesi'nin Öğrenci Bilgi Sistemi (OBS) ve e-Kampüs sistemlerini tek bir modern, bütünleşik ve güvenli platformda birleştiren bitirme projesi.

> Proje detayları, mimari ve yol haritası ileride buraya eklenecektir.

## Monorepo Yapısı

| Dizin                  | Açıklama                                                      |
| ---------------------- | ------------------------------------------------------------- |
| [`backend/`](backend/) | Go ile yazılan REST API (PostgreSQL, Redis)                   |
| [`web/`](web/)         | Vite + React + TypeScript + shadcn/ui + Tailwind CSS + Redux Toolkit |
| [`mobile/`](mobile/)   | React Native + Expo (file-based routing) + TypeScript + NativeWind |
