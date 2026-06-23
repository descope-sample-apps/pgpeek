package guard

import (
	"strings"
	"testing"
)

func TestValidate_Allows(t *testing.T) {
	ok := []string{
		"SELECT 1",
		"select * from users where id = 1",
		"  SELECT now();  ",
		"WITH t AS (SELECT 1) SELECT * FROM t",
		"(SELECT 1) UNION (SELECT 2)",
		"SELECT update_time, created_at FROM events", // 'update' inside identifier
		"SELECT 'drop table users' AS note",          // keyword inside string literal
		"SELECT * FROM updates",                      // 'updates' not whole-word 'update'
		"SELECT * FROM t -- delete this later\n",     // keyword in comment
		"VALUES (1),(2)",
		"TABLE users",
		"EXPLAIN SELECT * FROM users",
		"SELECT col FROM t WHERE name = 'a;b'", // semicolon inside string
	}
	for _, q := range ok {
		if err := Validate(q); err != nil {
			t.Errorf("expected OK, got error for %q: %v", q, err)
		}
	}
}

func TestValidate_Rejects(t *testing.T) {
	bad := []string{
		"",
		"   ",
		"INSERT INTO users VALUES (1)",
		"UPDATE users SET x = 1",
		"DELETE FROM users",
		"DROP TABLE users",
		"TRUNCATE users",
		"SELECT 1; DROP TABLE users",         // two statements
		"SELECT 1; SELECT 2",                 // two statements
		"WITH t AS (SELECT 1) DELETE FROM t", // CTE then DML
		"SET statement_timeout = 0",          // session tampering
		"COPY users TO '/tmp/x'",             // exfil
		"CREATE TABLE x (id int)",            // DDL
		"GRANT ALL ON users TO public",       // privilege
		"SHOW server_version",                // SHOW no longer allowed
		"SHOW hba_file",                      // host-topology leak
		"SELECT * FROM pg_shadow",            // credential catalog
		"TABLE pg_authid",                    // credential catalog
		"SELECT * FROM pg_hba_file_rules",    // host topology
		"select 1; ",                         // wait: trailing semicolon should be OK
	}
	// The last entry is intentionally OK; verify trailing-semicolon handling separately.
	for _, q := range bad[:len(bad)-1] {
		if err := Validate(q); err == nil {
			t.Errorf("expected error, got OK for %q", q)
		}
	}
}

func TestValidate_TrailingSemicolon(t *testing.T) {
	for _, q := range []string{"SELECT 1;", "SELECT 1; ", "SELECT 1;\n\n"} {
		if err := Validate(q); err != nil {
			t.Errorf("trailing semicolon should be allowed for %q: %v", q, err)
		}
	}
}

// FuzzValidate asserts the guard never panics on arbitrary input, and that any
// input it *accepts* really is a single statement beginning with an allowed
// keyword and containing no forbidden keyword (in masked form). This is the
// invariant the rest of the system relies on.
func FuzzValidate(f *testing.F) {
	seeds := []string{
		"", "SELECT 1", "select * from t; drop table t",
		"WITH x AS (SELECT 1) SELECT * FROM x",
		"-- c\nSELECT 1", "/* */ DELETE FROM t",
		"SELECT '$$'; $$", "$$a$$", "((((", "';--", "SET x=1",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, sql string) {
		err := Validate(sql) // must never panic
		if err != nil {
			return
		}
		masked, statements := mask(sql)
		if statements > 1 {
			t.Errorf("accepted multi-statement input: %q", sql)
		}
		if fk := firstKeyword(strings.TrimSpace(masked)); !allowedStart[fk] {
			t.Errorf("accepted input with disallowed leading keyword %q: %q", fk, sql)
		}
		up := strings.ToUpper(masked)
		for _, kw := range forbidden {
			if containsWord(up, kw) {
				t.Errorf("accepted input containing forbidden keyword %q: %q", kw, sql)
			}
		}
		for _, rel := range forbiddenRelations {
			if containsWord(up, rel) {
				t.Errorf("accepted input referencing restricted catalog %q: %q", rel, sql)
			}
		}
	})
}

func TestValidate_DollarQuoted(t *testing.T) {
	// A dollar-quoted string containing a semicolon and a keyword must not trip
	// the multi-statement or keyword checks.
	q := "SELECT $$ delete; from $$ AS x"
	if err := Validate(q); err != nil {
		t.Errorf("dollar-quoted content should be ignored: %v", err)
	}
}

// TestMaskEdgeCases exercises every branch of the comment/string scanner.
func TestMaskEdgeCases(t *testing.T) {
	cases := []struct {
		name           string
		in             string
		wantStatements int
	}{
		{"nested block comment", "/* a /* b */ c */ SELECT 1", 1},
		{"unterminated block comment", "SELECT 1 /* unterminated", 1},
		{"unterminated single quote", "SELECT 'abc", 1},
		{"escaped single quote", "SELECT 'a''b'", 1},
		{"double-quoted ident", `SELECT "col" FROM t`, 1},
		{"escaped double quote", `SELECT "a""b" FROM t`, 1},
		{"unterminated double quote", `SELECT "abc`, 1},
		{"unterminated dollar quote", "SELECT $$ abc", 1},
		{"dollar tag with word", "SELECT $tag$ x $tag$ AS y", 1},
		{"line comment to EOF", "SELECT 1 -- trailing", 1},
		{"dollar then space (not a tag)", "SELECT 1 $ 2", 1},
		{"two statements", "SELECT 1; SELECT 2", 2},
		{"trailing semicolon only", "SELECT 1;", 1},
		{"three statements", "SELECT 1; SELECT 2; SELECT 3", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, statements := mask(c.in)
			if statements != c.wantStatements {
				t.Errorf("mask(%q) statements = %d, want %d", c.in, statements, c.wantStatements)
			}
		})
	}
}

func TestDollarTag(t *testing.T) {
	cases := []struct {
		in      string
		wantTag string
		wantOK  bool
	}{
		{"$$", "$$", true},
		{"$tag$", "$tag$", true},
		{"$ ", "", false},   // space is not a word char
		{"$abc", "", false}, // no closing $
	}
	for _, c := range cases {
		tag, ok := dollarTag(c.in, 0)
		if tag != c.wantTag || ok != c.wantOK {
			t.Errorf("dollarTag(%q) = (%q, %v), want (%q, %v)", c.in, tag, ok, c.wantTag, c.wantOK)
		}
	}
}

func TestFirstKeywordEmpty(t *testing.T) {
	if got := firstKeyword("((("); got != "" {
		t.Errorf("firstKeyword of only-parens = %q, want empty", got)
	}
}
