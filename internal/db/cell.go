package db

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

func CellHash(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func truncateCell(cell any) (any, bool) {
	encoded, err := json.Marshal(cell)
	if err != nil || len(encoded) <= cellPreviewBytes {
		return cell, false
	}
	preview := []byte(CellString(cell))
	preview = preview[:min(len(preview), cellPreviewBytes-len("…"))]
	for !utf8.Valid(preview) {
		preview = preview[:len(preview)-1]
	}
	return string(preview) + "…", true
}
