package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fasih-kode/sim_guru/internal/academic"
)

type placementBatchView struct {
	Classes          []academic.Class
	Candidates       []academic.PlacementDate
	Selected         map[int64]bool
	Batch            academic.PlacementStartBatch
	Preview          *academic.PlacementStartPreview
	Suggested, Token string
}
type placementBatchToken struct {
	Actor, Expires int64
	Batch          academic.PlacementStartBatch
	Digest         string
}

func (a *application) signPlacementBatch(p placementBatchToken) string {
	raw, _ := json.Marshal(p)
	m := hmac.New(sha256.New, a.options.CSRFKey)
	m.Write([]byte("placement-start-batch-v1\x00"))
	m.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func (a *application) readPlacementBatch(token string, actor int64) (placementBatchToken, error) {
	var p placementBatchToken
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(token) > 256<<10 {
		return p, academic.ValidationError("Pratinjau tidak valid. Periksa ulang pilihan siswa.")
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return p, academic.ErrConflict
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return p, academic.ErrConflict
	}
	m := hmac.New(sha256.New, a.options.CSRFKey)
	m.Write([]byte("placement-start-batch-v1\x00"))
	m.Write(raw)
	if !hmac.Equal(sig, m.Sum(nil)) {
		return p, academic.ErrConflict
	}
	if e = json.Unmarshal(raw, &p); e != nil || p.Actor != actor || p.Expires <= time.Now().Unix() {
		return p, academic.ErrConflict
	}
	return p, nil
}
func (a *application) placementDatesBulk(w http.ResponseWriter, r *http.Request) {
	actor := principal(r).ID
	s := a.options.Academics
	year, _ := strconv.ParseInt(r.URL.Query().Get("year_id"), 10, 64)
	class, _ := strconv.ParseInt(r.URL.Query().Get("class_id"), 10, 64)
	years, e := s.Years(r.Context(), actor)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	found := false
	for _, y := range years {
		if y.ID == year {
			found = true
		}
	}
	if !found {
		a.academicFailure(w, academic.ErrNotFound)
		return
	}
	classes, _, e := s.Classes(r.Context(), actor)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	b := &placementBatchView{Selected: map[int64]bool{}, Batch: academic.PlacementStartBatch{YearID: year, ClassID: class}}
	for _, c := range classes {
		if c.SchoolYearID == year {
			b.Classes = append(b.Classes, c)
		}
	}
	if class != 0 {
		found = false
		for _, c := range b.Classes {
			if c.ID == class {
				found = true
			}
		}
		if !found {
			a.academicFailure(w, academic.ErrNotFound)
			return
		}
	}
	b.Suggested, e = s.SuggestedPlacementStart(r.Context(), actor, year)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	b.Batch.Date = b.Suggested
	f := &foundationView{Years: years, YearID: year, Batch: b}
	v := view{Page: "placement-dates-bulk", Title: "Isi Tanggal Massal", Foundation: f}
	status := 200
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
		if e = r.ParseForm(); e != nil {
			http.Error(w, "Form terlalu besar atau tidak valid.", 400)
			return
		}
		if r.PostForm.Get("action") == "confirm" {
			var p placementBatchToken
			p, e = a.readPlacementBatch(r.PostForm.Get("preview_token"), actor)
			if e == nil && (p.Batch.YearID != year || p.Batch.ClassID != class) {
				e = academic.ErrConflict
			}
			if e == nil && r.PostForm.Get("confirmed") != "yes" {
				e = academic.ValidationError("Konfirmasikan ringkasan sebelum menyimpan.")
			}
			if e == nil {
				_, e = s.ApplyPlacementStartBatch(r.Context(), actor, p.Batch, p.Digest)
			}
			if e == nil {
				http.Redirect(w, r, fmt.Sprintf("/admin/placements/dates?year_id=%d&saved=1", year), 303)
				return
			}
			b.Batch = p.Batch
		} else if r.PostForm.Get("action") == "preview" {
			b.Batch.Date = r.PostForm.Get("starts_on")
			b.Batch.Reason = r.PostForm.Get("reason")
			for _, raw := range r.PostForm["placement_ids"] {
				id, err := strconv.ParseInt(raw, 10, 64)
				if err != nil {
					e = academic.ValidationError("Pilihan siswa tidak valid.")
					break
				}
				b.Batch.IDs = append(b.Batch.IDs, id)
			}
			if e == nil {
				var p academic.PlacementStartPreview
				p, e = s.PreviewPlacementStartBatch(r.Context(), actor, b.Batch)
				if e == nil {
					b.Preview = &p
					if p.Valid {
						b.Token = a.signPlacementBatch(placementBatchToken{Actor: actor, Expires: time.Now().Add(10 * time.Minute).Unix(), Batch: b.Batch, Digest: p.Digest})
						v.ModalOpen = true
					} else {
						v.Error = "Ada siswa yang perlu diperiksa. Sesuaikan tanggal atau keluarkan dari pilihan, lalu periksa ulang."
						status = 422
					}
				}
			}
		} else {
			e = academic.ValidationError("Aksi tidak valid.")
		}
		if e != nil {
			status, v.Error = academicError(e)
			if status >= 500 {
				a.failure(w, e)
				return
			}
		}
		for _, id := range b.Batch.IDs {
			b.Selected[id] = true
		}
	}
	dates, e := s.PlacementDates(r.Context(), actor, year)
	if e != nil {
		a.academicFailure(w, e)
		return
	}
	// Scope by the immutable initial class, just like the individual date editor.
	for _, p := range dates {
		if p.StartsOn == "" && (class == 0 || p.ClassID == class) {
			b.Candidates = append(b.Candidates, p)
			if r.Method == http.MethodGet {
				b.Selected[p.ID] = true
			}
		}
	}
	a.render(w, r, "page", status, v)
}
