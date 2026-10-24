package academic

import (
	"context"
	"fmt"
	"sort"
	"time"
)

type scheduleCandidate struct {
	day, start, end int
	periods         []LessonPeriod
	softJP          int
	softTeacher     bool
}
type scheduleTask struct {
	rule       int
	length     int
	candidates []scheduleCandidate
}

func periodBlocked(p LessonPeriod, blocks []SchoolTimeBlock) bool {
	for _, b := range blocks {
		if p.Weekday == b.Weekday && clockMinute(p.StartsAt) < clockMinute(b.EndsAt) && clockMinute(p.EndsAt) > clockMinute(b.StartsAt) {
			return true
		}
	}
	return false
}
func candidateOverlap(a, b scheduleCandidate) bool {
	if a.day != b.day {
		return false
	}
	for _, p := range a.periods {
		for _, q := range b.periods {
			if clockMinute(p.StartsAt) < clockMinute(q.EndsAt) && clockMinute(p.EndsAt) > clockMinute(q.StartsAt) {
				return true
			}
		}
	}
	return false
}
func planReady(p SchedulePlan) error {
	for _, g := range p.ConstraintGrids {
		if g.NeedsReview(p) {
			return ValidationError("Jam sekolah berubah. Tinjau dan simpan ulang kisi Batasan Jadwal sebelum generate atau publikasi.")
		}
	}
	if len(p.Rules) == 0 {
		return ValidationError("Atur Struktur Kurikulum dan guru pengampu sebelum generate.")
	}
	available := 0
	for _, lp := range p.Periods {
		if !periodBlocked(lp, p.Blocks) {
			available++
		}
	}
	classes, teachers := map[int64]int{}, map[int64]int{}
	seen := map[[2]int64]bool{}
	total := 0
	for _, r := range p.Rules {
		if r.Missing {
			return ValidationError(fmt.Sprintf("Aturan JP belum diisi: %s · %s.", r.ClassName, r.SubjectName))
		}
		if err := validateCurriculumRule(r.CurriculumRule); err != nil {
			return err
		}
		key := [2]int64{r.ClassID, r.SubjectID}
		if seen[key] {
			return ValidationError("Penugasan kelas–mapel ganda; periksa tanggal pengampu.")
		}
		seen[key] = true
		if r.WeeklyJP > 0 && r.AssignmentID == 0 {
			return ValidationError(fmt.Sprintf("Pengampu aktif belum tersedia: %s · %s.", r.ClassName, r.SubjectName))
		}
		classes[r.ClassID] += r.WeeklyJP
		teachers[r.TeacherID] += r.WeeklyJP
		total += len(r.Meetings)
		if classes[r.ClassID] > available {
			return ValidationError(fmt.Sprintf("Kebutuhan %s melebihi %d JP sekolah yang tersedia.", r.ClassName, available))
		}
		if r.TeacherID > 0 && teachers[r.TeacherID] > available {
			return ValidationError(fmt.Sprintf("Beban %s melebihi %d JP sekolah yang tersedia.", r.TeacherName, available))
		}
	}
	for _, c := range p.Classes {
		if _, ok := classes[c.ID]; !ok {
			return ValidationError(fmt.Sprintf("Struktur kurikulum/pengampu rombel %s belum tersedia.", c.Name))
		}
	}
	for i, lp := range p.Periods {
		if i == 0 {
			continue
		}
		prev := p.Periods[i-1]
		if prev.Weekday != lp.Weekday {
			continue
		}
		if clockMinute(lp.StartsAt) < clockMinute(prev.EndsAt) {
			return ValidationError("Urutan nomor jam harus mengikuti waktu dan tidak bertumpang tindih.")
		}
		if clockMinute(lp.StartsAt) > clockMinute(prev.EndsAt) {
			covered := false
			for _, b := range p.Blocks {
				if b.Kind == "break" && b.Weekday == lp.Weekday && clockMinute(b.StartsAt) <= clockMinute(prev.EndsAt) && clockMinute(b.EndsAt) >= clockMinute(lp.StartsAt) {
					covered = true
				}
			}
			if !covered {
				return ValidationError(fmt.Sprintf("Jeda hari %d antara jam %d–%d belum ditandai sebagai Istirahat di Waktu Sekolah.", lp.Weekday, prev.Number, lp.Number))
			}
		}
	}
	if total == 0 {
		return ValidationError("Belum ada kebutuhan pembelajaran di atas 0 JP.")
	}
	if total > 1200 {
		return ValidationError("Cakupan melebihi 1.200 pertemuan; kurangi cakupan semester sebelum generate.")
	}
	for _, b := range p.Blocks {
		found := b.Kind == "break"
		for _, lp := range p.Periods {
			if b.Weekday == lp.Weekday && clockMinute(b.StartsAt) < clockMinute(lp.EndsAt) && clockMinute(b.EndsAt) > clockMinute(lp.StartsAt) {
				if b.Kind != "activity" || b.PeriodNo != lp.Number || clockMinute(b.StartsAt) != clockMinute(lp.StartsAt) || clockMinute(b.EndsAt) != clockMinute(lp.EndsAt) {
					return ValidationError("Jam pelajaran memotong istirahat/kegiatan. Sesuaikan Waktu Sekolah.")
				}
				found = true
			}
		}
		if !found {
			return ValidationError("Nomor/waktu kegiatan tidak cocok dengan Jam Pelajaran.")
		}
	}
	return mandatoryCapacity(p)
}

// SolveSchedule searches for a feasible complete timetable. It never changes JP,
// teachers or blocked time to obtain a result, and never writes to the database.
func SolveSchedule(ctx context.Context, p SchedulePlan, preferred []TimetableEntry) ([]TimetableEntry, error) {
	if err := planReady(p); err != nil {
		return nil, err
	}
	days := map[int][]LessonPeriod{}
	for _, lp := range p.Periods {
		days[lp.Weekday] = append(days[lp.Weekday], lp)
	}
	var tasks []scheduleTask
	for ri, r := range p.Rules {
		for _, length := range r.Meetings {
			select {
			case <-ctx.Done():
				return nil, ValidationError("Batas pencarian tercapai sebelum selesai; belum terbukti mustahil.")
			default:
			}
			task := scheduleTask{rule: ri, length: length}
			for day := 1; day <= 7; day++ {
				periods := days[day]
				sort.Slice(periods, func(i, j int) bool { return periods[i].Number < periods[j].Number })
				for i := 0; i+length <= len(periods); i++ {
					span := periods[i : i+length]
					valid := true
					for j, lp := range span {
						if lp.Number != span[0].Number+j || periodBlocked(lp, p.Blocks) || hardUnavailable(p, r.TeacherID, r.SubjectID, lp) {
							valid = false
							break
						}
					}
					if valid {
						candidate := scheduleCandidate{day: day, start: span[0].Number, end: span[len(span)-1].Number, periods: span}
						for _, lp := range span {
							hit := false
							for _, constraint := range p.Constraints {
								if constraint.Severity == "soft" && constraint.matches(r.TeacherID, r.SubjectID, lp) {
									hit = true
									if constraint.TeacherID > 0 {
										candidate.softTeacher = true
									}
								}
							}
							if hit {
								candidate.softJP++
							}
						}
						task.candidates = append(task.candidates, candidate)
					}
				}
			}
			// Existing published placements are only a search preference, never copied
			// without validating them against the new effective curriculum and blocks.
			rank := func(c scheduleCandidate) int {
				for _, e := range preferred {
					if e.ClassID == r.ClassID && e.SubjectID == r.SubjectID && e.Weekday == c.day && e.PeriodStart == c.start && e.PeriodEnd == c.end {
						return 0
					}
				}
				return 1
			}
			sort.SliceStable(task.candidates, func(i, j int) bool { return rank(task.candidates[i]) < rank(task.candidates[j]) })
			if len(task.candidates) == 0 {
				return nil, ValidationError(fmt.Sprintf("Tidak ada rentang %d JP untuk %s · %s pada jam sekolah tersedia.", length, r.ClassName, r.SubjectName))
			}
			tasks = append(tasks, task)
		}
	}
	placed := make([]int, len(tasks))
	for i := range placed {
		placed[i] = -1
	}
	// Scores are monotonic: distinct teacher/day soft violations first, then
	// teaching JP touching any soft constraint. Published positions break search ties.
	var solution []int
	bestScore := preferenceScore{days: int(^uint(0) >> 1), jp: int(^uint(0) >> 1)}
	score := func() preferenceScore { return placementPreferenceScore(p, tasks, placed) }
	// Keep an already valid complete published timetable as an incumbent. Soft
	// preferences must not turn a known feasible schedule into a timeout failure.
	if len(preferred) == len(tasks) && ValidatePlannedEntries(p, preferred, true) == nil {
		seed := make([]int, len(tasks))
		used := make([]bool, len(preferred))
		matched := true
		for i, t := range tasks {
			seed[i] = -1
			r := p.Rules[t.rule]
			for ei, e := range preferred {
				if used[ei] || e.ClassID != r.ClassID || e.SubjectID != r.SubjectID || e.PeriodEnd-e.PeriodStart+1 != t.length {
					continue
				}
				for ci, c := range t.candidates {
					if e.Weekday == c.day && e.PeriodStart == c.start && e.PeriodEnd == c.end {
						seed[i] = ci
						used[ei] = true
						break
					}
				}
				if seed[i] >= 0 {
					break
				}
			}
			if seed[i] < 0 {
				matched = false
				break
			}
		}
		if matched {
			solution = seed
			bestScore = placementPreferenceScore(p, tasks, seed)
		}
	}

	nodes := 0
	timedOut := false
	valid := func(ti, ci int) bool {
		select {
		case <-ctx.Done():
			timedOut = true
			return false
		default:
		}
		task := tasks[ti]
		r := p.Rules[task.rule]
		c := task.candidates[ci]
		for j, index := range placed {
			if index < 0 {
				continue
			}
			other := tasks[j]
			q := p.Rules[other.rule]
			d := other.candidates[index]
			if other.rule == task.rule && r.DifferentDays && c.day == d.day {
				return false
			}
			if (r.ClassID == q.ClassID || r.TeacherID == q.TeacherID) && candidateOverlap(c, d) {
				return false
			}
		}
		return true
	}
	var search func(int) bool
	search = func(depth int) bool {
		nodes++
		select {
		case <-ctx.Done():
			timedOut = true
			return false
		default:
		}
		if nodes > 1500000 {
			timedOut = true
			return false
		}
		currentScore := score()
		if !currentScore.less(bestScore) {
			return false
		}
		if depth == len(tasks) {
			solution = append([]int(nil), placed...)
			bestScore = currentScore
			return bestScore.days == 0 && bestScore.jp == 0
		}
		best := -1
		var choices []int
		for i, t := range tasks {
			if placed[i] >= 0 {
				continue
			}
			skip := false
			for j := 0; j < i; j++ {
				if tasks[j].rule == t.rule && tasks[j].length == t.length && placed[j] < 0 {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			var possible []int
			for ci := range t.candidates {
				if valid(i, ci) {
					possible = append(possible, ci)
				}
			}
			if len(possible) == 0 {
				return false
			}
			if best < 0 || len(possible) < len(choices) || (len(possible) == len(choices) && t.length > tasks[best].length) {
				best = i
				choices = possible
			}
		}
		if best < 0 {
			return false
		}
		sort.SliceStable(choices, func(i, j int) bool {
			placed[best] = choices[i]
			a := score()
			placed[best] = choices[j]
			b := score()
			placed[best] = -1
			return a.less(b)
		})
		for _, ci := range choices {
			placed[best] = ci
			if search(depth + 1) {
				return true
			}
			placed[best] = -1
			if timedOut {
				return false
			}
		}
		return false
	}
	select {
	case <-ctx.Done():
		return nil, ValidationError("Pencarian dihentikan sebelum selesai; belum membuktikan jadwal mustahil.")
	default:
	}
	search(0)
	if solution == nil {
		if timedOut {
			return nil, ValidationError("Batas pencarian tercapai; belum ditemukan jadwal lengkap dan belum terbukti mustahil. Tinjau pengaturan lalu coba lagi. Draf sebelumnya tetap utuh.")
		}
		return nil, ValidationError("Tidak ada susunan yang memenuhi seluruh JP, pembagian pertemuan, hari, serta bentrok guru/kelas pada pengaturan ini.")
	}
	out := make([]TimetableEntry, 0, len(tasks))
	for i, index := range solution {
		r := p.Rules[tasks[i].rule]
		c := tasks[i].candidates[index]
		out = append(out, TimetableEntry{ClassID: r.ClassID, SubjectID: r.SubjectID, TeacherID: r.TeacherID, AssignmentID: r.AssignmentID, ClassName: r.ClassName, SubjectName: r.SubjectName, TeacherName: r.TeacherName, Weekday: c.day, PeriodStart: c.start, PeriodEnd: c.end})
	}
	return out, ValidatePlannedEntries(p, out, true)
}

func ValidatePlannedEntries(p SchedulePlan, entries []TimetableEntry, complete bool) error {
	if err := planReady(p); err != nil {
		return err
	}
	rules := map[[2]int64]EffectiveCurriculum{}
	for _, r := range p.Rules {
		rules[[2]int64{r.ClassID, r.SubjectID}] = r
	}
	counts := map[[2]int64]map[int]int{}
	days := map[[2]int64]map[int]bool{}
	var used []struct {
		e TimetableEntry
		c scheduleCandidate
	}
	for _, e := range entries {
		key := [2]int64{e.ClassID, e.SubjectID}
		r, ok := rules[key]
		if !ok || e.AssignmentID != r.AssignmentID || e.TeacherID != r.TeacherID {
			return ValidationError("Entri tidak sesuai struktur kurikulum/pengampu efektif.")
		}
		c := scheduleCandidate{day: e.Weekday, start: e.PeriodStart, end: e.PeriodEnd}
		for _, lp := range p.Periods {
			if lp.Weekday == e.Weekday && lp.Number >= e.PeriodStart && lp.Number <= e.PeriodEnd {
				if periodBlocked(lp, p.Blocks) {
					return ValidationError("Pembelajaran tidak boleh mengisi istirahat atau kegiatan sekolah.")
				}
				if hardUnavailable(p, e.TeacherID, e.SubjectID, lp) {
					return ValidationError(fmt.Sprintf("%s · %s melanggar batasan Tidak tersedia pada hari %d jam %d (%s–%s).", r.TeacherName, r.SubjectName, lp.Weekday, lp.Number, lp.StartsAt, lp.EndsAt))
				}
				c.periods = append(c.periods, lp)
			}
		}
		n := e.PeriodEnd - e.PeriodStart + 1
		if n < 1 || n != len(c.periods) {
			return ValidationError("Rentang jam pembelajaran tidak lengkap.")
		}
		for _, old := range used {
			if (old.e.ClassID == e.ClassID || old.e.TeacherID == e.TeacherID) && candidateOverlap(old.c, c) {
				return ValidationError("Jadwal bentrok dengan kelas atau guru.")
			}
		}
		allowed := 0
		for _, v := range r.Meetings {
			if v == n {
				allowed++
			}
		}
		if counts[key] == nil {
			counts[key] = map[int]int{}
			days[key] = map[int]bool{}
		}
		counts[key][n]++
		if counts[key][n] > allowed {
			return ValidationError(fmt.Sprintf("Pembagian pertemuan %s · %s harus %s JP.", r.ClassName, r.SubjectName, r.Pattern()))
		}
		if r.DifferentDays && days[key][e.Weekday] {
			return ValidationError("Pertemuan mapel yang sama harus ditempatkan pada hari berbeda sesuai pengaturan.")
		}
		days[key][e.Weekday] = true
		used = append(used, struct {
			e TimetableEntry
			c scheduleCandidate
		}{e, c})
	}
	if complete {
		for key, r := range rules {
			expected := map[int]int{}
			for _, n := range r.Meetings {
				expected[n]++
			}
			for n, total := range expected {
				if counts[key][n] != total {
					return ValidationError(fmt.Sprintf("JP/pertemuan %s · %s belum sesuai target %d JP (%s).", r.ClassName, r.SubjectName, r.WeeklyJP, r.Pattern()))
				}
			}
		}
	}
	return nil
}

// A bounded solver fits inside the HTTP server's 15 second write timeout.
func solveWithBudget(ctx context.Context, p SchedulePlan, preferred []TimetableEntry) ([]TimetableEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return SolveSchedule(ctx, p, preferred)
}
