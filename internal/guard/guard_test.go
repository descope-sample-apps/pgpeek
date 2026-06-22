package guard

import "testing"

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

func TestValidate_DollarQuoted(t *testing.T) {
	// A dollar-quoted string containing a semicolon and a keyword must not trip
	// the multi-statement or keyword checks.
	q := "SELECT $$ delete; from $$ AS x"
	if err := Validate(q); err != nil {
		t.Errorf("dollar-quoted content should be ignored: %v", err)
	}
}
