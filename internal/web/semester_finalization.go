package web

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type semesterFinalView struct {
	Selected                     academic.LearningObjectiveContext
	Preview                      academic.SemesterFinalPreview
	Score, Reason, SubmissionKey string
	URL, ReturnURL               string
	StaleRequest                 bool
	Exception                    bool
}

func (a *application) semesterFinalization(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	values := r.URL.Query()
	if r.Method == http.MethodPost {
		if !parseForm(w, r) {
			return
		}
		values = r.PostForm
	}
	semesterID, classID, subjectID, selected, err := learningObjectiveSelectionValues(values)
	if err != nil || !selected {
		a.academicFailure(w, academic.ValidationError("Konteks semester, kelas, dan mata pelajaran tidak valid."))
		return
	}
	studentID, e := strconv.ParseInt(values.Get("student_id"), 10, 64)
	if e != nil || studentID < 1 {
		a.academicFailure(w, academic.ValidationError("Siswa tidak valid."))
		return
	}
	historical := values.Get("history") == "1"
	contexts, err := a.options.Academics.LearningObjectiveContexts(r.Context(), actor)
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data := &semesterFinalView{}
	data.URL = semesterFinalURL(semesterID, classID, subjectID, studentID)
	data.ReturnURL = "/guru/rekomendasi-semester?context=" + url.QueryEscape(fmt.Sprintf("%d:%d:%d", semesterID, classID, subjectID))
	if historical {
		data.URL += "&history=1"
		data.ReturnURL = "/guru/rekomendasi-semester"
		owned, loadErr := a.options.Academics.OwnedSemesterFinals(r.Context(), actor)
		if loadErr != nil {
			a.academicFailure(w, loadErr)
			return
		}
		for _, item := range owned {
			if item.SemesterID == semesterID && item.ClassID == classID && item.SubjectID == subjectID && item.StudentID == studentID {
				data.Selected = academic.LearningObjectiveContext{SemesterID: item.SemesterID, ClassID: item.ClassID, SubjectID: item.SubjectID, SemesterName: item.SemesterName, ClassName: item.ClassName, SubjectName: item.SubjectName}
				break
			}
		}
	} else {
		for _, item := range contexts {
			if item.SemesterID == semesterID && item.ClassID == classID && item.SubjectID == subjectID {
				data.Selected = item
				break
			}
		}
	}
	if data.Selected.SemesterID == 0 {
		a.academicFailure(w, academic.ErrForbidden)
		return
	}
	page := func(status int, message string) {
		a.render(w, r, "page", status, view{Page: "semester-finalization", Title: "Pratinjau Final Nilai Semester", Error: message, SemesterFinalization: data})
	}
	loadPreview := func() (academic.SemesterFinalPreview, error) {
		if historical {
			return a.options.Academics.SemesterFinalHistoricalPreview(r.Context(), actor, semesterID, classID, subjectID, studentID)
		}
		return a.options.Academics.SemesterFinalPreview(r.Context(), actor, semesterID, classID, subjectID, studentID)
	}
	if r.Method == http.MethodPost {
		data.Score = values.Get("final_score")
		data.Reason = values.Get("reason")
		data.Exception = values.Get("mode") == "exception"
		data.SubmissionKey = values.Get("submission_key")
		score, parseErr := strconv.Atoi(data.Score)
		if parseErr != nil {
			parseErr = academic.ValidationError("Nilai Final harus berupa bilangan bulat 0–100.")
		}
		if parseErr == nil {
			if historical {
				version, versionErr := strconv.ParseInt(values.Get("expected_version"), 10, 64)
				if versionErr != nil {
					parseErr = academic.ValidationError("Versi Final tidak valid.")
				} else {
					parseErr = a.options.Academics.ReviewSemesterFinal(r.Context(), actor, semesterID, classID, subjectID, studentID, version, score, data.Reason, values.Get("snapshot_token"), data.SubmissionKey, values.Get("action"))
				}
			} else if data.Exception {
				parseErr = a.options.Academics.FinalizeSemesterException(r.Context(), actor, semesterID, classID, subjectID, studentID, score, data.Reason, values.Get("snapshot_token"), data.SubmissionKey)
			} else {
				parseErr = a.options.Academics.FinalizeSemesterNormal(r.Context(), actor, semesterID, classID, subjectID, studentID, score, data.Reason, values.Get("snapshot_token"), data.SubmissionKey)
			}
		}
		if parseErr != nil {
			status, message := academicError(parseErr)
			if status >= 500 {
				a.failure(w, parseErr)
				return
			}
			if errors.Is(parseErr, academic.ErrForbidden) {
				http.Error(w, message, status)
				return
			}
			data.Preview, err = loadPreview()
			if err != nil {
				code, denied := academicError(err)
				http.Error(w, denied, code)
				return
			}
			data.StaleRequest = errors.Is(parseErr, academic.ErrConflict)
			page(status, message)
			return
		}
		if historical {
			q := url.Values{"result": {values.Get("action")}, "submission": {data.SubmissionKey}}
			http.Redirect(w, r, data.URL+"&"+q.Encode(), http.StatusSeeOther)
		} else {
			http.Redirect(w, r, data.URL+"&saved=1", http.StatusSeeOther)
		}
		return
	}
	data.Preview, err = loadPreview()
	if err != nil {
		a.academicFailure(w, err)
		return
	}
	data.Exception = data.Preview.CanFinalizeException
	if !data.Preview.ReviewUncertain && (data.Preview.CanFinalize || data.Exception || historical) {
		if data.Preview.CanFinalize {
			data.Score = data.Preview.Student.Recommendation
		}
		if historical {
			data.Score = data.Preview.FinalValue
		}
		data.SubmissionKey, err = newFinalSubmissionKey()
		if err != nil {
			a.failure(w, err)
			return
		}
	}
	if historical {
		action, key := r.URL.Query().Get("result"), r.URL.Query().Get("submission")
		found, resultErr := a.options.Academics.SemesterFinalReviewResult(r.Context(), actor, semesterID, classID, subjectID, studentID, action, key)
		if resultErr != nil {
			a.academicFailure(w, resultErr)
			return
		}
		if found {
			message := "Nilai Final versi baru berhasil disimpan."
			if action == "retain" {
				message = "Keputusan mempertahankan Nilai Final lama berhasil dicatat."
			}
			a.render(w, r, "page", http.StatusOK, view{Page: "semester-finalization", Title: "Pratinjau Final Nilai Semester", Message: message, SemesterFinalization: data})
			return
		}
	}
	if !historical && r.URL.Query().Get("saved") == "1" && data.Preview.Finalized {
		a.render(w, r, "page", http.StatusOK, view{Page: "semester-finalization", Title: "Pratinjau Final Nilai Semester", Message: "Final Nilai Semester berhasil disimpan.", SemesterFinalization: data})
		return
	}
	page(http.StatusOK, "")
}

func learningObjectiveSelectionValues(values url.Values) (int64, int64, int64, bool, error) {
	parts := strings.Split(values.Get("context"), ":")
	if len(parts) != 3 {
		return 0, 0, 0, false, academic.ValidationError("Pilih konteks semester, kelas, dan mata pelajaran.")
	}
	semester, err1 := strconv.ParseInt(parts[0], 10, 64)
	class, err2 := strconv.ParseInt(parts[1], 10, 64)
	subject, err3 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || semester < 1 || class < 1 || subject < 1 {
		return 0, 0, 0, false, academic.ValidationError("Konteks semester, kelas, dan mata pelajaran tidak valid.")
	}
	return semester, class, subject, true, nil
}

func semesterFinalURL(semester, class, subject, student int64) string {
	q := url.Values{}
	q.Set("context", fmt.Sprintf("%d:%d:%d", semester, class, subject))
	q.Set("student_id", strconv.FormatInt(student, 10))
	return "/guru/rekomendasi-semester/final?" + q.Encode()
}

func newFinalSubmissionKey() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
