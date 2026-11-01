package extractor

import (
	"ai-audit-service/internal/entity"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/sync/errgroup"
)

const (
	// halaman dengan huruf/angka di bawah ini dianggap hasil scan
	minTextChars = 50
	// gambar yang menutupi >= 80% halaman = halaman scan
	scanCoverage = 0.8
	// jumlah halaman yang di-OCR bersamaan
	ocrConcurrency = 4
)

func (e *Extractor) extractPDF(ctx context.Context, data []byte) ([]entity.Page, error) {
	dir, err := os.MkdirTemp("", "ingest-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	pdfPath := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(pdfPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("write temp pdf: %w", err)
	}

	texts, err := pdfTexts(ctx, pdfPath)
	if err != nil {
		return nil, err
	}
	scanned, err := scannedPages(ctx, pdfPath)
	if err != nil {
		return nil, err
	}

	pages := make([]entity.Page, len(texts))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(ocrConcurrency)

	for i, text := range texts {
		num := i + 1
		if !scanned[num] && countTextChars(text) >= minTextChars {
			pages[i] = entity.Page{Number: num, Text: text}
			continue
		}

		g.Go(func() error {
			ocrText, err := e.ocrPage(gctx, pdfPath, dir, num)
			if err != nil {
				return fmt.Errorf("ocr page %d: %w", num, err)
			}
			pages[i] = entity.Page{Number: num, Text: ocrText, OCR: true}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return pages, nil
}

func pdfTexts(ctx context.Context, pdfPath string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "pdftotext", "-layout", "-enc", "UTF-8", pdfPath, "-").Output()
	if err != nil {
		return nil, fmt.Errorf("pdftotext: %w", err)
	}
	// pdftotext menutup tiap halaman dengan form feed (\f)
	return strings.Split(strings.TrimSuffix(string(out), "\f"), "\f"), nil
}

// scannedPages menandai halaman yang isinya gambar hampir satu halaman penuh.
// PDF scan (misal salinan peraturan dari JDIH) sering sudah punya text layer
// hasil OCR lama yang kualitasnya jelek, jadi halaman seperti ini tetap di-OCR ulang.
func scannedPages(ctx context.Context, pdfPath string) (map[int]bool, error) {
	sizes, err := pageAreas(ctx, pdfPath)
	if err != nil {
		return nil, err
	}

	out, err := exec.CommandContext(ctx, "pdfimages", "-list", pdfPath).Output()
	if err != nil {
		return nil, fmt.Errorf("pdfimages: %w", err)
	}

	scanned := map[int]bool{}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[min(2, len(lines)):] { // 2 baris pertama = header tabel
		// kolom: page num type width height color comp bpc enc interp object ID x-ppi y-ppi ...
		f := strings.Fields(line)
		if len(f) < 14 {
			continue
		}
		page, _ := strconv.Atoi(f[0])
		width, _ := strconv.ParseFloat(f[3], 64)
		height, _ := strconv.ParseFloat(f[4], 64)
		xppi, _ := strconv.ParseFloat(f[12], 64)
		yppi, _ := strconv.ParseFloat(f[13], 64)

		pageArea, ok := sizes[page]
		if !ok || xppi == 0 || yppi == 0 {
			continue
		}
		// pixel / ppi = inch, x 72 = points (satuan ukuran halaman PDF)
		imgArea := (width / xppi * 72) * (height / yppi * 72)
		if imgArea >= scanCoverage*pageArea {
			scanned[page] = true
		}
	}
	return scanned, nil
}

var pageSizeLine = regexp.MustCompile(`(?m)^Page\s+(\d+) size:\s+([\d.]+) x ([\d.]+)`)

// pageAreas mengembalikan luas tiap halaman dalam points persegi
func pageAreas(ctx context.Context, pdfPath string) (map[int]float64, error) {
	// -l besar = sampai halaman terakhir, pdfinfo otomatis membatasi
	out, err := exec.CommandContext(ctx, "pdfinfo", "-f", "1", "-l", "100000", pdfPath).Output()
	if err != nil {
		return nil, fmt.Errorf("pdfinfo: %w", err)
	}

	areas := map[int]float64{}
	for _, m := range pageSizeLine.FindAllStringSubmatch(string(out), -1) {
		page, _ := strconv.Atoi(m[1])
		w, _ := strconv.ParseFloat(m[2], 64)
		h, _ := strconv.ParseFloat(m[3], 64)
		areas[page] = w * h
	}
	return areas, nil
}

func (e *Extractor) ocrPage(ctx context.Context, pdfPath, dir string, page int) (string, error) {
	prefix := filepath.Join(dir, "page-"+strconv.Itoa(page))
	p := strconv.Itoa(page)

	// -singlefile: output persis <prefix>.png tanpa akhiran nomor halaman
	cmd := exec.CommandContext(ctx, "pdftoppm", "-f", p, "-l", p, "-r", "150", "-png", "-singlefile", pdfPath, prefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftoppm: %w: %s", err, out)
	}

	img, err := os.ReadFile(prefix + ".png")
	if err != nil {
		return "", fmt.Errorf("read rendered page: %w", err)
	}
	return e.ocr.TranscribeImage(ctx, img)
}

func countTextChars(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			n++
		}
	}
	return n
}
