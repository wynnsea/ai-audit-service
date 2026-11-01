package extractor

import (
	"ai-audit-service/internal/entity"
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

func extractDOCX(data []byte) ([]entity.Page, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("docx: open zip: %w", err)
	}

	var docXML *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docXML = f
			break
		}
	}
	if docXML == nil {
		return nil, errors.New("docx: word/document.xml not found")
	}

	rc, err := docXML.Open()
	if err != nil {
		return nil, fmt.Errorf("docx: open document.xml: %w", err)
	}
	defer rc.Close()

	var sb strings.Builder
	dec := xml.NewDecoder(rc)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("docx: parse xml: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				var s string
				if err := dec.DecodeElement(&s, &t); err != nil {
					return nil, fmt.Errorf("docx: read text: %w", err)
				}
				sb.WriteString(s)
			case "tab":
				sb.WriteByte('\t')
			case "br", "cr":
				sb.WriteByte('\n')
			}
		case xml.EndElement:
			if t.Name.Local == "p" {
				sb.WriteByte('\n')
			}
		}
	}

	return []entity.Page{{Text: sb.String()}}, nil
}
