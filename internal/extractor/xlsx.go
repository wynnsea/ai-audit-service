package extractor

import (
	"ai-audit-service/internal/entity"
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// extractXLSX mengubah tiap baris jadi "Kolom: nilai; ..." dengan baris pertama sebagai header,
// supaya setiap chunk tetap bisa dipahami walaupun header-nya ada di chunk lain
func extractXLSX(data []byte) ([]entity.Page, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("xlsx: open: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("xlsx: read sheet %q: %w", sheet, err)
		}
		if len(rows) == 0 {
			continue
		}

		header := rows[0]
		for i, row := range rows[1:] {
			var cells []string
			for j, v := range row {
				v = strings.Join(strings.Fields(v), " ") // isi sel multi-baris jadi satu baris
				if v == "" {
					continue
				}
				name := fmt.Sprintf("Kolom %d", j+1)
				if j < len(header) && strings.TrimSpace(header[j]) != "" {
					name = strings.TrimSpace(header[j])
				}
				cells = append(cells, name+": "+v)
			}
			if len(cells) == 0 {
				continue
			}
			// i+2: baris 1 = header, dan nomor baris Excel mulai dari 1
			fmt.Fprintf(&sb, "[%s, baris %d] %s\n", sheet, i+2, strings.Join(cells, "; "))
		}
	}

	return []entity.Page{{Text: sb.String()}}, nil
}
