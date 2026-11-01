package chunker

import (
	"ai-audit-service/internal/entity"
	"regexp"
	"strings"
)

// ±500 token untuk teks bahasa Indonesia
const maxChunkChars = 2000

// unit adalah satu bagian utuh dokumen (misal satu Pasal) sebelum dipecah sesuai ukuran
type unit struct {
	section string
	lines   []line
}

var pasalHeading = regexp.MustCompile(`^Pasal \d+[A-Z]?$`)

// Split memecah halaman dokumen menjadi chunk siap di-embed
func Split(pages []entity.Page) []entity.Chunk {
	lines := cleanPages(pages)

	var units []unit
	if looksLikeRegulation(lines) {
		units = regulationUnits(lines)
	} else {
		units = []unit{{lines: lines}}
	}

	var chunks []entity.Chunk
	for _, u := range units {
		for _, c := range splitUnit(u) {
			c.Index = len(chunks)
			chunks = append(chunks, c)
		}
	}
	return chunks
}

// peraturan Indonesia (UU, PP, POJK, Permen, ...) selalu tersusun dari Pasal
func looksLikeRegulation(lines []line) bool {
	n := 0
	for _, l := range lines {
		if pasalHeading.MatchString(l.text) {
			n++
		}
	}
	return n >= 3
}

// item adalah baris yang aman dijadikan awal potongan: ayat "(1)", angka "1.", huruf "a.",
// atau label penjelasan "Ayat (1)" / "Huruf a"
var itemStart = regexp.MustCompile(`^(\(\d+\)|\d+\.|[a-z]\.|Ayat \(\d+\)|Huruf [a-z]|Angka \d+)\s?`)
var ayatStart = regexp.MustCompile(`^(?:Ayat )?\((\d+)\)`)

// splitUnit memecah unit yang terlalu panjang di batas ayat/butir
func splitUnit(u unit) []entity.Chunk {
	var groups [][]line
	var cur []line
	size := 0
	for _, l := range u.lines {
		n := len(l.text) + 1
		// potong di awal ayat/butir kalau bisa; kalau potongan sudah lewat setengah
		// batas dan tidak ketemu awal ayat, potong di baris mana saja
		if size+n > maxChunkChars && len(cur) > 0 && (itemStart.MatchString(l.text) || size >= maxChunkChars/2) {
			groups = append(groups, cur)
			cur, size = nil, 0
		}
		cur = append(cur, l)
		size += n
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}

	chunks := make([]entity.Chunk, 0, len(groups))
	for i, g := range groups {
		content := joinLines(g)
		if isEmptyExplanation(content) {
			continue
		}
		section := u.section
		if len(groups) > 1 {
			section = strings.TrimSpace(section + " " + partLabel(g, i))
		}
		chunks = append(chunks, entity.Chunk{Section: section, Page: g[0].page, Content: content})
	}
	return chunks
}

// partLabel memberi label potongan, contoh "ayat (2)-(4)"
func partLabel(g []line, i int) string {
	var first, last string
	for _, l := range g {
		if m := ayatStart.FindStringSubmatch(l.text); m != nil {
			if first == "" {
				first = m[1]
			}
			last = m[1]
		}
	}
	switch {
	case first == "":
		if i == 0 {
			return ""
		}
		return "(lanjutan)"
	case first == last:
		return "ayat (" + first + ")"
	default:
		return "ayat (" + first + ")-(" + last + ")"
	}
}

func joinLines(g []line) string {
	parts := make([]string, len(g))
	for i, l := range g {
		parts[i] = l.text
	}
	return strings.Join(parts, "\n")
}

var explanationLabel = regexp.MustCompile(`(?m)^(Pasal \d+[A-Z]?|Ayat \(\d+\)|Huruf [a-z]|Angka \d+)$`)

// penjelasan yang isinya cuma "Cukup jelas." tidak berguna untuk menjawab pertanyaan
func isEmptyExplanation(content string) bool {
	rest := explanationLabel.ReplaceAllString(content, "")
	rest = strings.ReplaceAll(rest, "Cukup jelas.", "")
	return strings.TrimSpace(rest) == ""
}
