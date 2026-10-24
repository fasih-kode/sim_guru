package web

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fasih-kode/sim_guru/internal/academic"
	"github.com/fasih-kode/sim_guru/internal/auth"
)

func TestTransitionFormAndPreview(t *testing.T) {
	templates, err := template.New("web").Funcs(template.FuncMap{"rowNumber": func(page, index int) int { return (page-1)*10 + index + 1 }, "weekdayName": func(day int) string { return "Senin" }}).ParseFS(content, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	tv := &transitionView{Years: []academic.Year{{ID: 1, Name: "2026/2027", Active: true}, {ID: 2, Name: "2027/2028"}}, SourceYear: 1, TargetYear: 2, Outcome: map[int64]string{}, Destination: map[int64]int64{}, Preview: academic.TransitionPreview{Students: []academic.TransitionStudent{{StudentID: 1, Name: "Siswa VII", FromGrade: 7, FromClass: "A"}, {StudentID: 2, Name: "Siswa VIII", FromGrade: 8, FromClass: "A"}, {StudentID: 3, Name: "Siswa IX", FromGrade: 9, FromClass: "A"}}, Classes: []academic.Class{{ID: 7, Grade: 7, Name: "A"}, {ID: 8, Grade: 8, Name: "B"}, {ID: 9, Grade: 9, Name: "B"}}}}
	v := view{Title: "Transisi Tahun Ajaran", Page: "student-transition", User: &auth.User{ID: 1, DisplayName: "Kurikulum Uji", Roles: []string{"kurikulum"}}, Transition: tv}
	render := func() string {
		t.Helper()
		var b bytes.Buffer
		if err := templates.ExecuteTemplate(&b, "page", v); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	body := render()
	ninth := body[strings.Index(body, `<tr data-transition-row data-grade="9">`):]
	ninth = ninth[:strings.Index(ninth, "</tr>")]
	if strings.Contains(ninth, `value="promote"`) || !strings.Contains(ninth, "Tidak lulus — mengulang kelas 9") {
		t.Fatal("kelas9 memiliki pilihan hasil salah")
	}
	if strings.Count(body, `<option value="">Pilih hasil</option>`) != 3 || strings.Count(body, "data-transition-destination hidden") != 3 {
		t.Fatal("hasil harus dipilih eksplisit, tujuan awal disembunyikan")
	}
	if dir := os.Getenv("SIM_GURU_VISUAL_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "transition.html"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	tv.Outcome[1] = "promote"
	tv.Destination[1] = 8
	tv.Outcome[2] = "leave"
	tv.Outcome[3] = "repeat"
	tv.Destination[3] = 9
	tv.Stage = true
	body = render()
	if !strings.Contains(body, "Tidak lulus — mengulang kelas 9") || !strings.Contains(body, "8B") || !strings.Contains(body, "9B") || strings.Contains(body, ">repeat<") {
		t.Fatal("pratinjau tidak memakai label/kelas yang jelas")
	}
	tv.Outcome[3] = "graduate"
	tv.Destination[3] = 0
	if tv.TargetGrade(tv.Preview.Students[2]) != 0 || tv.OutcomeLabel(tv.Preview.Students[2]) != "Lulus" {
		t.Fatal("lulus tidak boleh punya tujuan")
	}
}
