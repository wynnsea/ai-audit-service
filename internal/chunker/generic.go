package chunker

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	maxHeadingRunes = 100
	maxHeadingWords = 12
	// heading satu tingkat ("1. Pendahuluan") lebih mudah tertukar dengan butir daftar, jadi lebih ketat
	maxShortHeadingWords = 6
	// supaya singkatan seperti "UI UX" tidak dianggap judul bagian
	minCapsLetters = 8
	genericStart   = "Awal dokumen"
)

var (
	// baris daftar isi/gambar/tabel, contoh "1.4 Manfaat ........ 13"
	tocLine = regexp.MustCompile(`(?:\.\s*){4,}[0-9IVXLCivxlc]*\s*$|…\s*[0-9]*\s*$`)
	// bertingkat: "3.2.1 Optimalisasi", "5.1 Leadership", atau klausul "A.8.2 Kontrol"
	nestedHeading = regexp.MustCompile(`^((?:[A-Z]\.\d+|\d+\.\d+)(?:\.\d+)*)\.?\s+\p{Lu}`)
	// satu tingkat: "1. Pendahuluan", "1 Pendahuluan", "A. Latar Belakang"
	shortHeading = regexp.MustCompile(`^(\d+\.?|[A-Z]\.)\s+(\S.*)$`)
	// judul bagian satu kata yang boleh berdiri sendiri; kata kapital lain bisa saja singkatan
	singleWordTitle = map[string]bool{
		"ABSTRAK": true, "ABSTRACT": true, "PENDAHULUAN": true,
		"KESIMPULAN": true, "PENUTUP": true, "LAMPIRAN": true,
	}
	// judul penutup yang tetap dikenali setelah dokumen memakai BAB
	closingTitle = map[string]bool{
		"DAFTAR PUSTAKA": true, "DAFTAR REFERENSI": true, "REFERENCES": true, "LAMPIRAN": true,
	}
	titleConnectors = map[string]bool{
		"dan": true, "atau": true, "di": true, "ke": true, "dari": true, "pada": true,
		"untuk": true, "dengan": true, "yang": true, "of": true, "and": true, "the": true,
		"for": true, "in": true, "on": true, "to": true,
	}
)

type heading struct {
	depth int
	// short: heading satu tingkat ("1. Judul"), selalu jadi anak heading bertingkat di atasnya
	short bool
	title string
}

// genericUnits mengelompokkan dokumen non-regulasi (laporan, SOP, standar) per judul bagian.
// Contoh hasil section: "BAB II TINJAUAN PUSTAKA > 2.1 Landasan Teori > 2.1.1 Definisi"
func genericUnits(lines []line) []unit {
	var (
		units []unit
		cur   = unit{section: genericStart, plain: true}
		top   string
		stack []heading
		// setelah ada BAB, teks kapital lain lebih mungkin isi (kutipan, tulisan di gambar) daripada judul
		hasBab bool
	)

	start := func() {
		if len(cur.lines) > 0 {
			units = append(units, cur)
		}
		parts := []string{top}
		for _, h := range stack {
			parts = append(parts, h.title)
		}
		cur = unit{section: joinSection(parts...), plain: true}
	}

	for i := 0; i < len(lines); i++ {
		l := lines[i]
		// daftar isi hanya mengulang judul, dan kalau ikut masuk malah menyaingi isi aslinya saat pencarian
		if tocLine.MatchString(l.text) {
			continue
		}

		// judulnya dipindah ke section, jadi tidak ikut lines; bagian tanpa isi otomatis tidak jadi chunk
		isBab := babHeading.MatchString(l.text)
		if isBab || closingTitle[l.text] || (!hasBab && isCapsHeading(l.text)) {
			hasBab = hasBab || isBab
			var title string
			title, i = readTitle(lines, i, true)
			top, stack = joinSection(l.text, title), nil
			top = strings.ReplaceAll(top, " > ", " ")
			start()
			continue
		}
		if h, ok := parseHeading(l.text); ok {
			for len(stack) > 0 {
				last := stack[len(stack)-1]
				// heading bertingkat menutup semua "1. Judul"; "1. Judul" hanya menutup sesamanya
				if !last.short && (h.short || last.depth < h.depth) {
					break
				}
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, h)
			start()
			continue
		}
		cur.lines = append(cur.lines, l)
	}
	if len(cur.lines) > 0 {
		units = append(units, cur)
	}
	return units
}

// parseHeading mengenali heading bernomor; kalimat biasa diakhiri tanda baca, jadi tidak lolos
func parseHeading(text string) (heading, bool) {
	if len([]rune(text)) > maxHeadingRunes || strings.ContainsAny(text[len(text)-1:], ".,;:?!") {
		return heading{}, false
	}
	if len(strings.Fields(text)) > maxHeadingWords {
		return heading{}, false
	}
	if m := nestedHeading.FindStringSubmatch(text); m != nil {
		return heading{depth: strings.Count(m[1], ".") + 1, title: text}, true
	}
	if m := shortHeading.FindStringSubmatch(text); m != nil {
		words := strings.Fields(m[2])
		if len(words) <= maxShortHeadingWords && isTitleCase(words) {
			return heading{short: true, title: text}, true
		}
	}
	return heading{}, false
}

// isTitleCase membedakan "1. Metode Penelitian" (judul) dari "1. Menyiapkan peralatan" (butir daftar)
func isTitleCase(words []string) bool {
	for _, w := range words {
		first := []rune(w)[0]
		if !unicode.IsUpper(first) && !unicode.IsDigit(first) && !titleConnectors[w] {
			return false
		}
	}
	return true
}

// isCapsHeading mengenali judul bagian tanpa nomor, contoh "KATA PENGANTAR"
func isCapsHeading(text string) bool {
	if len([]rune(text)) > maxHeadingRunes || text != strings.ToUpper(text) || strings.ContainsAny(text, "|\"“”=") {
		return false
	}
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < minCapsLetters || strings.ContainsAny(text[len(text)-1:], ".,;:?!") {
		return false
	}
	words := strings.Fields(text)
	if len(words) == 1 {
		return singleWordTitle[text]
	}
	return len(words) <= maxHeadingWords
}
