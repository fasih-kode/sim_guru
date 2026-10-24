package academic

import (
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

const MaxASCBytes int64 = 5 << 20
const MaxASCCards = 10000

type ASCItem struct{ ID, Name, Short string }
type ASCPeriod struct {
	Number           int
	StartsAt, EndsAt string
}
type ASCMeeting struct {
	LessonID, ClassID, SubjectID, TeacherID, DayID string
	Start, End                                     int
}
type ASCDocument struct {
	Digest                            string
	Version                           string
	Classes, Teachers, Subjects, Days []ASCItem
	Periods                           []ASCPeriod
	Meetings                          []ASCMeeting
	CardCount, ExpectedJP             int
	Issues                            []string
}
type ascLesson struct {
	id, class, subject, teacher, group, days, weeks, terms string
	length, weekly                                         int
}
type ascCard struct {
	lesson, days, weeks, terms string
	period                     int
}

func ascInteger(text string, min, max int) (int, error) {
	n, e := strconv.Atoi(text)
	if e != nil || n < min || n > max {
		return 0, ValidationError("Angka pada XML tidak valid atau di luar batas yang didukung.")
	}
	return n, nil
}
func ascClock(text string) (string, error) {
	t, e := time.Parse("15:04", text)
	if e != nil {
		return "", ValidationError("Waktu pada XML tidak valid.")
	}
	return t.Format("15:04"), nil
}
func ascIDList(text string) []string {
	var ids []string
	for _, s := range strings.Split(text, ",") {
		if s = strings.TrimSpace(s); s != "" {
			ids = append(ids, s)
		}
	}
	return ids
}
func oneASCID(text string) string {
	ids := ascIDList(text)
	if len(ids) != 1 {
		return ""
	}
	return ids[0]
}
func ascDays(mask string) ([]string, error) {
	if len(mask) < 1 || len(mask) > 7 {
		return nil, ValidationError("Pola hari XML harus berisi 1–7 posisi.")
	}
	var out []string
	for i, c := range mask {
		if c != '0' && c != '1' {
			return nil, ValidationError("Pola hari XML tidak valid.")
		}
		if c == '1' {
			b := bytes.Repeat([]byte{'0'}, len(mask))
			b[i] = '1'
			out = append(out, string(b))
		}
	}
	if len(out) == 0 {
		return nil, ValidationError("Kartu XML belum mempunyai hari penempatan.")
	}
	return out, nil
}

// ParseASCXML reads only scheduling fields. Contacts, students and instructions
// in uploaded documents are never retained or executed. No DTD/network loading.
func ParseASCXML(reader io.Reader) (ASCDocument, error) {
	var doc ASCDocument
	raw, err := io.ReadAll(io.LimitReader(reader, MaxASCBytes+1))
	if err != nil {
		return doc, ValidationError("Berkas XML belum dapat dibaca.")
	}
	if len(raw) == 0 || int64(len(raw)) > MaxASCBytes {
		return doc, ValidationError("Gunakan XML maksimal 5 MiB.")
	}
	doc.Digest = fmt.Sprintf("%x", sha256.Sum256(raw))
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.CharsetReader = charset.NewReaderLabel
	depth, tokens, roots := 0, 0, 0
	section := ""
	sections := map[string]bool{}
	ids := map[string]map[string]bool{}
	var lessons []ascLesson
	var cards []ascCard
	dayNames := map[string]string{}
	groups := map[string][2]string{}
	defs := map[string]map[string]string{"daysdefs": {}, "weeksdefs": {}, "termsdefs": {}}
	for {
		token, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return doc, ValidationError("XML tidak valid atau encoding tidak didukung.")
		}
		tokens++
		if tokens > 200000 {
			return doc, ValidationError("XML terlalu kompleks.")
		}
		switch t := token.(type) {
		case xml.Directive:
			return doc, ValidationError("XML dengan DTD atau deklarasi entity tidak didukung.")
		case xml.ProcInst:
			if t.Target != "xml" {
				return doc, ValidationError("Instruksi pemrosesan XML tidak didukung.")
			}
		case xml.StartElement:
			depth++
			if depth > 3 || t.Name.Space != "" {
				return doc, ValidationError("Struktur XML ini belum didukung; gunakan ekspor aSc Timetables 2012 XML.")
			}
			a := map[string]string{}
			for _, v := range t.Attr {
				if len(v.Value) > 2048 {
					return doc, ValidationError("Nilai atribut XML terlalu panjang.")
				}
				if _, duplicate := a[v.Name.Local]; duplicate {
					return doc, ValidationError("Atribut XML duplikat.")
				}
				a[v.Name.Local] = v.Value
			}
			if depth == 1 {
				roots++
				if roots != 1 || t.Name.Local != "timetable" {
					return doc, ValidationError("Berkas harus berupa ekspor aSc Timetables XML.")
				}
				doc.Version = a["ascttversion"]
				continue
			}
			if depth == 2 {
				section = t.Name.Local
				if sections[section] {
					return doc, ValidationError("Bagian XML duplikat.")
				}
				sections[section] = true
				continue
			}
			if len(a["id"]) > 128 || len(a["name"]) > 200 {
				return doc, ValidationError("ID/nama XML terlalu panjang.")
			}
			if a["id"] != "" {
				if ids[section] == nil {
					ids[section] = map[string]bool{}
				}
				if ids[section][a["id"]] {
					return doc, ValidationError("ID XML duplikat dalam satu bagian.")
				}
				ids[section][a["id"]] = true
				if len(ids[section]) > 2000 {
					return doc, ValidationError("Terlalu banyak referensi XML.")
				}
			}
			switch section + "/" + t.Name.Local {
			case "classes/class", "teachers/teacher", "subjects/subject":
				if a["id"] == "" || strings.TrimSpace(a["name"]) == "" {
					return doc, ValidationError("Referensi kelas/guru/mapel XML tidak lengkap.")
				}
				item := ASCItem{ID: a["id"], Name: a["name"], Short: a["short"]}
				switch section {
				case "classes":
					doc.Classes = append(doc.Classes, item)
				case "teachers":
					doc.Teachers = append(doc.Teachers, item)
				case "subjects":
					doc.Subjects = append(doc.Subjects, item)
				}
			case "periods/period":
				n, e := ascInteger(a["period"], 1, 30)
				if e != nil {
					return doc, e
				}
				start, e := ascClock(a["starttime"])
				if e != nil {
					return doc, e
				}
				end, e := ascClock(a["endtime"])
				if e != nil || start >= end {
					return doc, ValidationError("Rentang jam XML tidak valid.")
				}
				for _, p := range doc.Periods {
					if p.Number == n {
						return doc, ValidationError("Nomor jam XML duplikat.")
					}
				}
				doc.Periods = append(doc.Periods, ASCPeriod{n, start, end})
			case "daysdefs/daysdef":
				defs[section][a["id"]] = a["days"]
				if ds, e := ascDays(a["days"]); e == nil && len(ds) == 1 {
					dayNames[ds[0]] = a["name"]
				}
			case "weeksdefs/weeksdef":
				defs[section][a["id"]] = a["weeks"]
			case "termsdefs/termsdef":
				defs[section][a["id"]] = a["terms"]
			case "groups/group":
				groups[a["id"]] = [2]string{a["classid"], a["entireclass"]}
			case "lessons/lesson":
				if len(lessons) >= 2000 {
					return doc, ValidationError("Maksimal 2.000 definisi pelajaran XML.")
				}
				length, e := ascInteger(a["periodspercard"], 1, 30)
				if e != nil {
					return doc, e
				}
				weekly, e := strconv.ParseFloat(a["periodsperweek"], 64)
				if e != nil || !(weekly >= 1 && weekly <= 210) || weekly != float64(int(weekly)) {
					return doc, ValidationError("JP mingguan pecahan atau tidak valid belum didukung.")
				}
				lessons = append(lessons, ascLesson{a["id"], oneASCID(a["classids"]), a["subjectid"], oneASCID(a["teacherids"]), a["groupids"], a["daysdefid"], a["weeksdefid"], a["termsdefid"], length, int(weekly)})
			case "cards/card":
				if len(cards) >= MaxASCCards {
					return doc, ValidationError("Maksimal 10.000 kartu/slot XML.")
				}
				n, e := ascInteger(a["period"], 1, 30)
				if e != nil {
					return doc, e
				}
				cards = append(cards, ascCard{a["lessonid"], a["days"], a["weeks"], a["terms"], n})
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return doc, ValidationError("XML harus berisi data atribut aSc tanpa teks tambahan.")
			}
		}
	}
	if roots != 1 || depth != 0 || len(lessons) == 0 || len(cards) == 0 || len(doc.Periods) == 0 {
		return doc, ValidationError("XML belum memuat definisi pelajaran, jam, dan penempatan jadwal.")
	}
	doc.CardCount = len(cards)
	daySet := map[string]bool{}
	lessonIDs := map[string]bool{}
	periodSet := map[int]bool{}
	for _, p := range doc.Periods {
		periodSet[p.Number] = true
	}
	for _, l := range lessons {
		lessonIDs[l.id] = true
		doc.ExpectedJP += l.weekly
		issue := func(message string) {
			doc.Issues = append(doc.Issues, fmt.Sprintf("Pelajaran XML %s: %s", l.id, message))
		}
		if l.class == "" || l.teacher == "" {
			issue("pengampu bersama/kelas gabungan belum didukung.")
		}
		if !ids["classes"][l.class] || !ids["teachers"][l.teacher] || !ids["subjects"][l.subject] {
			issue("referensi kelas, guru, atau mapel tidak ditemukan.")
		}
		if l.id == "" {
			issue("ID pelajaran kosong.")
		}
		for _, g := range ascIDList(l.group) {
			ref, ok := groups[g]
			if !ok || ref[0] != l.class || ref[1] != "1" {
				issue("pembelajaran kelompok/sebagian siswa belum didukung.")
			}
		}
		if l.group == "" {
			issue("cakupan seluruh rombel belum dinyatakan pada groups XML.")
		}
		if defs["weeksdefs"][l.weeks] != "1" || defs["termsdefs"][l.terms] != "1" {
			issue("jadwal bergilir beberapa minggu/term belum didukung.")
		}
		if _, ok := defs["daysdefs"][l.days]; !ok {
			issue("definisi hari tidak ditemukan.")
		}
		byDay := map[string][]int{}
		count := 0
		for _, c := range cards {
			if c.lesson != l.id {
				continue
			}
			if c.weeks != "1" || c.terms != "1" {
				issue("kartu memakai pola minggu/term yang belum didukung.")
			}
			ds, e := ascDays(c.days)
			if e != nil {
				issue(e.Error())
				continue
			}
			for _, d := range ds {
				daySet[d] = true
				byDay[d] = append(byDay[d], c.period)
				count++
				if !periodSet[c.period] {
					issue("nomor jam penempatan tidak ditemukan.")
				}
			}
		}
		if count != l.weekly {
			issue(fmt.Sprintf("membutuhkan %d JP, tetapi penempatan berisi %d JP; ekspor terpotong/tidak lengkap atau format kartu berbeda.", l.weekly, count))
		}
		var days []string
		for d := range byDay {
			days = append(days, d)
		}
		sort.Strings(days)
		for _, day := range days {
			numbers := byDay[day]
			sort.Ints(numbers)
			for i := 0; i < len(numbers); {
				end := i + 1
				for end < len(numbers) && numbers[end] == numbers[end-1]+1 {
					end++
				}
				for j := i; j < end; j += l.length {
					last := j + l.length
					if last > end {
						last = end
						issue("rentang pertemuan terpotong atau tidak sesuai durasi blok.")
					}
					doc.Meetings = append(doc.Meetings, ASCMeeting{l.id, l.class, l.subject, l.teacher, day, numbers[j], numbers[last-1]})
				}
				if end < len(numbers) && numbers[end] == numbers[end-1] {
					issue("kartu/slot penempatan duplikat.")
				}
				i = end
			}
		}
	}
	for _, c := range cards {
		if !lessonIDs[c.lesson] {
			doc.Issues = append(doc.Issues, "Kartu mengacu pada ID pelajaran yang tidak ditemukan.")
		}
	}
	var dayCodes []string
	for d := range daySet {
		dayCodes = append(dayCodes, d)
	}
	sort.Slice(dayCodes, func(i, j int) bool { return strings.Index(dayCodes[i], "1") < strings.Index(dayCodes[j], "1") })
	for _, d := range dayCodes {
		doc.Days = append(doc.Days, ASCItem{ID: d, Name: dayNames[d]})
		if doc.Days[len(doc.Days)-1].Name == "" {
			doc.Days[len(doc.Days)-1].Name = "Hari XML (" + d + ")"
		}
	}
	if len(doc.Days) > 7 || len(doc.Meetings) > 1200 {
		return doc, ValidationError("Maksimal 7 hari dan 1.200 pertemuan per impor.")
	}
	if len(doc.Issues) > 100 {
		doc.Issues = append(doc.Issues[:100], "Masalah tambahan dipangkas; perbaiki sumber XML.")
	}
	// Only catalog objects used by lesson definitions need a SIM-GURU match.
	usedClasses, usedTeachers, usedSubjects := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, l := range lessons {
		usedClasses[l.class] = true
		usedTeachers[l.teacher] = true
		usedSubjects[l.subject] = true
	}
	filter := func(items []ASCItem, used map[string]bool) []ASCItem {
		var out []ASCItem
		for _, item := range items {
			if used[item.ID] {
				out = append(out, item)
			}
		}
		return out
	}
	doc.Classes, doc.Teachers, doc.Subjects = filter(doc.Classes, usedClasses), filter(doc.Teachers, usedTeachers), filter(doc.Subjects, usedSubjects)
	return doc, nil
}
