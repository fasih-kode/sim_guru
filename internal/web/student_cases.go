package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fasih-kode/sim_guru/internal/studentcases"
)

type studentCasesView struct {
	List              studentcases.List
	Detail            studentcases.Detail
	Filter            studentcases.Filter
	Students          []studentcases.CaseStudentOption
	BKOptions         []studentcases.UserOption
	KesOptions        []studentcases.UserOption
	Previous          int
	Next              int
	NewModal          bool
	ErrorTargetUserID int64
	ErrorReason       string
}

func (a *application) studentCases(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	filter := studentcases.Filter{Page: page, Search: r.URL.Query().Get("q"), Stage: studentcases.Stage(r.URL.Query().Get("stage")), Status: studentcases.Status(r.URL.Query().Get("status"))}
	list, err := a.options.Academics.ListStudentCases(r.Context(), actor, filter)
	if err != nil {
		a.studentCaseFailure(w, r, err)
		return
	}
	data := &studentCasesView{List: list, Filter: filter}
	if page > 1 {
		data.Previous = page - 1
	}
	if list.HasNext {
		data.Next = list.Page + 1
	}
	if r.URL.Query().Get("new") == "1" {
		if !principal(r).HasRole(studentcases.RoleHomeroom) {
			http.Error(w, "Akses ditolak.", http.StatusForbidden)
			return
		}
		data.Students, err = a.options.Academics.StudentCaseStudents(r.Context(), actor)
		if err != nil {
			a.studentCaseFailure(w, r, err)
			return
		}
		data.NewModal = true
	}
	pageView := view{Page: "student-cases", Title: "Kasus Siswa", StudentCases: data}
	if r.URL.Query().Get("forwarded") == "1" {
		pageView.Message = "Kasus berhasil diteruskan kepada petugas Kesiswaan. Daftar ini hanya menampilkan kasus dalam cakupan akses Anda."
	} else if r.URL.Query().Get("saved") == "1" {
		pageView.Message = "Perubahan kasus berhasil disimpan."
	}
	a.render(w, r, "page", http.StatusOK, pageView)
}

func (a *application) studentCaseDetail(w http.ResponseWriter, r *http.Request) {
	id, err := studentCaseID(r)
	if err != nil {
		a.studentCaseFailure(w, r, studentcases.ErrNotFound)
		return
	}
	detail, err := a.options.Academics.GetStudentCase(r.Context(), principal(r).ID, id)
	if err != nil {
		a.studentCaseFailure(w, r, err)
		return
	}
	data := &studentCasesView{Detail: detail}
	if detail.CanReferToBK {
		data.BKOptions, err = a.options.Academics.StudentCaseBKOptions(r.Context(), principal(r).ID)
	} else if detail.CanReferToKes {
		data.KesOptions, err = a.options.Academics.StudentCaseKesiswaanOptions(r.Context(), principal(r).ID)
	}
	if err != nil {
		a.studentCaseFailure(w, r, err)
		return
	}
	pageView := view{Page: "student-case-detail", Title: "Detail Kasus Siswa", StudentCases: data}
	if r.URL.Query().Get("taken-over") == "1" {
		pageView.Message = "Anda berhasil mengambil alih penanganan kasus. Histori sebelumnya tetap tersimpan."
	}
	a.render(w, r, "page", http.StatusOK, pageView)
}

func (a *application) createStudentCase(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	_, err := a.options.Academics.CreateStudentCase(r.Context(), principal(r).ID, studentcases.CreateInput{StudentID: formInt64(r, "student_id"), Content: r.PostForm.Get("content")})
	if err != nil {
		a.studentCaseFailure(w, r, err)
		return
	}
	http.Redirect(w, r, "/kesiswaan/kasus?saved=1", http.StatusSeeOther)
}

func (a *application) addStudentCaseEntry(w http.ResponseWriter, r *http.Request) {
	a.caseMutation(w, r, func(id, version int64) error {
		return a.options.Academics.AddStudentCaseEntry(r.Context(), principal(r).ID, id, version, studentcases.EntryInput{Content: r.PostForm.Get("content")})
	})
}

func (a *application) takeOverStudentCase(w http.ResponseWriter, r *http.Request) {
	a.caseMutationTo(w, r, func(id, version int64) error {
		return a.options.Academics.TakeOverStudentCase(r.Context(), principal(r).ID, id, version, studentcases.HandoffInput{Reason: r.PostForm.Get("reason")})
	}, func(id int64) string { return "/kesiswaan/kasus/" + strconv.FormatInt(id, 10) + "?taken-over=1" })
}

func (a *application) addStudentCaseCounselingNote(w http.ResponseWriter, r *http.Request) {
	a.caseMutation(w, r, func(id, version int64) error {
		return a.options.Academics.AddStudentCaseCounselingNote(r.Context(), principal(r).ID, id, version, studentcases.CounselingInput{Content: r.PostForm.Get("content")})
	})
}

func (a *application) referStudentCaseToBK(w http.ResponseWriter, r *http.Request) {
	a.caseMutation(w, r, func(id, version int64) error {
		return a.options.Academics.ReferStudentCaseToBK(r.Context(), principal(r).ID, id, version, studentcases.ReferralInput{TargetUserID: formInt64(r, "target_user_id"), Reason: r.PostForm.Get("reason")})
	})
}

func (a *application) referStudentCaseToKesiswaan(w http.ResponseWriter, r *http.Request) {
	a.caseMutation(w, r, func(id, version int64) error {
		return a.options.Academics.ReferStudentCaseToKesiswaan(r.Context(), principal(r).ID, id, version, studentcases.ReferralInput{TargetUserID: formInt64(r, "target_user_id"), Reason: r.PostForm.Get("reason")})
	}, func(int64) string { return "/kesiswaan/kasus?forwarded=1" })
}

func (a *application) changeStudentCaseStatus(w http.ResponseWriter, r *http.Request) {
	a.caseMutation(w, r, func(id, version int64) error {
		return a.options.Academics.ChangeStudentCaseStatus(r.Context(), principal(r).ID, id, version, studentcases.StatusInput{Status: studentcases.Status(r.PostForm.Get("status")), Reason: r.PostForm.Get("reason")})
	})
}

func (a *application) caseMutation(w http.ResponseWriter, r *http.Request, mutate func(id, version int64) error, redirect ...func(int64) string) {
	destination := func(id int64) string { return "/kesiswaan/kasus/" + strconv.FormatInt(id, 10) + "?saved=1" }
	if len(redirect) > 0 {
		destination = redirect[0]
	}
	a.caseMutationTo(w, r, mutate, destination)
}

func (a *application) caseMutationTo(w http.ResponseWriter, r *http.Request, mutate func(id, version int64) error, successLocation func(id int64) string) {
	if !parseForm(w, r) {
		return
	}
	id, err := studentCaseID(r)
	if err != nil {
		a.studentCaseFailure(w, r, studentcases.ErrNotFound)
		return
	}
	version, err := strconv.ParseInt(r.PostForm.Get("version"), 10, 64)
	if err != nil || version < 1 {
		a.studentCaseMutationFailure(w, r, id, studentcases.ErrConflict)
		return
	}
	if err = mutate(id, version); err != nil {
		a.studentCaseMutationFailure(w, r, id, err)
		return
	}
	location := successLocation(id)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", location)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func studentCaseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid case id")
	}
	return id, nil
}

func (a *application) studentCaseFailure(w http.ResponseWriter, r *http.Request, err error) {
	status, message := studentCaseError(err)
	a.render(w, r, "page", status, view{Page: "student-case-error", Title: "Kasus Siswa", Error: message})
}

func studentCaseError(err error) (int, string) {
	status, message := http.StatusInternalServerError, "Kasus belum dapat diproses. Coba lagi atau hubungi Admin sekolah."
	switch {
	case errors.Is(err, studentcases.ErrInvalid), errors.Is(err, studentcases.ErrScopeUnclear), errors.Is(err, studentcases.ErrNoBKAvailable), errors.Is(err, studentcases.ErrNoKesAvailable):
		status, message = http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, studentcases.ErrForbidden):
		status, message = http.StatusForbidden, "Akses kasus ditolak."
	case errors.Is(err, studentcases.ErrNotFound):
		status, message = http.StatusNotFound, "Kasus tidak ditemukan atau tidak dapat diakses."
	case errors.Is(err, studentcases.ErrConflict):
		status, message = http.StatusConflict, err.Error()
	}
	return status, message
}

func (a *application) studentCaseMutationFailure(w http.ResponseWriter, r *http.Request, caseID int64, err error) {
	status, message := studentCaseError(err)
	detail, readErr := a.options.Academics.GetStudentCase(r.Context(), principal(r).ID, caseID)
	if readErr != nil {
		a.studentCaseFailure(w, r, err)
		return
	}
	data := &studentCasesView{Detail: detail, ErrorTargetUserID: formInt64(r, "target_user_id"), ErrorReason: r.PostForm.Get("reason")}
	if detail.CanReferToBK {
		data.BKOptions, readErr = a.options.Academics.StudentCaseBKOptions(r.Context(), principal(r).ID)
	} else if detail.CanReferToKes {
		data.KesOptions, readErr = a.options.Academics.StudentCaseKesiswaanOptions(r.Context(), principal(r).ID)
	}
	if readErr != nil {
		a.studentCaseFailure(w, r, err)
		return
	}
	page := view{Page: "student-case-detail", Title: "Detail Kasus Siswa", StudentCases: data, Error: message}
	if r.Header.Get("HX-Request") == "true" {
		a.render(w, r, "student-case-detail", status, page)
		return
	}
	a.render(w, r, "page", status, page)
}
