package db

import (
	"errors"
	"strconv"
	"unicode/utf8"
)

var ErrResultMetadataTooLarge = errors.New("query result metadata exceeds response limit")

func resultEnvelopeBytes(columns []string, rowCap int) int {
	size := len(`{"columns":[],"rows":[],"rowCount":,"truncated":false,"cellsTruncated":true,"truncatedCells":[],"elapsedMs":}`)
	size += len(strconv.Itoa(max(rowCap, 0))) + len("9223372036854775807")
	for _, column := range columns {
		size += jsonStringBytes(column) + 1
	}
	return size
}

func jsonStringBytes(value string) int {
	size := 2
	for len(value) > 0 {
		r, width := utf8.DecodeRuneInString(value)
		value = value[width:]
		if r == utf8.RuneError && width == 1 {
			size += 6
			continue
		}
		switch r {
		case '"', '\\', '\b', '\f', '\n', '\r', '\t':
			size += 2
		case '<', '>', '&', '\u2028', '\u2029':
			size += 6
		default:
			if r < ' ' {
				size += 6
			} else {
				size += utf8.RuneLen(r)
			}
		}
	}
	return size
}
