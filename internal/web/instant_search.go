package web

// liveListPage membatasi pembaruan HTMX pada komponen daftar, bukan layout,
// autentikasi, capture kamera, atau respons mutasi/form detail.
func liveListPage(page string) bool {
	switch page {
	case "users", "audit", "master-list", "master-import", "academic-years", "academic-semesters", "academic-classes", "teaching-assignments", "lesson-periods", "weekly-timetable", "my-schedule", "my-homerooms", "my-homeroom-roster", "academic-history", "placement-history", "homeroom-history", "student-placements", "student-transition", "placement-dates", "placement-dates-bulk", "school-calendar", "curriculum-structure", "school-time-blocks", "schedule-constraints", "timetable-xml-import", "arrival-rules", "daily-attendance", "daily-quick", "student-cards", "lesson-attendance", "teacher-attendance-management", "teacher-reports", "teacher-report-detail", "teacher-recap", "student-guidance", "student-guidance-archive", "student-cases", "teaching-journals", "assessment-owned-scores":
		return true
	}
	return false
}
