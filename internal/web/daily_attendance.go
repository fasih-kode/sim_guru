package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/skip2/go-qrcode"
)

type dailyChange struct{ Before, After, Actor, Reason, At string }

type dailyView struct {
	CommonTime     string
	QuickRows      map[int64]academic.DailyQuickRow
	ReasonKind     string
	Changes        []dailyChange
	CardClasses    []string
	Classes        []academic.Class
	ClassID        int64
	Date           string
	Sheet          academic.DailySheet
	Editing        academic.DailyStudent
	Input          academic.DailyInput
	History        []academic.History
	Cards          []academic.StudentCard
	Card           academic.StudentCard
	CardImage      template.URL
	Action, Reason string
}

func dailyToday() string { return time.Now().In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02") }
func (a *application) dailyAttendance(w http.ResponseWriter, r *http.Request) {
	s := a.options.Academics
	actor := principal(r).ID
	classes, e := s.DailyClasses(r.Context(), actor)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	d := &dailyView{Classes: classes, Date: r.URL.Query().Get("date")}
	if d.Date == "" {
		d.Date = dailyToday()
	}
	d.ClassID, _ = strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
	if d.ClassID == 0 && len(classes) > 0 {
		d.ClassID = classes[0].ID
	}
	v := view{Page: "daily-attendance", Title: "Absensi Harian Siswa", Daily: d}
	code := 200
	if d.ClassID == 0 {
		a.render(w, r, "page", 200, v)
		return
	}
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		in := academic.DailyInput{ClassID: d.ClassID, Date: d.Date, StudentID: formInt64(r, "student_id"), Version: formInt64(r, "version"), Status: r.PostForm.Get("status"), Arrival: r.PostForm.Get("arrival"), Reason: r.PostForm.Get("reason"), Fingerprint: r.PostForm.Get("fingerprint")}
		if r.PostForm.Get("action") == "now" {
			in.Now = true
			in.Status = "hadir"
			in.Reason = "Kedatangan dicatat langsung oleh petugas"
		}
		d.Input = in
		_, e = s.SaveDailyAttendance(r.Context(), actor, in)
		if e == nil {
			http.Redirect(w, r, fmt.Sprintf("/absensi/harian?class_id=%d&date=%s&saved=1", d.ClassID, d.Date), 303)
			return
		}
		code, v.Error = academicError(e)
		if code >= 500 {
			a.failure(w, e)
			return
		}
		v.ModalOpen = true
	}
	d.Sheet, e = s.DailyAttendance(r.Context(), actor, d.ClassID, d.Date)
	if e != nil {
		status, message := academicError(e)
		if status >= 500 {
			a.failure(w, e)
			return
		}
		v.Error = message
		a.render(w, r, "page", status, v)
		return
	}
	edit, _ := strconv.ParseInt(r.URL.Query().Get("edit"), 10, 64)
	if r.Method == http.MethodPost {
		edit = d.Input.StudentID
	}
	if edit > 0 {
		found := false
		for _, row := range d.Sheet.Students {
			if row.StudentID == edit {
				d.Editing = row
				found = true
				break
			}
		}
		if !found {
			a.academicFailure(w, academic.ErrNotFound)
			return
		}
		if r.Method == http.MethodGet {
			d.Input = academic.DailyInput{StudentID: edit, Version: d.Editing.Version, Status: d.Editing.Status, Arrival: d.Editing.Arrival, Fingerprint: d.Sheet.Fingerprint}
		}
		if d.Editing.ID > 0 {
			d.History, e = s.DailyHistory(r.Context(), actor, d.Editing.ID)
			if e != nil {
				a.academicFailure(w, e)
				return
			}
			for _, h := range d.History {
				before, err := dailyHistoryLabel(h.Before)
				if err != nil {
					a.failure(w, err)
					return
				}
				after, err := dailyHistoryLabel(h.After)
				if err != nil {
					a.failure(w, err)
					return
				}
				d.Changes = append(d.Changes, dailyChange{Before: before, After: after, Actor: h.Actor, Reason: h.Reason, At: h.At.Format("02 Jan 2006 15:04 MST")})
			}
		}
		v.ModalOpen = true
	}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Absensi harian tersimpan beserta histori."
	}
	a.render(w, r, "page", code, v)
}
func (a *application) dailyScanner(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, "page", 200, view{Page: "daily-scanner", Title: "Pemindai Kartu Siswa", Daily: &dailyView{Date: dailyToday()}})
}
func (a *application) dailyScan(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !parseForm(w, r) {
		return
	}
	result, e := a.options.Academics.ScanStudentQR(r.Context(), principal(r).ID, r.PostForm.Get("token"))
	if e != nil {
		code, message := academicError(e)
		if code >= 500 {
			message = "Pencatatan gagal. Coba lagi atau gunakan pencatatan manual."
		}
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": message})
		return
	}
	message := "Kedatangan berhasil dicatat."
	if result.Duplicate {
		message = "Sudah tercatat Hadir; waktu kedatangan sebelumnya tetap dipakai."
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": message, "name": result.Student.Name, "class": result.Student.ClassName, "arrival": result.Student.Arrival, "date": result.Student.Date, "punctuality": result.Student.PunctualityLabel(), "duplicate": result.Duplicate})
}
func (a *application) studentCards(w http.ResponseWriter, r *http.Request) {
	s := a.options.Academics
	actor := principal(r).ID
	d := &dailyView{}
	v := view{Page: "student-cards", Title: "Kartu QR Siswa", Daily: d}
	code := 200
	edit, _ := strconv.ParseInt(r.URL.Query().Get("edit"), 10, 64)
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		edit = formInt64(r, "student_id")
		d.Action = r.PostForm.Get("action")
		d.Reason = r.PostForm.Get("reason")
		var e error
		if r.PostForm.Get("confirmed") != "yes" {
			e = academic.ValidationError("Periksa ringkasan lalu centang konfirmasi kartu.")
		} else {
			_, e = s.ManageStudentCard(r.Context(), actor, edit, formInt64(r, "card_id"), d.Action, d.Reason)
		}
		if e == nil {
			http.Redirect(w, r, fmt.Sprintf("/absensi/kartu/%d/cetak", edit), 303)
			return
		}
		code, v.Error = academicError(e)
		if code >= 500 {
			a.failure(w, e)
			return
		}
		v.ModalOpen = true
	}
	var e error
	d.Cards, e = s.StudentCards(r.Context(), actor)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	seen := map[string]bool{}
	for _, c := range d.Cards {
		if !seen[c.ClassName] {
			d.CardClasses = append(d.CardClasses, c.ClassName)
			seen[c.ClassName] = true
		}
	}
	if edit > 0 {
		d.Card, e = s.StudentCard(r.Context(), actor, edit)
		if e != nil {
			a.academicFailure(w, e)
			return
		}
		d.Card.Token = ""
		v.ModalOpen = true
	}
	a.render(w, r, "page", code, v)
}
func (a *application) printStudentCard(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil {
		a.academicFailure(w, academic.ErrNotFound)
		return
	}
	c, e := a.options.Academics.StudentCard(r.Context(), principal(r).ID, id)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	if c.CardID == 0 {
		a.academicFailure(w, academic.ErrNotFound)
		return
	}
	png, e := qrcode.Encode(c.Token, qrcode.Medium, 320)
	if e != nil {
		a.failure(w, e)
		return
	}
	c.Token = ""
	w.Header().Set("Referrer-Policy", "no-referrer")
	a.render(w, r, "page", 200, view{Page: "student-card-print", Title: "Cetak Kartu QR Siswa", Daily: &dailyView{Card: c, CardImage: template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png))}})
}

func dailyHistoryLabel(raw string) (string, error) {
	if raw == "" || raw == "null" {
		return "Belum Dicatat", nil
	}
	var d academic.DailyStudent
	if e := json.Unmarshal([]byte(raw), &d); e != nil {
		return "", e
	}
	arrival := d.Arrival
	if arrival == "" {
		arrival = "tidak dicatat"
	} else {
		arrival += " WIB"
	}
	label := fmt.Sprintf("%s · datang %s · %s · kelas %s", d.StatusLabel(), arrival, d.SourceLabel(), d.ClassName) + " · " + d.PunctualityLabel()
	if d.Policy != nil {
		label += fmt.Sprintf(" (batas %s WIB, toleransi %d menit)", d.Policy.Deadline, d.Policy.Tolerance)
	}
	return label, nil
}
