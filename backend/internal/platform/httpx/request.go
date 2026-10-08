package httpx

import "regexp"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidUUID, s'nin kanonik biçimde (8-4-4-4-12) bir UUID olup olmadığını söyler.
// Path parametrelerini veritabanına göndermeden önce doğrulamak için kullanılır.
func ValidUUID(s string) bool {
	return uuidPattern.MatchString(s)
}
