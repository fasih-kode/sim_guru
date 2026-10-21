package web

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/fasih-kode/sim_guru/internal/master"
)

type importView struct {
	Kind   master.Kind
	Report master.ImportReport
	Result *master.ImportResult
}

func (a *application) importAccess(w http.ResponseWriter, r *http.Request) (master.Kind, bool) {
	kind := master.Kind(r.PathValue("kind"))
	if kind != master.Students && kind != master.Teachers {
		http.NotFound(w, r)
		return kind, false
	}
	return a.masterAccess(w, r, true)
}
func (a *application) importTemplate(w http.ResponseWriter, r *http.Request) {
	kind, ok := a.importAccess(w, r)
	if !ok {
		return
	}
	data, err := content.ReadFile("import_templates/" + string(kind) + ".xlsx")
	if err != nil {
		a.failure(w, err)
		return
	}
	name := "template-siswa.xlsx"
	if kind == master.Teachers {
		name = "template-guru.xlsx"
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Write(data)
}
func (a *application) masterImport(w http.ResponseWriter, r *http.Request) {
	kind, ok := a.importAccess(w, r)
	if !ok {
		return
	}
	v := view{Page: "master-import", Title: "Impor Excel " + kind.Label(), Import: &importView{Kind: kind}}
	if r.Method == "GET" {
		a.renderImportModal(w, r, kind, 200, "")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
	}()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		a.renderImportModal(w, r, kind, 400, "Unggahan tidak valid atau melebihi batas ukuran.")
		return
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File["file"]) != 1 {
		v.Error = "Pilih satu file Excel .xlsx."
		a.renderImportModal(w, r, kind, 422, v.Error)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		v.Error = "File belum dapat dibaca."
		a.renderImportModal(w, r, kind, 422, v.Error)
		return
	}
	defer file.Close()
	if !strings.EqualFold(filepath.Ext(header.Filename), ".xlsx") || header.Size > master.MaxImportBytes {
		v.Error = "Gunakan file .xlsx dengan ukuran maksimal 5 MiB."
		a.renderImportModal(w, r, kind, 422, v.Error)
		return
	}
	parsed, err := master.ParseImport(kind, file)
	if err != nil {
		status, message := masterError(err)
		a.renderImportModal(w, r, kind, status, message)
		return
	}
	report, err := a.options.Masters.PreviewImport(r.Context(), principal(r).ID, kind, parsed.Rows)
	if err != nil {
		status, message := masterError(err)
		a.renderImportModal(w, r, kind, status, message)
		return
	}
	v.Import.Report = report
	status := 200
	if report.ErrorCount > 0 {
		status = 422
		v.Error = fmt.Sprintf("%d baris perlu diperbaiki. Belum ada data yang disimpan. Perbaiki file lalu unggah ulang.", report.ErrorCount)
	}
	a.render(w, r, "page", status, v)
}

func (a *application) renderImportModal(w http.ResponseWriter, r *http.Request, kind master.Kind, status int, message string) {
	filter := master.Filter{Page: 1}
	records, err := a.options.Masters.List(r.Context(), principal(r).ID, kind, filter)
	if err != nil {
		a.failure(w, err)
		return
	}
	mv := &masterView{Kind: kind, Records: records, Filter: filter, CanWrite: true}
	v := view{Page: "master-list", Title: "Data " + kind.Label(), Master: mv, Import: &importView{Kind: kind}, ModalOpen: true, Error: message}
	a.render(w, r, "page", status, v)
}
func (a *application) confirmImport(w http.ResponseWriter, r *http.Request) {
	kind, ok := a.importAccess(w, r)
	if !ok {
		return
	}
	if !parseForm(w, r) {
		return
	}
	v := view{Page: "master-import", Title: "Impor Excel " + kind.Label(), Import: &importView{Kind: kind}}
	result, err := a.options.Masters.ConfirmImport(r.Context(), principal(r).ID, kind, r.PostForm.Get("import_token"))
	if err != nil {
		status, message := masterError(err)
		v.Error = message
		a.render(w, r, "page", status, v)
		return
	}
	v.Import.Result = &result
	v.Message = fmt.Sprintf("%d data %s tersimpan beserta historinya.", result.Count, kind.Label())
	if result.AlreadyComplete {
		v.Message = fmt.Sprintf("Impor ini sudah diproses sebelumnya. %d data tersimpan; tidak ada duplikasi penyimpanan.", result.Count)
	}
	a.render(w, r, "page", 200, v)
}
