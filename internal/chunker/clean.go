package chunker

import (
	"ai-audit-service/internal/entity"
	"regexp"
	"strings"
)

// line adalah satu baris teks bersih beserta halaman asalnya
type line struct {
	text string
	page int
}

const (
	// berapa baris teratas/terbawah tiap halaman yang dicek sebagai header/footer
	edgeLines = 4
	// baris di area tepi yang muncul di >= 30% halaman (dan minimal 3 halaman) = header/footer
	repeatRatio = 0.3
	minRepeat   = 3
)

var (
	digits     = regexp.MustCompile(`\d+`)
	spaces     = regexp.MustCompile(`[ \t\x{00A0}]+`)
	pageNumber = regexp.MustCompile(`^[-–—]?\s*\d+\s*[-–—]?$`)
	// "kata sambung" di bawah halaman, contoh: "Pasal 7 . . ." atau "(2) Ketentuan ..."
	catchword = regexp.MustCompile(`^.{1,60}?\s*(\. ?\. ?\.|…)$`)
	// heading struktur sering jatuh di baris paling atas halaman dan setelah angkanya
	// dinormalisasi ("PASAL #") terlihat berulang, padahal itu isi dokumen
	structural = regexp.MustCompile(`^(BAB|Bagian|Paragraf|Pasal|Ayat|Huruf|Angka)\b|^Cukup jelas`)
)

// cleanPages membuang header, footer, nomor halaman, dan kata sambung,
// lalu menggabungkan semua halaman jadi satu daftar baris
func cleanPages(pages []entity.Page) []line {
	split := make([][]string, len(pages))
	for i, p := range pages {
		split[i] = nonEmptyLines(p.Text)
	}
	repeated := repeatedEdgeLines(split)

	var out []line
	for i, lines := range split {
		for _, l := range trimEdges(lines, repeated) {
			out = append(out, line{text: l, page: pages[i].Number})
		}
	}
	return out
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(spaces.ReplaceAllString(l, " "))
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// edgeKey menyamakan baris yang hanya beda angka/huruf besar-kecil,
// jadi "SK No 155204 A" dan "SK No 155205 A" dianggap baris yang sama
func edgeKey(l string) string {
	return digits.ReplaceAllString(strings.ToUpper(l), "#")
}

func edgeZone(lines []string) []string {
	if len(lines) <= edgeLines*2 {
		return lines
	}
	return append(append([]string{}, lines[:edgeLines]...), lines[len(lines)-edgeLines:]...)
}

func repeatedEdgeLines(pages [][]string) map[string]bool {
	counts := map[string]int{}
	for _, lines := range pages {
		seen := map[string]bool{}
		for _, l := range edgeZone(lines) {
			k := edgeKey(l)
			if !seen[k] {
				seen[k] = true
				counts[k]++
			}
		}
	}

	threshold := max(minRepeat, int(float64(len(pages))*repeatRatio))
	repeated := map[string]bool{}
	for k, n := range counts {
		if n >= threshold {
			repeated[k] = true
		}
	}
	return repeated
}

func isNoise(l string, repeated map[string]bool) bool {
	if structural.MatchString(l) {
		return false
	}
	return repeated[edgeKey(l)] || pageNumber.MatchString(l)
}

// trimEdges membuang semua baris di zona atas sampai baris noise terakhir, dan di zona bawah
// mulai dari baris noise pertama. Jadi varian kop yang jarang muncul (hasil OCR
// "REPUBLIC" vs "REPUBLIK") ikut terbuang selama diapit kop lain/nomor halaman.
func trimEdges(lines []string, repeated map[string]bool) []string {
	top := 0
	for i := 0; i < min(edgeLines, len(lines)); i++ {
		if isNoise(lines[i], repeated) {
			top = i + 1
		}
	}
	lines = lines[top:]

	bottom := len(lines)
	for i := len(lines) - 1; i >= max(0, len(lines)-edgeLines); i-- {
		if isNoise(lines[i], repeated) || catchword.MatchString(lines[i]) {
			bottom = i
		}
	}
	return lines[:bottom]
}
