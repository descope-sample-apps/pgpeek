package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

const maxBodyBytes = 1 << 20

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func safeFilename(name string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '-', r == '.':
			return r
		default:
			return '_'
		}
	}, name)
	if out == "" {
		return "table"
	}
	return out
}

func parseFilters(raw []string) []db.Filter {
	if len(raw) == 0 {
		return nil
	}
	out := make([]db.Filter, 0, len(raw))
	for _, s := range raw {
		parts := strings.SplitN(s, ":", 3)
		f := db.Filter{Column: parts[0]}
		if len(parts) >= 2 {
			f.Op = parts[1]
		}
		if len(parts) == 3 {
			f.Value = parts[2]
		}
		out = append(out, f)
	}
	return out
}

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
