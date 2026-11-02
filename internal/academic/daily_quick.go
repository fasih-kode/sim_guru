package academic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type DailyQuickRow struct {
	StudentID       int64
	Status, Arrival string
}
type DailyQuickInput struct {
	ClassID                   int64
	Date, Reason, Fingerprint string
	Rows                      []DailyQuickRow
}

func quickFingerprint(sheet DailySheet) string {
	raw, _ := json.Marshal(struct {
		Base     string
		Students []DailyStudent
	}{sheet.Fingerprint, sheet.Students})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (s *Store) SaveQuickDaily(ctx context.Context, actor int64, in DailyQuickInput) (int, error) {
	tx, e := s.begin(ctx, actor, "petugas_absensi")
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	if e = foundationReason(in.Reason); e != nil {
		return 0, e
	}
	if len(in.Rows) == 0 || len(in.Rows) > 5000 {
		return 0, ValidationError("Pilih siswa untuk dicatat, maksimal 5.000 baris.")
	}
	sheet, e := dailySheet(ctx, tx, actor, in.ClassID, in.Date)
	if e != nil {
		return 0, e
	}
	if sheet.BlockReason != "" {
		return 0, ValidationError(sheet.BlockReason)
	}
	if !sheet.SchoolDay {
		return 0, ValidationError("Tanggal ini bukan hari sekolah.")
	}
	if in.Fingerprint == "" || in.Fingerprint != sheet.QuickFingerprint {
		return 0, ErrConflict
	}
	available := map[int64]DailyStudent{}
	for _, d := range sheet.Students {
		available[d.StudentID] = d
	}
	seen := map[int64]bool{}
	count := 0
	for _, r := range in.Rows {
		if r.StudentID < 1 || seen[r.StudentID] {
			return 0, ValidationError("Pilihan siswa tidak valid.")
		}
		seen[r.StudentID] = true
		if r.Status == "" {
			continue
		}
		d, ok := available[r.StudentID]
		if !ok || d.ID != 0 {
			return 0, ErrConflict
		}
		_, e = saveDailyAttendanceTx(ctx, tx, actor, DailyInput{ClassID: in.ClassID, Date: in.Date, StudentID: r.StudentID, Status: r.Status, Arrival: r.Arrival, Reason: in.Reason, Fingerprint: sheet.Fingerprint})
		if e != nil {
			if isValidation(e) {
				return 0, ValidationError(d.Name + ": " + e.Error())
			}
			return 0, e
		}
		count++
	}
	if count == 0 {
		return 0, ValidationError("Belum ada status siswa yang dipilih.")
	}
	if e = tx.Commit(); e != nil {
		return 0, e
	}
	return count, nil
}
