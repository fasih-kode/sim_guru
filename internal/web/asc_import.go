package web

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type ascMapOption struct {
	ID   int64
	Name string
	Day  int
}
type ascMapRow struct {
	Field, Source, Detail, DayField string
	Selected                        int64
	SelectedDay, SourceNumber       int
	Options                         []ascMapOption
}
type ascMapGroup struct {
	Label string
	Rows  []ascMapRow
}
type ascImportView struct {
	DateLocked bool
	Stage      academic.ASCImport
	Report     academic.ASCImportReport
	Groups     []ascMapGroup
	Date       string
}

func ascFormMapping(r *http.Request, doc academic.ASCDocument) (academic.ASCMapping, error) {
	m := academic.ASCMapping{Classes: map[string]int64{}, Teachers: map[string]int64{}, Subjects: map[string]int64{}, Days: map[string]int{}, Periods: map[string]int{}}
	read := func(key string) (int64, error) {
		v := r.PostForm[key]
		if len(v) != 1 {
			return 0, academic.ValidationError("Lengkapi semua padanan pada formulir.")
		}
		if v[0] == "" {
			return 0, nil
		}
		n, err := strconv.ParseInt(v[0], 10, 64)
		if err != nil || n < 0 {
			return 0, academic.ValidationError("Padanan harus berupa pilihan yang valid.")
		}
		return n, nil
	}
	for _, g := range []struct {
		prefix string
		items  []academic.ASCItem
		values map[string]int64
	}{{"class", doc.Classes, m.Classes}, {"teacher", doc.Teachers, m.Teachers}, {"subject", doc.Subjects, m.Subjects}} {
		for i, item := range g.items {
			n, e := read(fmt.Sprintf("map_%s_%d", g.prefix, i))
			if e != nil {
				return m, e
			}
			g.values[item.ID] = n
		}
	}
	for i, d := range doc.Days {
		n, e := read(fmt.Sprintf("map_day_%d", i))
		if e != nil || n > 7 {
			if e != nil {
				return m, e
			}
			return m, academic.ValidationError("Hari tujuan tidak valid.")
		}
		m.Days[d.ID] = int(n)
	}
	for i, key := range doc.PeriodKeys() {
		n, e := read(fmt.Sprintf("map_period_%d", i))
		if e != nil || n > 30 {
			if e != nil {
				return m, e
			}
			return m, academic.ValidationError("Jam tujuan tidak valid.")
		}
		m.Periods[key] = int(n)
	}
	return m, nil
}
func ascMappingGroups(doc academic.ASCDocument, m academic.ASCMapping, classes, teachers, subjects []academic.TeachingOption, p academic.SchedulePlan) []ascMapGroup {
	var groups []ascMapGroup
	for _, g := range []struct {
		label, prefix string
		items         []academic.ASCItem
		mapping       map[string]int64
		targets       []academic.TeachingOption
	}{{"Kelas", "class", doc.Classes, m.Classes, classes}, {"Guru", "teacher", doc.Teachers, m.Teachers, teachers}, {"Mata pelajaran", "subject", doc.Subjects, m.Subjects, subjects}} {
		group := ascMapGroup{Label: g.label}
		var options []ascMapOption
		for _, t := range g.targets {
			options = append(options, ascMapOption{ID: t.ID, Name: t.Name})
		}
		for i, item := range g.items {
			group.Rows = append(group.Rows, ascMapRow{Field: fmt.Sprintf("map_%s_%d", g.prefix, i), Source: item.Name, Selected: g.mapping[item.ID], Options: options})
		}
		groups = append(groups, group)
	}
	dayGroup := ascMapGroup{Label: "Hari"}
	names := []string{"", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}
	dayFields := map[string]string{}
	for i, d := range doc.Days {
		var options []ascMapOption
		for day := 1; day <= 7; day++ {
			options = append(options, ascMapOption{ID: int64(day), Name: names[day]})
		}
		field := fmt.Sprintf("map_day_%d", i)
		dayFields[d.ID] = field
		dayGroup.Rows = append(dayGroup.Rows, ascMapRow{Field: field, Source: d.Name, Detail: "Kode hari " + d.ID, Selected: int64(m.Days[d.ID]), Options: options})
	}
	groups = append(groups, dayGroup)
	periodGroup := ascMapGroup{Label: "Nomor jam per hari"}
	for i, key := range doc.PeriodKeys() {
		parts := strings.Split(key, "/")
		number, _ := strconv.Atoi(parts[1])
		var options []ascMapOption
		for _, lp := range p.Periods {
			blocked := false
			for _, b := range p.Blocks {
				if lp.Weekday == b.Weekday && lp.StartsAt < b.EndsAt && lp.EndsAt > b.StartsAt {
					blocked = true
					break
				}
			}
			if !blocked {
				options = append(options, ascMapOption{ID: int64(lp.Number), Day: lp.Weekday, Name: fmt.Sprintf("Jam %d · %s–%s", lp.Number, lp.StartsAt[:5], lp.EndsAt[:5])})
			}
		}
		detail := ""
		for _, sp := range doc.Periods {
			if sp.Number == number {
				detail = sp.StartsAt + "–" + sp.EndsAt
				break
			}
		}
		dayName := ""
		for _, d := range doc.Days {
			if d.ID == parts[0] {
				dayName = d.Name
				break
			}
		}
		periodGroup.Rows = append(periodGroup.Rows, ascMapRow{Field: fmt.Sprintf("map_period_%d", i), Source: fmt.Sprintf("%s · Jam %d", dayName, number), Detail: detail, Selected: int64(m.Periods[key]), DayField: dayFields[parts[0]], SelectedDay: m.Days[parts[0]], SourceNumber: number, Options: options})
	}
	groups = append(groups, periodGroup)
	return groups
}
func (a *application) timetableXMLImport(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	semesters, err := a.options.Academics.Semesters(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	semester := selectedSemester(r, semesters)
	v := view{Page: "timetable-xml-import", Title: "Impor jadwal XML aSc", Academic: &academicView{Semesters: semesters, SelectedSemesterID: semester}, ASCImport: &ascImportView{}}
	token := r.URL.Query().Get("token")
	renderError := func(e error, modal bool) {
		code, msg := academicError(e)
		v.Error = msg
		v.ModalOpen = modal
		v.ModalID = "asc-upload-modal"
		a.render(w, r, "page", code, v)
	}
	if token == "" {
		v.ASCImport.Date = time.Now().Format("2006-01-02")
		if semester > 0 {
			rev, _, _, e := a.options.Academics.Timetable(r.Context(), actor, semester)
			if e != nil {
				a.academicFailure(w, e)
				return
			}
			if rev.Status == "draft" {
				v.ASCImport.DateLocked = true
				v.ASCImport.Date = rev.EffectiveFrom.Format("2006-01-02")
			} else if rev.EffectiveFrom.Format("2006-01-02") >= v.ASCImport.Date {
				v.ASCImport.Date = rev.EffectiveFrom.AddDate(0, 0, 1).Format("2006-01-02")
			}
		}
		v.ModalOpen = semester > 0
		v.ModalID = "asc-upload-modal"
		if r.Method == http.MethodPost {
			if err = r.ParseMultipartForm(1 << 20); err != nil {
				renderError(academic.ValidationError("Unggahan tidak valid atau melebihi batas ukuran."), true)
				return
			}
			date, e := time.Parse("2006-01-02", r.FormValue("effective_from"))
			if e != nil {
				renderError(academic.ValidationError("Tanggal berlaku tidak valid."), true)
				return
			}
			v.ASCImport.Date = date.Format("2006-01-02")
			if r.MultipartForm == nil || len(r.MultipartForm.File["file"]) != 1 {
				renderError(academic.ValidationError("Pilih satu file XML jadwal."), true)
				return
			}
			file, header, e := r.FormFile("file")
			if e != nil {
				renderError(academic.ValidationError("File belum dapat dibaca."), true)
				return
			}
			defer file.Close()
			if !strings.EqualFold(filepath.Ext(header.Filename), ".xml") || header.Size > academic.MaxASCBytes {
				renderError(academic.ValidationError("Gunakan file .xml maksimal 5 MiB."), true)
				return
			}
			doc, e := academic.ParseASCXML(file)
			if e != nil {
				renderError(e, true)
				return
			}
			stage, e := a.options.Academics.StageASCImport(r.Context(), actor, semester, date, header.Filename, doc)
			if e != nil {
				renderError(e, true)
				return
			}
			http.Redirect(w, r, "/akademik/jadwal/impor?token="+stage.Token, http.StatusSeeOther)
			return
		}
		if semester == 0 {
			v.Message = "Siapkan semester terlebih dahulu."
		}
		a.render(w, r, "page", http.StatusOK, v)
		return
	}
	stage, report, err := a.options.Academics.ASCImportPreview(r.Context(), actor, token)
	if err != nil {
		renderError(err, false)
		return
	}
	if stage.State == "complete" {
		http.Redirect(w, r, fmt.Sprintf("/akademik/jadwal?semester_id=%d&revision_id=%d&saved=1", stage.SemesterID, stage.RevisionID), http.StatusSeeOther)
		return
	}
	v.Academic.SelectedSemesterID = stage.SemesterID
	if r.Method == http.MethodPost {
		if e := r.ParseForm(); e != nil {
			renderError(academic.ValidationError("Form pemetaan tidak valid atau terlalu besar."), false)
			return
		}
		version := formInt64(r, "version")
		switch r.PostForm.Get("action") {
		case "preview":
			mapping, e := ascFormMapping(r, stage.Payload.Document)
			if e == nil {
				stage, report, e = a.options.Academics.PreviewASCMapping(r.Context(), actor, token, version, mapping)
			}
			err = e
		case "confirm":
			var id int64
			id, err = a.options.Academics.ConfirmASCImport(r.Context(), actor, token, version, r.PostForm.Get("replace_draft") == "yes", r.PostForm.Get("allow_incomplete") == "yes", r.PostForm.Get("school_time") == "yes")
			if err == nil {
				http.Redirect(w, r, fmt.Sprintf("/akademik/jadwal?semester_id=%d&revision_id=%d&saved=1", stage.SemesterID, id), http.StatusSeeOther)
				return
			}
		default:
			err = academic.ValidationError("Aksi impor tidak valid.")
		}
	}
	if err != nil {
		if latest, latestReport, e := a.options.Academics.ASCImportPreview(r.Context(), actor, token); e == nil {
			stage, report = latest, latestReport
		}
		report.CanConfirm = false
	}
	// On a conflict keep the latest server preview; never render an empty draft.
	if stage.Token == "" {
		stage, report, _ = a.options.Academics.ASCImportPreview(r.Context(), actor, token)
	}
	v.ASCImport.Stage, v.ASCImport.Report = stage, report
	classes, teachers, subjects, e := a.options.Academics.TeachingOptions(r.Context(), actor, stage.SemesterID)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	p, e := a.options.Academics.SchedulePlan(r.Context(), actor, stage.SemesterID, stage.Payload.Date)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	v.ASCImport.Groups = ascMappingGroups(stage.Payload.Document, stage.Payload.Mapping, classes, teachers, subjects, p)
	code := http.StatusOK
	if err != nil {
		code, v.Error = academicError(err)
	}
	a.render(w, r, "page", code, v)
}
