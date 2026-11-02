package web

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/xuri/excelize/v2"
)

func TestJournalExportURLKeepsFiltersAndDropsPagination(t *testing.T) {
	filter := academic.TeachingJournalReadFilter{
		Page: 8, Search: "aljabar", SemesterID: 12, ClassID: 23, SubjectID: 34,
		FromDate: "2026-11-01", ToDate: "2026-11-30", State: "final", Outcome: "held",
	}
	for _, format := range []string{"pdf", "xlsx"} {
		parsed, err := url.Parse(journalExportURL(filter, format))
		if err != nil {
			t.Fatal(err)
		}
		query := parsed.Query()
		if query.Get("format") != format || query.Get("page") != "" || query.Get("q") != filter.Search || query.Get("semester_id") != "12" || query.Get("class_id") != "23" || query.Get("subject_id") != "34" || query.Get("from") != filter.FromDate || query.Get("to") != filter.ToDate || query.Get("state") != "final" || query.Get("outcome") != "held" {
			t.Fatalf("filter query not preserved for %s: %s", format, parsed.RawQuery)
		}
	}
}

func TestTeachingJournalExportSizeBoundary(t *testing.T) {
	limit := teachingJournalExportMaxBytes
	buffer := &teachingJournalExportBuffer{limit: limit}
	chunk := bytes.Repeat([]byte{'x'}, 1<<20)
	for i := 0; i < 19; i++ {
		if _, err := buffer.Write(chunk); err != nil {
			t.Fatalf("write under size limit failed: %v", err)
		}
	}
	if _, err := buffer.Write(chunk[:(1<<20)-1]); err != nil || buffer.Len() != limit-1 {
		t.Fatalf("output just under limit rejected: size=%d err=%v", buffer.Len(), err)
	}
	if _, err := buffer.Write([]byte{'x'}); err != nil || buffer.Len() != limit {
		t.Fatalf("output exactly at limit rejected: size=%d err=%v", buffer.Len(), err)
	}
	if _, err := buffer.Write([]byte{'x'}); !errors.Is(err, errTeachingJournalExportTooLarge) {
		t.Fatalf("output above limit did not produce a clear size error: %v", err)
	}
	if buffer.Len() != limit {
		t.Fatalf("rejected output left a partial suffix: size=%d", buffer.Len())
	}
}

func TestTeachingJournalXLSXWritesUserTextAsString(t *testing.T) {
	items := []academic.TeachingJournalListItem{{
		MeetingDate: "2026-11-01", ClassSnapshot: " =1+1", SubjectSnapshot: "Matematika",
		TeacherSnapshot: "-3+5", PeriodLabel: "Jam ke-2–3", State: "draft", Outcome: "not_held",
		Material: "=HYPERLINK(\"https://example.invalid\")", ActivityNotes: "+SUM(A1:A2)", NotHeldReason: "@cmd",
	}}
	data, err := buildTeachingJournalXLSX(items)
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for cell, want := range map[string]string{
		"C2": " =1+1", "E2": "-3+5", "G2": "=HYPERLINK(\"https://example.invalid\")",
		"H2": "+SUM(A1:A2)", "K2": "@cmd",
	} {
		got, readErr := book.GetCellValue("Sheet1", cell)
		if readErr != nil || got != want {
			t.Fatalf("cell %s = %q, %v; want %q", cell, got, readErr, want)
		}
		formula, formulaErr := book.GetCellFormula("Sheet1", cell)
		if formulaErr != nil || formula != "" {
			t.Fatalf("user text in %s became formula %q (%v)", cell, formula, formulaErr)
		}
		kind, typeErr := book.GetCellType("Sheet1", cell)
		if typeErr != nil || kind != excelize.CellTypeSharedString {
			t.Fatalf("user text in %s is not stored as a string: type=%v err=%v", cell, kind, typeErr)
		}
	}
	if got, err := book.GetCellValue("Sheet1", "F2"); err != nil || got != "Jam ke-2–3" {
		t.Fatalf("JP snapshot not exported: %q %v", got, err)
	}
}

func TestTeachingJournalExportsSupportEmptyAndMultpageUnicode(t *testing.T) {
	for name, build := range map[string]func([]academic.TeachingJournalListItem) ([]byte, error){
		"pdf":  buildTeachingJournalPDF,
		"xlsx": buildTeachingJournalXLSX,
	} {
		t.Run(name+" empty", func(t *testing.T) {
			data, err := build(nil)
			if err != nil || len(data) == 0 {
				t.Fatalf("empty export failed: len=%d err=%v", len(data), err)
			}
		})
	}
	items := make([]academic.TeachingJournalListItem, 12)
	longText := strings.Repeat("Pembelajaran Bahasa Indonesia: murid berdiskusi, menulis, membaca, dan menyimpulkan. café — “Indonesia”. ", 18)
	longRunes := []rune(longText)
	for i := range items {
		items[i] = academic.TeachingJournalListItem{
			MeetingDate: fmt.Sprintf("2026-11-%02d", i+1), ClassSnapshot: "7A", SubjectSnapshot: "Bahasa Indonesia",
			TeacherSnapshot: "Guru Pengampu", PeriodLabel: "Jam ke-2–3", State: "final", Outcome: "held",
			Material: string(longRunes[:min(len(longRunes), 500)]), ActivityNotes: string(longRunes[:min(len(longRunes), 1900)]),
		}
	}
	pdf, err := buildTeachingJournalPDF(items)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || bytes.Count(pdf, []byte("/Type /Page")) < 2 {
		t.Fatalf("PDF is invalid or did not wrap to multiple pages: size=%d pages=%d", len(pdf), bytes.Count(pdf, []byte("/Type /Page")))
	}
	if got := journalPDFVisibleTextBlocks(pdf); got < (len(items)+1)*len(journalPDFColumns) {
		t.Fatalf("PDF table text does not consistently use a visible foreground color: visible text blocks=%d", got)
	}
}

var pdfStreamStart = regexp.MustCompile(`(?m)^stream\r?\n`)

func journalPDFVisibleTextBlocks(pdf []byte) int {
	visible := 0
	for offset := 0; offset < len(pdf); {
		loc := pdfStreamStart.FindIndex(pdf[offset:])
		if loc == nil {
			break
		}
		start := offset + loc[1]
		endRel := bytes.Index(pdf[start:], []byte("endstream"))
		if endRel < 0 {
			break
		}
		end := start + endRel
		compressed := bytes.TrimRight(pdf[start:end], "\r\n")
		reader, err := zlib.NewReader(bytes.NewReader(compressed))
		if err == nil {
			content, readErr := io.ReadAll(reader)
			_ = reader.Close()
			if readErr == nil && bytes.Contains(content, []byte("BT")) {
				visible += countDarkPDFTextBlocks(content)
			}
		}
		offset = end + len("endstream")
	}
	return visible
}

func countDarkPDFTextBlocks(content []byte) int {
	const tableText = "0.122 0.161 0.216 rg"
	scanner := bufio.NewScanner(bytes.NewReader(content))
	colorIsDark, insideText, hasText := false, false, false
	visible := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasSuffix(line, " rg") {
			colorIsDark = line == tableText
		}
		switch line {
		case "BT":
			insideText, hasText = true, false
		case "ET":
			if insideText && hasText && colorIsDark {
				visible++
			}
			insideText = false
		}
		if insideText && (strings.HasSuffix(line, " TJ") || strings.HasSuffix(line, " Tj")) {
			hasText = true
		}
	}
	return visible
}
