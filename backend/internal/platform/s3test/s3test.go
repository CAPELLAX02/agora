// Package s3test, entegrasyon testleri için gerçek ve geçici bir S3 uyumlu depo (SeaweedFS)
// sağlar.
package s3test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/CAPELLAX02/agora/backend/internal/files"
)

// Image, testlerde kullanılan SeaweedFS imajıdır. compose.yaml ile aynı sürüm olmalı.
const Image = "chrislusf/seaweedfs:4.48"

const config = `{"identities":[{"name":"test","credentials":[{"accessKey":"test","secretKey":"test_secret_key"}],` +
	`"actions":["Admin","Read","Write","List","Tagging"]}]}`

// New, Docker'da geçici bir SeaweedFS başlatır, kovayı oluşturur ve depoyu döndürür.
// Konteyner test bitince kapatılır. "go test -short" ile çalıştırıldığında test atlanır.
func New(t *testing.T) (*files.S3Store, files.S3Config) {
	t.Helper()
	if testing.Short() {
		t.Skip("entegrasyon testi: -short modunda atlanıyor")
	}
	ctx := context.Background()
	ctr, err := testcontainers.Run(ctx, Image,
		testcontainers.WithCmd("server", "-dir=/data", "-s3", "-s3.port=8333", "-s3.config=/etc/seaweedfs/s3.json",
			"-master.volumeSizeLimitMB=64", "-volume.max=0"),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			Reader: strings.NewReader(config), ContainerFilePath: "/etc/seaweedfs/s3.json", FileMode: 0o644,
		}),
		testcontainers.WithExposedPorts("8333/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("8333/tcp").WithStartupTimeout(time.Minute)),
		testcontainers.WithLogger(tclog.TestLogger(t)),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("s3test: SeaweedFS başlatılamadı: %v", err)
	}
	endpoint, err := ctr.PortEndpoint(ctx, "8333/tcp", "")
	if err != nil {
		t.Fatalf("s3test: adres alınamadı: %v", err)
	}
	cfg := files.S3Config{
		Endpoint: endpoint, PublicEndpoint: endpoint, Region: "us-east-1", Bucket: "agora-test",
		AccessKey: "test", SecretKey: "test_secret_key",
	}
	store, err := files.NewS3Store(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// S3 portu açıldıktan sonra filer'ın hazır olması birkaç saniye sürebilir.
	deadline := time.Now().Add(time.Minute)
	for {
		err = store.EnsureBucket(ctx)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("s3test: kova oluşturulamadı: %v", err)
	}
	return store, cfg
}
