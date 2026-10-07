-- Bu betik sadece volume ilk oluşturulduğunda (boş veri dizininde) bir kez çalışır.
-- OLTP veritabanı (agora) POSTGRES_DB ile zaten oluşturuluyor.
-- Analitik veri ambarı (Faz 8) için ayrı veritabanı:
CREATE DATABASE agora_dw OWNER agora;
