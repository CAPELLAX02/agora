package files

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Store, dosya içeriklerinin tutulduğu nesne deposudur.
type Store interface {
	// Put, içeriği akışla yazar; boyut bilinmiyorsa -1.
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// PresignGet, nesneyi ttl süresince indirtecek imzalı URL'dir. disposition indirmede
	// dönen Content-Disposition'dır ve imzaya dahildir: değiştirilemez.
	PresignGet(ctx context.Context, key, disposition string, ttl time.Duration) (string, error)
	Remove(ctx context.Context, key string) error
}

// S3Config, S3 uyumlu deponun ayarlarıdır.
type S3Config struct {
	Endpoint       string // API ve worker'ın eriştiği host:port
	PublicEndpoint string // imzalı URL'lerin host:port'u (tarayıcının eriştiği)
	UseSSL         bool
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
}

// S3Store, S3 uyumlu bir depodur (SeaweedFS, bulut S3 ...).
type S3Store struct {
	client *minio.Client // içerik yazma ve silme
	public *minio.Client // sadece imza: ağa çıkmaz, URL'ler tarayıcının adresiyle üretilir
	bucket string
}

// NewS3Store, depoya bağlanan bir S3Store oluşturur. Bağlantı ilk istekte kurulur.
func NewS3Store(c S3Config) (*S3Store, error) {
	opts := func(endpoint string) (*minio.Client, error) {
		// Bölge verilir: aksi halde istemci imzadan önce depoya bölgesini sorar.
		return minio.New(endpoint, &minio.Options{
			Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""), Secure: c.UseSSL, Region: c.Region,
		})
	}
	client, err := opts(c.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("files: S3 istemcisi: %w", err)
	}
	public, err := opts(c.PublicEndpoint)
	if err != nil {
		return nil, fmt.Errorf("files: S3 istemcisi: %w", err)
	}
	return &S3Store{client: client, public: public, bucket: c.Bucket}, nil
}

// EnsureBucket, kova yoksa oluşturur.
func (s *S3Store) EnsureBucket(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("files: kova denetlenemedi: %w", err)
	}
	if ok {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("files: kova oluşturulamadı: %w", err)
	}
	return nil
}

func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *S3Store) PresignGet(ctx context.Context, key, disposition string, ttl time.Duration) (string, error) {
	params := url.Values{}
	params.Set("response-content-disposition", disposition)
	u, err := s.public.PresignedGetObject(ctx, s.bucket, key, ttl, params)
	if err != nil {
		return "", fmt.Errorf("files: imzalı URL üretilemedi: %w", err)
	}
	return u.String(), nil
}

func (s *S3Store) Remove(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
