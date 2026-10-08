package password

import (
	_ "embed"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Parola uzunluk sınırları (NIST SP 800-63B).
//
// Karmaşıklık kuralı ("en az bir büyük harf, bir rakam, bir sembol") bilinçli
// olarak yok: bu kurallar kullanıcıları "Parola1!" gibi tahmin edilebilir
// kalıplara iter. Bunun yerine uzunluk ve yaygın parola kontrolü yapılır.
const (
	MinLength = 10
	MaxLength = 128
)

// Violation, parola politikasının ihlal edilen bir kuralıdır. Değer istemciye
// makine okunur kod olarak gider, kullanıcıya gösterilecek metni frontend seçer.
type Violation string

// Politika ihlalleri.
const (
	TooShort             Violation = "TOO_SHORT"
	TooLong              Violation = "TOO_LONG"
	TooCommon            Violation = "TOO_COMMON"
	ContainsPersonalInfo Violation = "CONTAINS_PERSONAL_INFO"
)

//go:embed common.txt
var commonFile string

// common, yaygın parolaların kümesidir. Paket yüklenirken bir kez oluşturulur.
var common = parseList(commonFile)

// Validate, parolayı politikaya göre denetler ve ihlal edilen kuralları döndürür.
// Parola geçerliyse nil döner.
//
// personal, parolanın içermemesi gereken kişisel bilgilerdir: öğrenci numarası,
// ad, soyad, e-posta adresinin @ öncesi gibi. 3 karakterden kısa olanlar dikkate alınmaz.
func Validate(password string, personal ...string) []Violation {
	var violations []Violation

	n := utf8.RuneCountInString(password)
	if n < MinLength {
		violations = append(violations, TooShort)
	}
	if n > MaxLength {
		violations = append(violations, TooLong)
	}

	normalized := normalize(password)
	if _, ok := common[normalized]; ok {
		violations = append(violations, TooCommon)
	}

	for _, p := range personal {
		p = normalize(p)
		if utf8.RuneCountInString(p) >= 3 && strings.Contains(normalized, p) {
			violations = append(violations, ContainsPersonalInfo)
			break
		}
	}

	return violations
}

// normalize, metni Türkçe kurallarıyla küçük harfe çevirir ve kenar boşluklarını atar.
// strings.ToLower "İ" harfini "i̇" (i + birleşik nokta) yapar, Türkçe kuralında ise "i" olur.
func normalize(s string) string {
	return strings.ToLowerSpecial(unicode.TurkishCase, strings.TrimSpace(s))
}

// parseList, her satırda bir değer bulunan metni kümeye çevirir.
// Boş satırlar ve '#' ile başlayan açıklama satırları atlanır.
func parseList(text string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set[normalize(line)] = struct{}{}
	}
	return set
}
