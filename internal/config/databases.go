package config

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const maxNumberedDatabases = 64

// DatabaseEntry holds one named database target.
type DatabaseEntry struct {
	ID      string
	Name    string
	DSN     string
	IAMAuth bool
	Region  string
}

func loadDatabases(globalIAMAuth bool, globalRegion string) ([]DatabaseEntry, string, error) {
	if err := validateDatabaseSourceFamily(); err != nil {
		return nil, "", err
	}
	entries := make([]DatabaseEntry, 0, 4)
	fileDefault, err := appendDatabaseFileEntries(&entries, globalIAMAuth, globalRegion)
	if err != nil {
		return nil, "", err
	}
	if err := appendListDatabaseEntries(&entries, globalIAMAuth, globalRegion); err != nil {
		return nil, "", err
	}
	if err := appendNumberedDatabaseEntries(&entries, globalIAMAuth, globalRegion); err != nil {
		return nil, "", err
	}
	if len(entries) == 0 {
		dsn, err := envOrFile("DATABASE_URL")
		if err != nil {
			return nil, "", err
		}
		if dsn == "" {
			return nil, "", errors.New("DATABASE_URL (or DATABASE_URL_FILE) is required")
		}
		entries = append(entries, DatabaseEntry{ID: "default", Name: "Default", DSN: dsn, IAMAuth: globalIAMAuth, Region: globalRegion})
	}
	defaultID := env("PGPEEK_DEFAULT_DATABASE", fileDefault)
	if defaultID == "" && len(entries) > 0 {
		defaultID = entries[0].ID
	}
	return entries, defaultID, nil
}

func validateDatabaseSourceFamily() error {
	sources := 0
	if os.Getenv("PGPEEK_DATABASES_FILE") != "" {
		sources++
	}
	if strings.TrimSpace(os.Getenv("PGPEEK_DATABASE_URLS")) != "" {
		sources++
	}
	for i := 1; i <= maxNumberedDatabases; i++ {
		key := fmt.Sprintf("PGPEEK_DATABASE_URL_%d", i)
		if os.Getenv(key) != "" || os.Getenv(key+"_FILE") != "" {
			sources++
			break
		}
	}
	if sources > 1 {
		return errors.New("configure only one multi-database source: PGPEEK_DATABASES_FILE, PGPEEK_DATABASE_URLS, or PGPEEK_DATABASE_URL_N")
	}
	return nil
}

func appendDatabaseFileEntries(entries *[]DatabaseEntry, globalIAMAuth bool, globalRegion string) (string, error) {
	if os.Getenv("PGPEEK_DATABASES_FILE") == "" {
		return "", nil
	}
	b, err := readOperatorFile(os.Getenv("PGPEEK_DATABASES_FILE"), "PGPEEK_DATABASES_FILE")
	if err != nil {
		return "", err
	}
	var file struct {
		Default           string `json:"default"`
		DefaultDatabaseID string `json:"defaultDatabaseID"`
		Databases         []struct {
			ID, Name, URL, URLFile, Region string
			IAMAuth                        bool
		} `json:"databases"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		return "", fmt.Errorf("parse PGPEEK_DATABASES_FILE: %w", err)
	}
	for i, item := range file.Databases {
		dsn, err := databaseFileDSN(item.URL, item.URLFile)
		if err != nil {
			return "", err
		}
		*entries = append(*entries, DatabaseEntry{ID: item.ID, Name: entryName(item.Name, i+1), DSN: dsn, IAMAuth: item.IAMAuth || globalIAMAuth, Region: pick(item.Region, globalRegion)})
	}
	if file.DefaultDatabaseID != "" {
		return file.DefaultDatabaseID, nil
	}
	return file.Default, nil
}

func databaseFileDSN(url, urlFile string) (string, error) {
	dsn := strings.TrimSpace(url)
	if dsn != "" || urlFile == "" {
		return dsn, nil
	}
	b, err := readOperatorFile(urlFile, "database urlFile")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func appendListDatabaseEntries(entries *[]DatabaseEntry, globalIAMAuth bool, globalRegion string) error {
	urls, err := splitDatabaseList(os.Getenv("PGPEEK_DATABASE_URLS"))
	if err != nil || len(urls) == 0 {
		return err
	}
	ids, err := splitDatabaseList(os.Getenv("PGPEEK_DATABASE_IDS"))
	if err != nil {
		return err
	}
	names, err := splitDatabaseList(os.Getenv("PGPEEK_DATABASE_NAMES"))
	if err != nil {
		return err
	}
	for i, url := range urls {
		*entries = append(*entries, DatabaseEntry{ID: listValue(ids, i, fmt.Sprintf("db%d", i+1)), Name: entryName(listValue(names, i, ""), i+1), DSN: strings.TrimSpace(url), IAMAuth: globalIAMAuth, Region: globalRegion})
	}
	return nil
}

func appendNumberedDatabaseEntries(entries *[]DatabaseEntry, globalIAMAuth bool, globalRegion string) error {
	for i := 1; i <= maxNumberedDatabases; i++ {
		key := fmt.Sprintf("PGPEEK_DATABASE_URL_%d", i)
		dsn, err := envOrFile(key)
		if err != nil {
			return err
		}
		if dsn == "" {
			break
		}
		*entries = append(*entries, DatabaseEntry{ID: env(fmt.Sprintf("PGPEEK_DATABASE_ID_%d", i), fmt.Sprintf("db%d", i)), Name: entryName(os.Getenv(fmt.Sprintf("PGPEEK_DATABASE_NAME_%d", i)), i), DSN: dsn, IAMAuth: globalIAMAuth, Region: globalRegion})
	}
	return nil
}

func applyDefaultDatabase(c *Config) error {
	for _, db := range c.Databases {
		if db.ID == c.DefaultDatabaseID {
			c.DB.DSN = db.DSN
			c.DB.IAMAuth = db.IAMAuth
			c.DB.Region = db.Region
			return nil
		}
	}
	return fmt.Errorf("PGPEEK_DEFAULT_DATABASE %q does not match a configured database", c.DefaultDatabaseID)
}

func validateDatabases(databases []DatabaseEntry) error {
	if len(databases) == 0 {
		return errors.New("at least one database is required")
	}
	seen := make(map[string]struct{}, len(databases))
	for _, db := range databases {
		if err := validateDatabaseEntry(db, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateDatabaseEntry(db DatabaseEntry, seen map[string]struct{}) error {
	if db.ID == "" {
		return errors.New("database ID must be non-empty")
	}
	if !isDatabaseID(db.ID) {
		return fmt.Errorf("database ID %q must contain only letters, numbers, dot, underscore, or dash", db.ID)
	}
	if _, ok := seen[db.ID]; ok {
		return fmt.Errorf("database ID %q is duplicated", db.ID)
	}
	seen[db.ID] = struct{}{}
	if db.DSN == "" {
		return fmt.Errorf("database %q requires url or urlFile", db.ID)
	}
	if db.IAMAuth && db.Region == "" {
		return fmt.Errorf("database %q IAM auth requires region", db.ID)
	}
	return nil
}

func splitDatabaseList(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	reader := csv.NewReader(strings.NewReader(normalizeListSeparators(value)))
	reader.TrimLeadingSpace = true
	items, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("parse database list: %w", err)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
}

func normalizeListSeparators(value string) string {
	inQuotes := false
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch == '"' {
			inQuotes = !inQuotes
		}
		if ch == ';' && !inQuotes {
			ch = ','
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func listValue(values []string, index int, fallback string) string {
	if index < len(values) && values[index] != "" {
		return values[index]
	}
	return fallback
}

func entryName(value string, index int) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fmt.Sprintf("Database %d", index)
}

func pick(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func isDatabaseID(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return value != ""
}

func readOperatorFile(path, label string) ([]byte, error) {
	// The path is supplied by the operator (env var / mounted-secret convention),
	// not by any request input — this is the intended use.
	b, err := os.ReadFile(path) //nolint:gosec // operator-controlled config/secret path
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	return b, nil
}
