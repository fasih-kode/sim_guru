package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/fasih-kode/sim_guru/internal/master"
)

type masterView struct {
	Kind           master.Kind
	Records        []master.Record
	Input          master.Input
	Record         master.Record
	History        []master.History
	Accounts       []master.Account
	Filter         master.Filter
	CanWrite       bool
	Previous, Next int
}

func masterError(err error) (int, string) {
	var validation master.ValidationError
	switch {
	case errors.As(err, &validation):
		return 422, err.Error()
	case errors.Is(err, master.ErrForbidden):
		return 403, err.Error()
	case errors.Is(err, master.ErrNotFound):
		return 404, err.Error()
	case errors.Is(err, master.ErrConflict):
		return 409, err.Error()
	default:
		return 500, "Data belum dapat diproses. Silakan coba kembali."
	}
}
func (a *application) masterAccess(w http.ResponseWriter, r *http.Request, write bool) (master.Kind, bool) {
	kind := master.Kind(r.PathValue("kind"))
	if !kind.Valid() {
		http.NotFound(w, r)
		return kind, false
	}
	u := principal(r)
	allowed := u.HasRole(kind.WriterRole())
	if !write && kind != master.Subjects && kind != master.Staff {
		allowed = allowed || u.HasRole("kurikulum")
	}
	if !allowed {
		http.Error(w, master.ErrForbidden.Error(), 403)
		return kind, false
	}
	return kind, true
}
func masterID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}
func (a *application) masterList(w http.ResponseWriter, r *http.Request) {
	kind, ok := a.masterAccess(w, r, false)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	f := master.Filter{Search: r.URL.Query().Get("q"), Status: r.URL.Query().Get("status"), Page: page}
	if err := f.Validate(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	records, err := a.options.Masters.List(r.Context(), principal(r).ID, kind, f)
	if err != nil {
		code, message := masterError(err)
		http.Error(w, message, code)
		return
	}
	mv := &masterView{Kind: kind, Records: records, Filter: f, CanWrite: principal(r).HasRole(kind.WriterRole())}
	if f.Page > 1 {
		mv.Previous = f.Page - 1
	}
	if len(records) > 10 {
		mv.Next = f.Page + 1
		records = records[:10]
		mv.Records = records
	}
	v := view{Page: "master-list", Title: "Data " + kind.Label(), Master: mv}
	if r.URL.Query().Get("saved") == "1" {
		v.Message = "Data dan histori perubahan tersimpan."
	}
	a.render(w, r, "page", 200, v)
}
func (a *application) masterForm(w http.ResponseWriter, r *http.Request) {
	kind, ok := a.masterAccess(w, r, true)
	if !ok {
		return
	}
	id := int64(0)
	if r.PathValue("id") != "" {
		id, ok = masterID(w, r)
		if !ok {
			return
		}
	}
	mv := &masterView{Kind: kind, CanWrite: true, Input: master.Input{Record: master.Record{Active: true}}}
	if id != 0 {
		record, err := a.options.Masters.Get(r.Context(), principal(r).ID, kind, id)
		if err != nil {
			code, message := masterError(err)
			http.Error(w, message, code)
			return
		}
		mv.Input.Record = record
	}
	if kind == master.Teachers || kind == master.Staff {
		accounts, err := a.options.Masters.Accounts(r.Context(), principal(r).ID)
		if err != nil {
			a.failure(w, err)
			return
		}
		mv.Accounts = accounts
	}
	filter := master.Filter{Page: 1}
	records, err := a.options.Masters.List(r.Context(), principal(r).ID, kind, filter)
	if err != nil {
		a.failure(w, err)
		return
	}
	if len(records) > 10 {
		records = records[:10]
	}
	mv.Records = records
	v := view{Page: "master-list", Title: "Data " + kind.Label(), Master: mv, ModalOpen: true}
	status := 200
	if r.Method == "POST" {
		if !parseForm(w, r) {
			return
		}
		version, _ := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
		userID, err := strconv.ParseInt(r.PostForm.Get("user_id"), 10, 64)
		if r.PostForm.Get("user_id") == "" {
			userID = 0
			err = nil
		}
		if err != nil {
			http.Error(w, "Akun terkait tidak valid.", 422)
			return
		}
		mv.Input = master.Input{Record: master.Record{ID: id, Name: r.PostForm.Get("name"), BirthDate: r.PostForm.Get("birth_date"), Gender: r.PostForm.Get("gender"), NIS: r.PostForm.Get("nis"), NISN: r.PostForm.Get("nisn"), NIP: r.PostForm.Get("nip"), NUPTK: r.PostForm.Get("nuptk"), EmployeeNumber: r.PostForm.Get("employee_number"), Email: r.PostForm.Get("email"), Phone: r.PostForm.Get("phone"), UserID: userID, Code: r.PostForm.Get("code"), Active: r.PostForm.Get("active") == "true", Version: version}, Reason: r.PostForm.Get("reason")}
		_, err = a.options.Masters.Save(r.Context(), principal(r).ID, kind, id, mv.Input)
		if err == nil {
			http.Redirect(w, r, "/master/"+string(kind)+"?saved=1", 303)
			return
		}
		status, v.Error = masterError(err)
	}
	a.render(w, r, "page", status, v)
}
func (a *application) masterDetail(w http.ResponseWriter, r *http.Request) {
	kind, ok := a.masterAccess(w, r, false)
	if !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	record, err := a.options.Masters.Get(r.Context(), principal(r).ID, kind, id)
	if err != nil {
		code, message := masterError(err)
		http.Error(w, message, code)
		return
	}
	a.render(w, r, "page", 200, view{Page: "master-detail", Title: "Detail " + kind.Label(), Master: &masterView{Kind: kind, Record: record, CanWrite: principal(r).HasRole(kind.WriterRole())}})
}
func (a *application) masterHistory(w http.ResponseWriter, r *http.Request) {
	kind, ok := a.masterAccess(w, r, true)
	if !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	records, err := a.options.Masters.History(r.Context(), principal(r).ID, kind, id)
	if err != nil {
		code, message := masterError(err)
		http.Error(w, message, code)
		return
	}
	for i := range records {
		records[i].At = records[i].At.In(time.FixedZone("WIB", 7*3600))
	}
	a.render(w, r, "page", 200, view{Page: "master-history", Title: "Histori " + kind.Label(), Master: &masterView{Kind: kind, History: records, Record: master.Record{ID: id}, CanWrite: true}})
}
