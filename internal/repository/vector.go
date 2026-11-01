package repository

import (
	"strconv"
	"strings"
)

// vectorLiteral mengubah []float32 jadi format teks pgvector "[0.1,0.2,...]".
// Dikirim sebagai text lalu di-cast di SQL ($n::text::halfvec), jadi nggak butuh library tambahan.
func vectorLiteral(v []float32) string {
	var sb strings.Builder
	sb.Grow(len(v) * 12)
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	sb.WriteByte(']')
	return sb.String()
}
