package password_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
)

func ExampleHasher() {
	h := password.NewHasher(password.DefaultParams, 4)
	ctx := context.Background()

	encoded, _ := h.Hash(ctx, "kahve fincanı mavi gökyüzü")
	fmt.Println(strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=2$"))
	fmt.Println(h.Verify(ctx, "kahve fincanı mavi gökyüzü", encoded))
	fmt.Println(h.Verify(ctx, "yanlış parola", encoded))
	// Output:
	// true
	// <nil>
	// password: parola eşleşmiyor
}

func ExampleValidate() {
	fmt.Println(password.Validate("kahve fincanı mavi gökyüzü"))
	fmt.Println(password.Validate("Kısa1!"))
	fmt.Println(password.Validate("GALATASARAY1905"))
	fmt.Println(password.Validate("ahmet-güçlü-parola", "22290230", "Ahmet"))
	// Output:
	// []
	// [TOO_SHORT]
	// [TOO_COMMON]
	// [CONTAINS_PERSONAL_INFO]
}
