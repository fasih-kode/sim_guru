package master

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

const MaxImportBytes int64 = 5 << 20
const MaxImportRows = 500
const maxUnzipBytes uint64 = 20 << 20

type ImportRow struct {
	Row    int
	Input  Input
	Errors []string
}
type ImportReport struct {
	Rows       []ImportRow
	Token      string
	ErrorCount int
}

func ImportHeaders(kind Kind) []string {
	if kind == Students {
		return []string{"nama", "tanggal_lahir", "jenis_kelamin", "nis", "nisn", "telepon", "aktif"}
	}
	if kind == Teachers {
		return []string{"nama", "nip", "nuptk", "email", "telepon", "aktif"}
	}
	return nil
}
func checkArchive(data []byte) error {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ValidationError("File harus berupa Excel .xlsx yang sah, bukan .xls, CSV, atau file terenkripsi.")
	}
	if len(z.File) > 200 {
		return ValidationError("Struktur file Excel terlalu besar.")
	}
	var total uint64
	for _, entry := range z.File {
		if entry.UncompressedSize64 > maxUnzipBytes || total > maxUnzipBytes-entry.UncompressedSize64 {
			return ValidationError("Ukuran isi Excel melebihi batas 20 MiB.")
		}
		total += entry.UncompressedSize64
		name := strings.ToLower(entry.Name)
		if strings.Contains(name, "vbaproject") || strings.Contains(name, "externallinks/") {
			return ValidationError("Macro dan tautan workbook eksternal tidak didukung.")
		}
	}
	return nil
}

// ParseImport reads values only. It never evaluates formulas or silently coerces identifiers to numbers.
func ParseImport(kind Kind, reader io.Reader) (ImportReport, error) {
	var report ImportReport
	headers := ImportHeaders(kind)
	if headers == nil {
		return report, ValidationError("Impor Excel hanya tersedia untuk siswa dan guru.")
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxImportBytes+1))
	if err != nil {
		return report, ValidationError("File tidak dapat dibaca.")
	}
	if int64(len(data)) > MaxImportBytes {
		return report, ValidationError("Ukuran file maksimal 5 MiB.")
	}
	if err = checkArchive(data); err != nil {
		return report, err
	}
	book, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: int64(maxUnzipBytes), UnzipXMLSizeLimit: 1 << 20})
	if err != nil {
		return report, ValidationError("File Excel rusak atau tidak didukung.")
	}
	defer book.Close()
	props, err := book.GetWorkbookProps()
	if err != nil {
		return report, ValidationError("Properti workbook tidak dapat dibaca.")
	}
	date1904 := props.Date1904 != nil && *props.Date1904
	iterator, err := book.Rows("Data")
	if err != nil {
		return report, ValidationError("Sheet Data tidak ditemukan. Gunakan template yang disediakan.")
	}
	defer iterator.Close()
	physical := 0
	headerFound := false
	for iterator.Next() {
		physical++
		if physical > 2000 {
			return report, ValidationError("Sheet Data melewati 2000 baris fisik. Hapus baris tambahan yang tidak diperlukan.")
		}
		values, err := iterator.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return report, ValidationError("Sel Excel tidak dapat dibaca.")
		}
		if physical == 1 {
			if len(values) != len(headers) {
				return report, ValidationError("Kolom sheet Data tidak sesuai template.")
			}
			for i, h := range headers {
				cell, _ := excelize.CoordinatesToCellName(i+1, 1)
				formula, err := book.GetCellFormula("Data", cell)
				if err != nil || formula != "" || strings.TrimSpace(values[i]) != h {
					return report, ValidationError("Header sheet Data harus sama dan berurutan seperti template.")
				}
			}
			headerFound = true
			continue
		}
		blank := true
		for _, v := range values {
			if strings.TrimSpace(v) != "" {
				blank = false
			}
		}
		// Formula-only rows with empty cached results must not be silently skipped.
		if len(values) > len(headers) {
			return report, ValidationError("Terdapat kolom tambahan di luar template.")
		}
		formulas := false
		for col := 1; col <= len(headers); col++ {
			cell, _ := excelize.CoordinatesToCellName(col, physical)
			f, err := book.GetCellFormula("Data", cell)
			if err != nil {
				return report, ValidationError("Formula sel tidak dapat diperiksa.")
			}
			if f != "" {
				formulas = true
			}
		}
		if blank && !formulas {
			continue
		}
		if len(report.Rows) >= MaxImportRows {
			return report, ValidationError("Maksimal 500 baris data per impor.")
		}
		row := ImportRow{Row: physical, Input: Input{Record: Record{Active: true}}}
		if formulas {
			row.Errors = append(row.Errors, "Formula tidak didukung. Tempel sebagai nilai sebelum mengunggah.")
		}
		for i, h := range headers {
			value := ""
			if i < len(values) {
				value = strings.TrimSpace(values[i])
			}
			cell, _ := excelize.CoordinatesToCellName(i+1, physical)
			if value != "" && (h == "nis" || h == "nisn" || h == "nip" || h == "nuptk" || h == "telepon") {
				cellType, err := book.GetCellType("Data", cell)
				if err != nil {
					return report, ValidationError("Tipe sel tidak dapat dibaca.")
				}
				if cellType != excelize.CellTypeInlineString && cellType != excelize.CellTypeSharedString {
					row.Errors = append(row.Errors, h+" harus disimpan sebagai teks agar nol awal/nomor panjang tidak berubah.")
				}
			}
			switch h {
			case "nama":
				row.Input.Name = value
			case "tanggal_lahir":
				row.Input.BirthDate = value
				if value != "" {
					if serial, err := strconv.ParseFloat(value, 64); err == nil {
						if math.IsNaN(serial) || math.IsInf(serial, 0) || serial != math.Trunc(serial) || serial < 0 || (!date1904 && (serial == 0 || serial == 60)) {
							row.Errors = append(row.Errors, "Tanggal lahir Excel tidak valid.")
						} else if date, err := excelize.ExcelDateToTime(serial, date1904); err == nil {
							row.Input.BirthDate = date.Format("2006-01-02")
						} else {
							row.Errors = append(row.Errors, "Tanggal lahir Excel tidak valid.")
						}
					} else if date, err := time.Parse(time.RFC3339, value); err == nil {
						row.Input.BirthDate = date.Format("2006-01-02")
					}
				}
			case "jenis_kelamin":
				row.Input.Gender = strings.ToUpper(value)
			case "nis":
				row.Input.NIS = value
			case "nisn":
				row.Input.NISN = value
			case "nip":
				row.Input.NIP = value
			case "nuptk":
				row.Input.NUPTK = value
			case "email":
				row.Input.Email = value
			case "telepon":
				row.Input.Phone = value
			case "aktif":
				switch strings.ToLower(value) {
				case "", "ya", "true", "1", "aktif":
					row.Input.Active = true
				case "tidak", "false", "0", "nonaktif":
					row.Input.Active = false
				default:
					row.Errors = append(row.Errors, "aktif harus YA atau TIDAK (kosong berarti YA).")
				}
			}
		}
		if err := row.Input.Validate(kind, true); err != nil {
			row.Errors = append(row.Errors, err.Error())
		}
		if len(row.Errors) > 0 {
			report.ErrorCount++
		}
		report.Rows = append(report.Rows, row)
	}
	if err = iterator.Error(); err != nil {
		return report, ValidationError("Sheet Data tidak dapat dibaca sampai selesai.")
	}
	if !headerFound || len(report.Rows) == 0 {
		return report, ValidationError("Sheet Data belum berisi data untuk diimpor.")
	}
	return report, nil
}
func importConflict(row int) error {
	return ValidationError(fmt.Sprintf("Baris %d berpotensi duplikat atau sudah berubah sejak pratinjau. Periksa data dan unggah ulang; belum ada baris yang disimpan.", row))
}
