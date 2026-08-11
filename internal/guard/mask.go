package guard

import (
	"strings"
	"unicode"
)

type maskMode int

const (
	maskKeywords maskMode = iota
	maskRelations
)

func mask(sql string) (string, int) {
	masked, statements, _ := maskSQL(sql, maskKeywords)
	return masked, statements
}

func relationMask(sql string) string {
	masked, _, _ := maskSQL(sql, maskRelations)
	return masked
}

func maskSQL(sql string, mode maskMode) (string, int, int) {
	var b strings.Builder
	statements := 1
	sawCodeAfterSemicolon := true
	pendingSemicolon := false
	trailingStart := -1
	markCode := func() {
		if pendingSemicolon {
			statements++
			pendingSemicolon = false
			trailingStart = -1
		}
		sawCodeAfterSemicolon = true
	}

	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			depth := 1
			i += 2
			for i < len(sql) && depth > 0 {
				if sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*' {
					depth++
					i += 2
				} else if sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/' {
					depth--
					i += 2
				} else {
					i++
				}
			}
			i--
			b.WriteByte(' ')
			continue
		}
		if c == '\'' {
			markCode()
			escapeString := i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') && (i == 1 || !isWordChar(sql[i-2]))
			i++
			for i < len(sql) {
				if escapeString && sql[i] == '\\' && i+1 < len(sql) {
					i += 2
					continue
				}
				if sql[i] == '\'' {
					if i+1 < len(sql) && sql[i+1] == '\'' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			b.WriteString("''")
			continue
		}
		if c == '"' {
			markCode()
			i++
			if mode == maskRelations {
				b.WriteByte(' ')
			}
			for i < len(sql) {
				if sql[i] == '"' {
					if i+1 < len(sql) && sql[i+1] == '"' {
						if mode == maskRelations {
							b.WriteByte('"')
						}
						i += 2
						continue
					}
					break
				}
				if mode == maskRelations {
					b.WriteByte(sql[i])
				}
				i++
			}
			if mode == maskRelations {
				b.WriteByte(' ')
			} else {
				b.WriteString(`"x"`)
			}
			continue
		}
		if c == '$' {
			if tag, ok := dollarTag(sql, i); ok {
				markCode()
				end := strings.Index(sql[i+len(tag):], tag)
				if end < 0 {
					i = len(sql)
				} else {
					i = i + len(tag) + end + len(tag) - 1
				}
				b.WriteByte(' ')
				continue
			}
		}
		if c == ';' {
			if sawCodeAfterSemicolon {
				pendingSemicolon = true
				trailingStart = i
			}
			sawCodeAfterSemicolon = false
			b.WriteByte(' ')
			continue
		}
		if !unicode.IsSpace(rune(c)) {
			markCode()
		}
		b.WriteByte(c)
	}
	return b.String(), statements, trailingStart
}

func dollarTag(sql string, i int) (string, bool) {
	if i > 0 && (isWordChar(sql[i-1]) || sql[i-1] == '$') {
		return "", false
	}
	if i+1 >= len(sql) {
		return "", false
	}
	if sql[i+1] == '$' {
		return "$$", true
	}
	if !isIdentifierStart(sql[i+1]) {
		return "", false
	}
	for j := i + 2; j < len(sql); j++ {
		if sql[j] == '$' {
			return sql[i : j+1], true
		}
		if !isWordChar(sql[j]) {
			return "", false
		}
	}
	return "", false
}

func isIdentifierStart(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
