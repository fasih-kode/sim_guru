package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/signintech/gopdf"
	"github.com/xuri/excelize/v2"
)

//go:embed pdf_fonts/DejaVuSans.ttf
var journalExportFonts embed.FS

const teachingJournalExportMaxBytes = 20 << 20

var errTeachingJournalExportTooLarge = errors.New("file ekspor jurnal melebihi batas ukuran")

type teachingJournalExportBuffer struct {
	bytes.Buffer
	limit int
}

func (b *teachingJournalExportBuffer) Write(data []byte) (int, error) {
	if len(data) > b.limit-b.Len() {
		return 0, errTeachingJournalExportTooLarge
	}
	return b.Buffer.Write(data)
}

func journalExportURL(filter academic.TeachingJournalReadFilter, extension string) string {
	pageURL := journalPageURL(filter, 1)
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return "/jurnal-saya/ekspor." + extension
	}
	values := parsed.Query()
	values.Del("page")
	values.Set("format", extension)
	return "/jurnal-saya/ekspor?" + values.Encode()
}

// teachingJournalExport hanya mengekspor seluruh hasil filter milik guru yang
// sedang login; batas baris dan ukuran mencegah file parsial maupun alokasi
// memori yang tidak terbatas.
func (a *application) teachingJournalExport(w http.ResponseWriter, r *http.Request) {
	filter, err := parseJournalFilter(r)
	if err != nil {
		http.Error(w, "Filter ekspor tidak valid.", http.StatusBadRequest)
		return
	}
	items, err := a.options.Academics.TeachingJournalsForExport(r.Context(), principal(r).ID, filter)
	if errors.Is(err, academic.ErrTeachingJournalExportLimit) {
		http.Error(w, "Hasil melebihi batas 1.000 jurnal. Tambahkan filter lalu coba kembali.", http.StatusRequestEntityTooLarge)
		return
	}
	if err != nil {
		code, message := academicError(err)
		if code >= 500 {
			a.failure(w, err)
			return
		}
		http.Error(w, message, code)
		return
	}

	var data []byte
	contentType, filename := "", ""
	switch r.URL.Query().Get("format") {
	case "pdf":
		data, err = buildTeachingJournalPDF(items)
		contentType, filename = "application/pdf", "jurnal-saya.pdf"
	case "xlsx":
		data, err = buildTeachingJournalXLSX(items)
		contentType, filename = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "jurnal-saya.xlsx"
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		if errors.Is(err, errTeachingJournalExportTooLarge) {
			http.Error(w, "File ekspor melebihi batas ukuran. Tambahkan filter lalu coba kembali.", http.StatusRequestEntityTooLarge)
			return
		}
		a.failure(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func buildTeachingJournalXLSX(items []academic.TeachingJournalListItem) ([]byte, error) {
	book := excelize.NewFile()
	defer book.Close()
	sheet := book.GetSheetName(0)
	headers := []string{"No.", "Tanggal WIB", "Kelas", "Mata Pelajaran", "Guru Pengampu", "Jam Pelajaran", "Materi Pembelajaran", "Catatan Kegiatan", "Status", "Hasil Pertemuan", "Alasan Tidak Terlaksana"}
	style, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"465FFF"}, Pattern: 1}, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	if err != nil {
		return nil, err
	}
	for column, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(column+1, 1)
		if err = book.SetCellStr(sheet, cell, header); err != nil {
			return nil, err
		}
		if err = book.SetCellStyle(sheet, cell, cell, style); err != nil {
			return nil, err
		}
	}
	widths := []float64{8, 16, 18, 24, 28, 20, 42, 42, 14, 22, 42}
	for i, width := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		if err = book.SetColWidth(sheet, col, col, width); err != nil {
			return nil, err
		}
	}
	for row, item := range items {
		values := []string{
			fmt.Sprint(row + 1), item.MeetingDate, item.ClassSnapshot, item.SubjectSnapshot,
			item.TeacherSnapshot, item.PeriodLabel, emptyDash(item.Material), emptyDash(item.ActivityNotes),
			journalStateLabel(item.State), journalOutcomeLabel(item.Outcome), emptyDash(item.NotHeldReason),
		}
		for column, value := range values {
			cell, _ := excelize.CoordinatesToCellName(column+1, row+2)
			// Semua nilai ditulis eksplisit sebagai string agar awalan formula tidak dieksekusi.
			if err = book.SetCellStr(sheet, cell, value); err != nil {
				return nil, err
			}
		}
	}
	if len(items) > 0 {
		last, _ := excelize.CoordinatesToCellName(len(headers), len(items)+1)
		if err = book.AutoFilter(sheet, "A1:"+last, nil); err != nil {
			return nil, err
		}
	}
	if err = book.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	buffer := &teachingJournalExportBuffer{limit: teachingJournalExportMaxBytes}
	if err = book.Write(buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

type journalPDFColumn struct {
	heading string
	width   float64
}

var journalPDFColumns = []journalPDFColumn{
	{"No.", 27}, {"Tanggal WIB", 58}, {"Kelas / Mapel", 87}, {"Guru", 80},
	{"JP", 56}, {"Materi", 127}, {"Catatan", 118}, {"Status", 48}, {"Hasil / Alasan", 118},
}

const (
	journalPDFBodyTop    = 77.0
	journalPDFBodyBottom = 573.0
	journalPDFLineHeight = 9.0
	journalPDFCellPad    = 8.0
	journalPDFLinesPage  = 54
)

func buildTeachingJournalPDF(items []academic.TeachingJournalListItem) ([]byte, error) {
	font, err := journalExportFonts.ReadFile("pdf_fonts/DejaVuSans.ttf")
	if err != nil {
		return nil, err
	}
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: gopdf.Rect{W: 842, H: 595}})
	if err = pdf.AddTTFFontData("DejaVuSans", font); err != nil {
		return nil, err
	}
	if err = pdf.SetFont("DejaVuSans", "", 7); err != nil {
		return nil, err
	}
	left := 20.0
	newPage := func() error {
		pdf.AddPage()
		if err := pdf.SetFont("DejaVuSans", "", 12); err != nil {
			return err
		}
		pdf.SetXY(left, 17)
		if err := pdf.Cell(&gopdf.Rect{W: 500, H: 18}, "Jurnal Mengajar Saya"); err != nil {
			return err
		}
		if err := pdf.SetFont("DejaVuSans", "", 7); err != nil {
			return err
		}
		pdf.SetXY(left, 37)
		if err := pdf.Cell(&gopdf.Rect{W: 400, H: 12}, "Dibuat "+time.Now().In(time.FixedZone("WIB", 7*60*60)).Format("02-01-2006 15:04 WIB")); err != nil {
			return err
		}
		x, y, h := left, 55.0, 22.0
		pdf.SetFillColor(238, 242, 255)
		pdf.SetStrokeColor(203, 213, 225)
		for _, column := range journalPDFColumns {
			pdf.RectFromUpperLeftWithStyle(x, y, column.width, h, "FD")
			x += column.width
		}
		// Gopdf memakai operator fill yang sama untuk warna teks dan isian
		// persegi. Gambar seluruh latar dahulu, lalu paksa warna teks gelap.
		setJournalPDFTextColor(pdf)
		x = left
		for _, column := range journalPDFColumns {
			pdf.SetXY(x+3, y+6)
			if err := pdf.Cell(&gopdf.Rect{W: column.width - 5, H: h - 5}, column.heading); err != nil {
				return err
			}
			x += column.width
		}
		return nil
	}
	if err = newPage(); err != nil {
		return nil, err
	}
	y := journalPDFBodyTop
	if len(items) == 0 {
		pdf.SetXY(left, y+8)
		if err = pdf.Cell(&gopdf.Rect{W: 500, H: 16}, "Tidak ada jurnal yang sesuai dengan filter."); err != nil {
			return nil, err
		}
	}
	for i, item := range items {
		values := journalPDFRow(i+1, item)
		lines := make([][]string, len(values))
		rowLines := 1
		widthCache := make(map[rune]float64)
		for col, value := range values {
			value = strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " ")
			if value == "" {
				value = "—"
			}
			lines[col] = wrapJournalPDFText(pdf, value, journalPDFColumns[col].width-6, widthCache)
			if len(lines[col]) == 0 {
				lines[col] = []string{"—"}
			}
			if len(lines[col]) > rowLines {
				rowLines = len(lines[col])
			}
		}
		lineOffset := 0
		for lineOffset < rowLines {
			if y+journalPDFLineHeight+journalPDFCellPad > journalPDFBodyBottom {
				if err = newPage(); err != nil {
					return nil, err
				}
				y = journalPDFBodyTop
			}
			lineCount := min(rowLines-lineOffset, journalPDFLinesPage)
			lineCount = min(lineCount, int((journalPDFBodyBottom-y-journalPDFCellPad)/journalPDFLineHeight))
			rowHeight := float64(lineCount)*journalPDFLineHeight + journalPDFCellPad
			x := left
			pdf.SetFillColor(255, 255, 255)
			pdf.SetStrokeColor(226, 232, 240)
			for _, column := range journalPDFColumns {
				pdf.RectFromUpperLeftWithStyle(x, y, column.width, rowHeight, "FD")
				x += column.width
			}
			setJournalPDFTextColor(pdf)
			x = left
			for col, column := range journalPDFColumns {
				end := min(len(lines[col]), lineOffset+lineCount)
				for lineNo := lineOffset; lineNo < end; lineNo++ {
					pdf.SetXY(x+3, y+4+float64(lineNo-lineOffset)*journalPDFLineHeight)
					if err = pdf.CellWithOption(&gopdf.Rect{W: column.width - 6, H: journalPDFLineHeight}, lines[col][lineNo], gopdf.CellOption{Align: gopdf.Left | gopdf.Top}); err != nil {
						return nil, err
					}
				}
				x += column.width
			}
			y += rowHeight
			lineOffset += lineCount
		}
	}
	var output teachingJournalExportBuffer
	output.limit = teachingJournalExportMaxBytes
	if _, err = pdf.WriteTo(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func setJournalPDFTextColor(pdf *gopdf.GoPdf) {
	// Warna fill yang dipakai menggambar sel dapat menimpa warna teks di PDF.
	// Reset melalui putih lalu abu gelap agar cache warna internal gopdf tidak
	// menganggap warna teks lama masih aktif setelah background sel digambar.
	pdf.SetTextColor(255, 255, 255)
	pdf.SetTextColor(31, 41, 55)
}

// wrapJournalPDFText mengukur tiap rune sekali, sehingga catatan panjang tidak
// memicu pengukuran ulang seluruh prefiks secara kuadratik. Baris panjang tetap
// dipotong di batas rune dan dapat diteruskan ke halaman berikutnya.
func wrapJournalPDFText(pdf *gopdf.GoPdf, value string, maxWidth float64, cache map[rune]float64) []string {
	measureRune := func(r rune) float64 {
		if width, ok := cache[r]; ok {
			return width
		}
		width, err := pdf.MeasureTextWidth(string(r))
		if err != nil {
			width = 0
		}
		cache[r] = width
		return width
	}
	var lines []string
	var line strings.Builder
	lineWidth := 0.0
	flush := func() {
		lines = append(lines, line.String())
		line.Reset()
		lineWidth = 0
	}
	for _, word := range strings.FieldsFunc(value, unicode.IsSpace) {
		wordWidth := 0.0
		for _, r := range word {
			wordWidth += measureRune(r)
		}
		spaceWidth := measureRune(' ')
		if line.Len() > 0 && lineWidth+spaceWidth+wordWidth <= maxWidth {
			line.WriteByte(' ')
			lineWidth += spaceWidth
		} else if line.Len() > 0 {
			flush()
		}
		for _, r := range word {
			width := measureRune(r)
			if line.Len() > 0 && lineWidth+width > maxWidth {
				flush()
			}
			line.WriteRune(r)
			lineWidth += width
		}
	}
	if line.Len() > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}

func journalPDFRow(number int, item academic.TeachingJournalListItem) []string {
	classSubject := item.ClassSnapshot + " / " + item.SubjectSnapshot
	outcome := journalOutcomeLabel(item.Outcome)
	if item.Outcome == "not_held" && item.NotHeldReason != "" {
		outcome += "\nAlasan: " + item.NotHeldReason
	}
	return []string{fmt.Sprint(number), item.MeetingDate, classSubject, item.TeacherSnapshot, item.PeriodLabel,
		emptyDash(item.Material), emptyDash(item.ActivityNotes), journalStateLabel(item.State), outcome}
}

func journalStateLabel(state string) string {
	if state == "final" {
		return "Final"
	}
	return "Draf"
}

func journalOutcomeLabel(outcome string) string {
	if outcome == "not_held" {
		return "Tidak Terlaksana"
	}
	return "Terlaksana"
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}
