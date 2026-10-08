package password

import (
	"slices"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		password string
		personal []string
		want     []Violation
	}{
		{name: "uzun ve özgün parola geçerli", password: "kahve fincanı mavi gökyüzü"},
		{name: "tam sınırda (10 karakter)", password: "abcdefghik"},
		{name: "kısa", password: "Kısa1!", want: []Violation{TooShort}},
		{name: "uzunluk karakterle ölçülür, byte'la değil", password: "ğüşıöçğüş", want: []Violation{TooShort}},
		{name: "çok uzun", password: strings.Repeat("a", 129), want: []Violation{TooLong}},
		{name: "yaygın parola", password: "password123", want: []Violation{TooCommon}},
		{name: "yaygın parola büyük harfle de yakalanır", password: "GALATASARAY1905", want: []Violation{TooCommon}},
		{name: "Türkçe büyük İ doğru küçültülür", password: "İSTANBUL1234", want: []Violation{TooCommon}},
		{
			name:     "öğrenci numarası içeriyor",
			password: "benim22290230parolam",
			personal: []string{"22290230", "Ahmet"},
			want:     []Violation{ContainsPersonalInfo},
		},
		{
			name:     "ad büyük/küçük harf farkıyla içeriyor",
			password: "ahmet-çok-güçlü-parola",
			personal: []string{"AHMET"},
			want:     []Violation{ContainsPersonalInfo},
		},
		{
			name:     "çok kısa kişisel bilgi dikkate alınmaz",
			password: "al-kahve-al-çay-123",
			personal: []string{"Al"},
		},
		{name: "birden çok ihlal birlikte", password: "123456", want: []Violation{TooShort, TooCommon}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Validate(tt.password, tt.personal...)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Validate(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}

func TestCommonListLoaded(t *testing.T) {
	if len(common) < 50 {
		t.Errorf("yaygın parola listesinde sadece %d kayıt var", len(common))
	}
	if _, ok := common["# sık kullanılan"]; ok {
		t.Error("açıklama satırları listeye girmemeli")
	}
}
