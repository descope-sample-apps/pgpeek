// Package guard is an app-layer read-only guardrail. It is NOT the security
// boundary — the read-only DB role (descoperead) is. This rejects obvious
// mistakes: multiple statements, DML/DDL, and anything that isn't a single
// SELECT/WITH/VALUES/TABLE statement, so a fat-fingered query fails fast and
// clearly instead of surprising someone.
package guard

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrEmpty is returned for blank input.
var ErrEmpty = errors.New("query is empty")

// forbidden keywords that must never appear in a read-only query (matched as
// whole words, against a version of the SQL with comments and string-literal
// contents removed so they can't trigger false positives).
var forbidden = []string{
	"INSERT", "UPDATE", "DELETE", "MERGE", "UPSERT",
	"DROP", "ALTER", "CREATE", "TRUNCATE", "RENAME",
	"GRANT", "REVOKE", "COPY", "CALL", "DO",
	"VACUUM", "ANALYZE", "REINDEX", "REFRESH", "CLUSTER",
	"LOCK", "SET", "RESET", "DISCARD", "PREPARE", "EXECUTE",
	"DEALLOCATE", "LISTEN", "NOTIFY", "COMMENT", "SECURITY",
}

// allowed leading keywords for a read-only statement.
var allowedStart = map[string]bool{
	"SELECT":  true,
	"WITH":    true,
	"VALUES":  true,
	"TABLE":   true,
	"EXPLAIN": true,
}

// forbiddenRelations are sensitive system catalogs that expose credentials or
// host topology (config-file paths, HBA rules). The DB role *should* already
// deny these, but the app blocks them too as defense in depth. Matched as whole
// words against the masked SQL, so they can't be hidden in strings/comments.
var forbiddenRelations = []string{
	"PG_SHADOW", "PG_AUTHID", "PG_HBA_FILE_RULES",
}

// Validate returns nil if sql is a single read-only statement, or a
// human-readable error explaining why it was rejected.
func Validate(sql string) error {
	masked, statements := mask(sql)
	masked = strings.TrimSpace(masked)
	if masked == "" {
		return ErrEmpty
	}
	if statements > 1 {
		return errors.New("only a single statement is allowed (remove extra ';' / multiple queries)")
	}

	// First significant keyword, skipping leading '(' for wrapped unions.
	first := firstKeyword(masked)
	if !allowedStart[first] {
		if first == "" {
			return ErrEmpty
		}
		return fmt.Errorf("only read-only queries are allowed; statement starts with %q, not SELECT/WITH/VALUES/TABLE", first)
	}

	upper := strings.ToUpper(masked)
	for _, kw := range forbidden {
		if containsWord(upper, kw) {
			return fmt.Errorf("query contains disallowed keyword %q — this tool is read-only", kw)
		}
	}
	for _, rel := range forbiddenRelations {
		if containsWord(upper, rel) {
			return fmt.Errorf("query references restricted system catalog %q", strings.ToLower(rel))
		}
	}
	return nil
}

// firstKeyword returns the first SQL keyword (uppercased), skipping any leading
// open parentheses and whitespace.
func firstKeyword(s string) string {
	i := 0
	for i < len(s) && (s[i] == '(' || unicode.IsSpace(rune(s[i]))) {
		i++
	}
	start := i
	for i < len(s) && (isWordChar(s[i])) {
		i++
	}
	return strings.ToUpper(s[start:i])
}

// containsWord reports whether word (already uppercase) appears in s (already
// uppercase) as a whole word — bounded by non-word characters on both sides.
func containsWord(s, word string) bool {
	from := 0
	for {
		idx := strings.Index(s[from:], word)
		if idx < 0 {
			return false
		}
		i := from + idx
		beforeOK := i == 0 || !isWordChar(s[i-1])
		end := i + len(word)
		afterOK := end >= len(s) || !isWordChar(s[end])
		if beforeOK && afterOK {
			return true
		}
		from = i + 1
	}
}

func isWordChar(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// mask walks the SQL once, removing comments and the *contents* of string
// literals (single-quoted and dollar-quoted) and quoted identifiers, so keyword
// scanning can't be fooled by data. It returns the masked SQL and the number of
// top-level statements (semicolons outside strings/comments that are followed by
// more non-whitespace input count as additional statements).
func mask(sql string) (string, int) {
	var b strings.Builder
	statements := 1
	sawCodeAfterSemicolon := true // start: first statement counts when code appears
	pendingSemicolon := false

	n := len(sql)
	for i := 0; i < n; i++ {
		c := sql[i]

		// line comment
		if c == '-' && i+1 < n && sql[i+1] == '-' {
			for i < n && sql[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
			continue
		}
		// block comment (PostgreSQL allows nesting)
		if c == '/' && i+1 < n && sql[i+1] == '*' {
			depth := 1
			i += 2
			for i < n && depth > 0 {
				if sql[i] == '/' && i+1 < n && sql[i+1] == '*' {
					depth++
					i += 2
				} else if sql[i] == '*' && i+1 < n && sql[i+1] == '/' {
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
		// single-quoted string
		if c == '\'' {
			i++
			for i < n {
				if sql[i] == '\'' {
					if i+1 < n && sql[i+1] == '\'' { // escaped quote
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
		// double-quoted identifier
		if c == '"' {
			i++
			for i < n {
				if sql[i] == '"' {
					if i+1 < n && sql[i+1] == '"' {
						i += 2
						continue
					}
					break
				}
				i++
			}
			b.WriteString(`"x"`)
			continue
		}
		// dollar-quoted string: $tag$ ... $tag$
		if c == '$' {
			if tag, ok := dollarTag(sql, i); ok {
				end := strings.Index(sql[i+len(tag):], tag)
				if end < 0 {
					i = n
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
			}
			sawCodeAfterSemicolon = false
			b.WriteByte(' ')
			continue
		}

		if !unicode.IsSpace(rune(c)) {
			if pendingSemicolon {
				statements++
				pendingSemicolon = false
			}
			sawCodeAfterSemicolon = true
		}
		b.WriteByte(c)
	}
	return b.String(), statements
}

// dollarTag returns the dollar-quote tag (e.g. "$$" or "$foo$") starting at i.
// The caller guarantees sql[i] == '$'.
func dollarTag(sql string, i int) (string, bool) {
	j := i + 1
	for j < len(sql) {
		c := sql[j]
		if c == '$' {
			return sql[i : j+1], true
		}
		if !isWordChar(c) {
			return "", false
		}
		j++
	}
	return "", false
}
