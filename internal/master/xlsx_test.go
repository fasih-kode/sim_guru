package master

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func importFile(t *testing.T, kind Kind, values [][]any, edit func(*excelize.File)) []byte {
	t.Helper()
	book, err := excelize.OpenFile("../web/import_templates/" + string(kind) + ".xlsx")
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for row, cols := range values {
		for col, v := range cols {
			if v == nil {
				continue
			}
			cell, _ := excelize.CoordinatesToCellName(col+1, row+2)
			if err = book.SetCellValue("Data", cell, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	if edit != nil {
		edit(book)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func TestParseImport(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      Kind
		values    [][]any
		edit      func(*excelize.File)
		fatal     bool
		rowErrors bool
	}{
		{name: "student text identifiers", kind: Students, values: [][]any{{"Siswa", time.Date(2013, 8, 17, 0, 0, 0, 0, time.UTC), "p", "0007", "0012345678", "0812345678", "YA"}}},
		{name: "teacher long identifier", kind: Teachers, values: [][]any{{"Guru", "001234567890123456", "0001234567890", "guru@example.test", "0812", "tidak"}}},
		{name: "numeric identifier rejected", kind: Teachers, values: [][]any{{"Guru", 123456789012345678, "", "", "", "YA"}}, rowErrors: true},
		{name: "missing required date", kind: Students, values: [][]any{{"Siswa", "", "L"}}, rowErrors: true},
		{name: "formula rejected", kind: Teachers, edit: func(f *excelize.File) {
			if err := f.SetCellFormula("Data", "A2", "1+1"); err != nil {
				t.Fatal(err)
			}
		}, rowErrors: true},
		{name: "bad status", kind: Teachers, values: [][]any{{"Guru", "", "", "", "", "maybe"}}, rowErrors: true},
		{name: "empty workbook", kind: Teachers, fatal: true},
		{name: "incorrect header", kind: Teachers, values: [][]any{{"Guru"}}, edit: func(f *excelize.File) { f.SetCellStr("Data", "A1", "nama_guru") }, fatal: true},
		{name: "extra column", kind: Teachers, values: [][]any{{"Guru", "", "", "", "", "YA", "extra"}}, fatal: true},
		{name: "bad leap serial", kind: Students, values: [][]any{{"Siswa", 60, "L"}}, rowErrors: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := ParseImport(tc.kind, bytes.NewReader(importFile(t, tc.kind, tc.values, tc.edit)))
			if (err != nil) != tc.fatal {
				t.Fatalf("unexpected parsing error: %v", err)
			}
			if err != nil {
				return
			}
			if (report.ErrorCount > 0) != tc.rowErrors {
				t.Fatalf("row errors=%d", report.ErrorCount)
			}
			if tc.name == "student text identifiers" {
				r := report.Rows[0].Input
				if r.NIS != "0007" || r.NISN != "0012345678" || r.BirthDate != "2013-08-17" || r.Gender != "P" {
					t.Fatal("identifiers or date changed")
				}
			}
			if tc.name == "teacher long identifier" {
				r := report.Rows[0].Input
				if r.NIP != "001234567890123456" || r.Active {
					t.Fatal("teacher identifier/status changed")
				}
			}
		})
	}
}
func TestImportBoundsAndSheets(t *testing.T) {
	for _, data := range [][]byte{[]byte("not an Excel file"), bytes.Repeat([]byte("x"), int(MaxImportBytes)+1)} {
		if _, err := ParseImport(Teachers, bytes.NewReader(data)); err == nil {
			t.Fatal("invalid/oversized file accepted")
		}
	}
	values := make([][]any, MaxImportRows+1)
	for i := range values {
		values[i] = []any{"Guru"}
	}
	if _, err := ParseImport(Teachers, bytes.NewReader(importFile(t, Teachers, values, nil))); err == nil {
		t.Fatal("row limit not enforced")
	}
	data := importFile(t, Teachers, [][]any{nil, nil, {"Guru"}}, nil)
	report, err := ParseImport(Teachers, bytes.NewReader(data))
	if err != nil || len(report.Rows) != 1 || report.Rows[0].Row != 4 {
		t.Fatal("blank rows changed Excel row numbers")
	}
	base := importFile(t, Teachers, [][]any{{"Guru"}}, nil)
	source, err := zip.NewReader(bytes.NewReader(base), int64(len(base)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	target := zip.NewWriter(&buf)
	for _, f := range source.File {
		w, err := target.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(w, r)
		r.Close()
	}
	w, err := target.Create("xl/vbaProject.bin")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("macro"))
	target.Close()
	if _, err := ParseImport(Teachers, bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("macro archive accepted")
	}
}
func TestImportDate1904AndTemplate(t *testing.T) {
	report, err := ParseImport(Students, bytes.NewReader(importFile(t, Students, [][]any{{"Siswa", 40000, "L"}}, func(f *excelize.File) {
		enabled := true
		f.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &enabled})
	})))
	expected := time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 40000).Format("2006-01-02")
	if err != nil || report.ErrorCount != 0 || report.Rows[0].Input.BirthDate != expected {
		t.Fatal("1904 date system failed")
	}
	if _, err := ParseImport(Subjects, strings.NewReader("")); err == nil {
		t.Fatal("unsupported import kind accepted")
	}
}
