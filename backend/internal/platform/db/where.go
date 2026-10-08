package db

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var placeholder = regexp.MustCompile(`\$(\d+)`)

// Where, dinamik bir WHERE ifadesini ve konumsal parametrelerini güvenli biçimde biriktirir.
//
// Her koşul kendi içinde $1, $2... ile yazılır. Add, bu numaraları o ana kadar
// eklenmiş parametre sayısına göre kaydırır. Böylece koşullar hangi sırayla
// eklenirse eklensin parametre numaraları çakışmaz. Kullanıcı girdisi asla
// sorgu metnine girmez, sadece args üzerinden gönderilir.
type Where struct {
	conds []string
	args  []any
}

// Add, bir koşul ve o koşulun parametrelerini ekler.
//
//	w.Add("f.unit_type = $1", "FACULTY")
//	w.Add("(f.code ILIKE $1 OR f.name_tr ILIKE $1)", "%muh%")  // aynı parametre iki kez
//	w.Add("(p.name_tr, p.id) > ($1, $2)", name, id)            // iki parametre
func (w *Where) Add(cond string, args ...any) {
	offset := len(w.args)
	cond = placeholder.ReplaceAllStringFunc(cond, func(m string) string {
		n, _ := strconv.Atoi(m[1:]) // desen sadece rakamla eşleştiği için hata olamaz
		return "$" + strconv.Itoa(offset+n)
	})
	w.conds = append(w.conds, cond)
	w.args = append(w.args, args...)
}

// Arg, koşul dışında kullanılacak (ör. LIMIT) bir parametre ekler ve yer tutucusunu döndürür.
func (w *Where) Arg(v any) string {
	w.args = append(w.args, v)
	return fmt.Sprintf("$%d", len(w.args))
}

// SQL, koşullar varsa "WHERE a AND b" ifadesini, yoksa boş string döndürür.
func (w *Where) SQL() string {
	if len(w.conds) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(w.conds, " AND ")
}

// Args, sorguya gönderilecek parametreleri sırasıyla döndürür.
func (w *Where) Args() []any {
	return w.args
}
