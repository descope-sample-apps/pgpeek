package server

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/descope-sample-apps/pgpeek/internal/db"
)

var errCSVResultChanged = errors.New("query result changed during export")

func writeCSVResponse(w http.ResponseWriter, filename string, render func(io.Writer) error) error {
	spool, err := os.CreateTemp("", "pgpeek-export-*.csv")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to prepare the CSV export")
		return err
	}
	defer func() {
		_ = spool.Close()
		_ = os.Remove(spool.Name())
	}()

	if err := render(spool); err != nil {
		if errors.Is(err, errCSVResultChanged) {
			writeError(w, http.StatusConflict, "The data changed. Reload and export again.")
		} else {
			writeError(w, http.StatusBadRequest, "Failed to export the data")
		}
		return err
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, err = io.Copy(w, io.NewSectionReader(spool, 0, 1<<63-1))
	return err
}

func writeCSV(w io.Writer, res *db.Result, resolve func(row, column int) (any, error)) error {
	cw := csv.NewWriter(w)
	_ = cw.Write(res.Columns)
	truncated := make(map[[2]int]string, len(res.TruncatedCells))
	for _, cell := range res.TruncatedCells {
		truncated[[2]int{cell.Row, cell.Column}] = cell.Hash
	}
	row := make([]string, len(res.Columns))
	for rowIndex, rec := range res.Rows {
		for i, cell := range rec {
			if hash, ok := truncated[[2]int{rowIndex, i}]; ok && resolve != nil {
				var err error
				cell, err = resolve(rowIndex, i)
				if err != nil {
					return err
				}
				if hash != "" && db.CellHash(cell) != hash {
					return errCSVResultChanged
				}
			}
			row[i] = db.CellString(cell)
		}
		_ = cw.Write(row)
	}
	cw.Flush()
	return cw.Error()
}
