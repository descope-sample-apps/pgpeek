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

var ErrCountExplain = errors.New("count is not supported for EXPLAIN queries")

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

// IsRestrictedRelation reports whether name is one of the sensitive catalogs
// pgpeek blocks as defense in depth across SQL and table-browse endpoints.
func IsRestrictedRelation(name string) bool {
	for _, rel := range forbiddenRelations {
		if strings.EqualFold(name, rel) {
			return true
		}
	}
	return false
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
	first, after := firstKeyword(masked)
	if !allowedStart[first] {
		if first == "" {
			return ErrEmpty
		}
		return fmt.Errorf("only read-only queries are allowed; statement starts with %q, not SELECT/WITH/VALUES/TABLE", first)
	}

	// EXPLAIN's options (ANALYZE, VERBOSE, BUFFERS, ...) are not part of the
	// explained statement, so they are left out of the keyword scan: EXPLAIN
	// ANALYZE does run the statement, which makes it exactly as read-only as
	// its target, and the target is scanned like any other query.
	body := masked
	if first == "EXPLAIN" {
		target, err := explainTarget(masked[after:])
		if err != nil {
			return err
		}
		body = target
	}

	upper := strings.ToUpper(body)
	for _, kw := range forbidden {
		if containsWord(upper, kw) {
			return fmt.Errorf("query contains disallowed keyword %q — this tool is read-only", kw)
		}
	}
	relationsUpper := strings.ToUpper(relationMask(sql))
	for _, rel := range forbiddenRelations {
		if containsWord(relationsUpper, rel) {
			return fmt.Errorf("query references restricted system catalog %q", strings.ToLower(rel))
		}
	}
	return nil
}

// PrepareCount validates sql, rejects EXPLAIN, and removes trailing statement terminators.
func PrepareCount(sql string) (string, error) {
	if err := Validate(sql); err != nil {
		return "", err
	}
	masked, _ := mask(sql)
	first, _ := firstKeyword(strings.TrimSpace(masked))
	if first == "EXPLAIN" {
		return "", ErrCountExplain
	}
	_, _, trailingStart := maskSQL(sql, maskKeywords)
	if trailingStart >= 0 {
		sql = sql[:trailingStart]
	}
	return strings.TrimSpace(sql), nil
}

// firstKeyword returns the first SQL keyword (uppercased), skipping any leading
// open parentheses and whitespace, plus the offset just past that keyword.
func firstKeyword(s string) (string, int) {
	i := 0
	for i < len(s) && (s[i] == '(' || unicode.IsSpace(rune(s[i]))) {
		i++
	}
	start := i
	for i < len(s) && (isWordChar(s[i])) {
		i++
	}
	return strings.ToUpper(s[start:i]), i
}

// explainTarget strips EXPLAIN's option list from rest (everything after the
// EXPLAIN keyword) and returns the statement being explained, which must itself
// start with a read-only keyword. Both spellings are handled: the parenthesised
// list, EXPLAIN (ANALYZE, BUFFERS) SELECT ..., and the legacy words,
// EXPLAIN ANALYZE VERBOSE SELECT ...
func explainTarget(rest string) (string, error) {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "(") {
		end := optionListEnd(rest)
		if end < 0 {
			return "", errors.New("EXPLAIN option list is missing a closing ')'")
		}
		rest = strings.TrimSpace(rest[end:])
	} else {
		for {
			kw, after := firstKeyword(rest)
			if kw != "ANALYZE" && kw != "VERBOSE" {
				break
			}
			rest = strings.TrimSpace(rest[after:])
		}
	}
	target, _ := firstKeyword(rest)
	if target == "" {
		return "", errors.New("EXPLAIN requires a statement to explain")
	}
	if !allowedStart[target] {
		return "", fmt.Errorf("only read-only queries are allowed; EXPLAIN targets %q, not SELECT/WITH/VALUES/TABLE", target)
	}
	return rest, nil
}

// optionListEnd returns the offset just past the parenthesised option list that
// s starts with, or -1 if it is never closed.
func optionListEnd(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
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
