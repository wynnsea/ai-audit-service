package chunker

import (
	"regexp"
	"strings"
)

var (
	babHeading    = regexp.MustCompile(`^BAB [IVXLC]+$`)
	bagianHeading = regexp.MustCompile(`^(Bagian|Paragraf) \S+$`)
	penjelasan    = regexp.MustCompile(`^PENJELASAN$`)
	// bagian penutup UU/PP selalu diawali kalimat ini
	penutup = regexp.MustCompile(`^Agar setiap orang mengetahuinya`)
)

// judul BAB/Bagian bisa terpotong jadi beberapa baris
const maxTitleLines = 3

// regulationUnits mengelompokkan baris jadi: Pembukaan, tiap Pasal, Penutup,
// dan bagian Penjelasan (Umum + tiap Pasal)
func regulationUnits(lines []line) []unit {
	var (
		units      []unit
		cur        = unit{section: "Pembukaan"}
		bab        string
		bagian     string
		inPenjelas bool
	)

	flush := func() {
		if len(cur.lines) > 0 {
			units = append(units, cur)
		}
	}
	start := func(section string) {
		flush()
		cur = unit{section: section}
	}

	for i := 0; i < len(lines); i++ {
		l := lines[i]
		switch {
		case penjelasan.MatchString(l.text):
			inPenjelas, bab, bagian = true, "", ""
			start("Penjelasan Umum")

		case !inPenjelas && penutup.MatchString(l.text):
			start("Penutup")

		case !inPenjelas && babHeading.MatchString(l.text):
			var title string
			title, i = readTitle(lines, i, true)
			bab, bagian = l.text+" "+title, ""
			continue

		case !inPenjelas && bagianHeading.MatchString(l.text):
			var title string
			title, i = readTitle(lines, i, false)
			bagian = l.text + " " + title
			continue

		case pasalHeading.MatchString(l.text):
			if inPenjelas {
				start("Penjelasan > " + l.text)
			} else {
				start(joinSection(bab, bagian, l.text))
			}
		}
		cur.lines = append(cur.lines, l)
	}
	flush()
	return units
}

// readTitle mengambil baris judul setelah heading di posisi i, berhenti di heading berikutnya.
// Judul BAB selalu huruf kapital, judul Bagian/Paragraf tidak.
// Mengembalikan judul dan index baris terakhir yang dipakai.
func readTitle(lines []line, i int, upper bool) (string, int) {
	var parts []string
	for j := i + 1; j < len(lines) && len(parts) < maxTitleLines; j++ {
		s := lines[j].text
		if pasalHeading.MatchString(s) || babHeading.MatchString(s) || bagianHeading.MatchString(s) {
			break
		}
		if isDataLine(s) || (upper && s != strings.ToUpper(s)) {
			break
		}
		parts = append(parts, s)
		i = j
	}
	return strings.Join(parts, " "), i
}

func joinSection(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " > ")
}
