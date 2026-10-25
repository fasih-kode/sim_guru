package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

func quickReason(kind, note string) (string, error) {
	labels := map[string]string{"manual": "Pencatatan manual kelas", "scanner": "Pemindai bermasalah", "card": "Kartu QR hilang", "other": "Lainnya"}
	label, ok := labels[kind]
	if !ok {
		return "", academic.ValidationError("Pilih alasan pencatatan kelompok.")
	}
	note = strings.TrimSpace(note)
	if kind == "other" && note == "" {
		return "", academic.ValidationError("Isi alasan lainnya.")
	}
	if note != "" {
		label += " — " + note
	}
	return label, nil
}
func (a *application) dailyQuick(w http.ResponseWriter, r *http.Request) {
	s := a.options.Academics
	actor := principal(r).ID
	classes, e := s.DailyClasses(r.Context(), actor)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	d := &dailyView{Classes: classes, Date: r.URL.Query().Get("date"), QuickRows: map[int64]academic.DailyQuickRow{}}
	if d.Date == "" {
		d.Date = dailyToday()
	}
	d.ClassID, _ = strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
	if d.ClassID == 0 && len(classes) > 0 {
		d.ClassID = classes[0].ID
	}
	v := view{Page: "daily-quick", Title: "Isi Cepat Absensi Harian", Daily: d}
	code := 200
	if d.ClassID == 0 {
		a.render(w, r, "page", 200, v)
		return
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
		if e = r.ParseForm(); e != nil {
			http.Error(w, "Form terlalu besar atau tidak valid.", 400)
			return
		}
		d.CommonTime = r.PostForm.Get("common_time")
		d.ReasonKind = r.PostForm.Get("reason_kind")
		d.Reason = r.PostForm.Get("reason_note")
		reason, err := quickReason(d.ReasonKind, d.Reason)
		in := academic.DailyQuickInput{ClassID: d.ClassID, Date: d.Date, Reason: reason, Fingerprint: r.PostForm.Get("fingerprint")}
		for _, raw := range r.PostForm["student_ids"] {
			id, er := strconv.ParseInt(raw, 10, 64)
			if er != nil {
				err = academic.ValidationError("Pilihan siswa tidak valid.")
				break
			}
			row := academic.DailyQuickRow{StudentID: id, Status: r.PostForm.Get("status_" + raw), Arrival: r.PostForm.Get("arrival_" + raw)}
			in.Rows = append(in.Rows, row)
			d.QuickRows[id] = row
		}
		if err != nil {
			e = err
		} else {
			_, e = s.SaveQuickDaily(r.Context(), actor, in)
		}
		if e == nil {
			http.Redirect(w, r, fmt.Sprintf("/absensi/harian?class_id=%d&date=%s&saved=1", d.ClassID, d.Date), 303)
			return
		}
		code, v.Error = academicError(e)
		if code >= 500 {
			a.failure(w, e)
			return
		}
		if code == 409 {
			v.Error = "Data kelas, aturan, atau absensi berubah. Isian Anda tetap ditampilkan; periksa ulang baris yang kini terkunci sebelum menyimpan lagi."
		}
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
	if r.Method == http.MethodPost && r.PostForm.Get("fingerprint") != "" && r.PostForm.Get("fingerprint") != d.Sheet.QuickFingerprint && code != http.StatusConflict {
		code = http.StatusConflict
		v.Error += " Data kelas, aturan, atau absensi juga berubah. Isian tetap ditampilkan; periksa ulang baris terkunci dan pilihan sebelum menyimpan lagi."
	}
	a.render(w, r, "page", code, v)
}
