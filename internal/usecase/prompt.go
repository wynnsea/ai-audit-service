package usecase

import (
	"ai-audit-service/internal/ai"
	"ai-audit-service/internal/entity"
	"fmt"
	"strings"

)
const notFoundAnswer = "Informasi tersebut tidak ditemukan dalam dokumen yang tersedia."

const systemPrompt = `Kamu adalah asisten audit kepatuhan. Tugasmu menjawab pertanyaan berdasarkan kutipan dokumen regulasi/standar yang diberikan di bagian KONTEKS.

Aturan:
1. Jawab HANYA berdasarkan KONTEKS. Jangan memakai pengetahuan di luar konteks dan jangan menebak.
2. Jika KONTEKS sama sekali tidak memuat jawaban, jawab persis: "` + notFoundAnswer + `"
3. Jika KONTEKS hanya menjawab sebagian, jawab bagian yang ada lalu sebutkan bagian mana yang tidak diatur dalam KONTEKS. Jangan menutup jawaban dengan kalimat "tidak ditemukan" bila sebagian jawaban sudah diberikan.
4. Setiap pernyataan wajib diberi nomor sumber dalam kurung siku sesuai KONTEKS, contoh: [1] atau [2][3].
5. Sebutkan pasal/klausul secara eksplisit (contoh: "Pasal 40 ayat (2)"). Nomor pasal WAJIB diambil dari label sumber tempat informasi itu berada, contoh: isi dari sumber "[3] Pasal 6" disebut Pasal 6. Jangan pernah menyimpulkan atau mengurutkan nomor pasal sendiri. Pasal sanksi sering berisi daftar nomor pasal yang dilanggar; jika pasal kewajiban yang ditanyakan tercantum dalam daftar itu, nyatakan dengan tegas bahwa pelanggarannya dikenai sanksi tersebut.
6. Untuk pertanyaan yang meminta daftar (misalnya "apa saja"), sebutkan semua butir yang ada di KONTEKS.
7. Tulis angka, jangka waktu, dan besaran sanksi persis seperti di dokumen.
8. Jawab dengan bahasa yang sama dengan pertanyaan. Ringkas dan terstruktur.
9. Judul dokumen tertera di label tiap sumber. Jika pertanyaan menyebut dokumen atau penulis tertentu, jawab hanya dari sumber dokumen itu. Jangan menggabungkan isi dokumen berbeda seolah-olah berasal dari satu dokumen; jika jawaban memakai lebih dari satu dokumen, sebutkan nama dokumennya.`

// buildMessages menyusun percakapan untuk LLM: aturan, riwayat chat, lalu konteks + pertanyaan baru
func buildMessages(history []ai.Message, sources []entity.Source, question string) []ai.Message {
	messages := make([]ai.Message, 0, len(history)+2)
	messages = append(messages, ai.Message{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, ai.Message{
		Role: "user",
		// rentang nomor ditulis eksplisit: tanpa ini model 9B cenderung terus menghitung
		// melewati jumlah sumber ([11], [12]) saat menyusun daftar panjang
		Content: fmt.Sprintf(
			"KONTEKS (sumber yang tersedia hanya [1] sampai [%d]):\n%s\n\nPERTANYAAN: %s\n\n"+
				"Ingat: gunakan hanya nomor sumber [1] sampai [%d], sesuai sumber tempat informasi itu berada.",
			len(sources), formatContext(sources), question, len(sources)),
	})
	return messages
}

// formatContext menulis tiap sumber sebagai blok bernomor dengan label pasal di depan, contoh:
// [1] Pasal 40 — BAB VI ... > Bagian Kedua ... — UU 27/2022 Pelindungan Data Pribadi, hal. 17
// Label di depan mencegah model salah menebak nomor pasal saat sumbernya banyak.
func formatContext(sources []entity.Source) string {
	var sb strings.Builder
	for i, s := range sources {
		if i > 0 {
			sb.WriteString("\n\n")
		}

		label, path := s.Section, ""
		if idx := strings.LastIndex(s.Section, " > "); idx >= 0 {
			label, path = s.Section[idx+3:], s.Section[:idx]
		}

		fmt.Fprintf(&sb, "[%d] %s", s.Number, label)
		if path != "" {
			fmt.Fprintf(&sb, " — %s", path)
		}
		fmt.Fprintf(&sb, " — %s", s.DocumentTitle)
		if s.Page > 0 {
			fmt.Fprintf(&sb, ", hal. %d", s.Page)
		}
		sb.WriteString("\n")
		sb.WriteString(s.Content)
	}
	return sb.String()
}


// rewritePrompt meminta LLM membuat pertanyaan lanjutan jadi mandiri, khusus untuk pencarian dokumen.
// Contoh kedua (pertanyaan yang sudah jelas) penting supaya model tidak "menambah-nambahi".
const rewritePrompt = `Tugasmu: ubah PERTANYAAN TERAKHIR pengguna menjadi pertanyaan mandiri yang bisa dipahami tanpa membaca riwayat. Hasilnya dipakai untuk mencari dokumen, bukan untuk dijawab.

Aturan:
1. Jika pertanyaan terakhir sudah jelas tanpa riwayat (punya subjek/topik sendiri), salin PERSIS apa adanya.
2. Jika pertanyaan merujuk riwayat (misalnya "itu", "tersebut", "sanksinya", "kalau korporasi", "pidananya"), ganti rujukan itu dengan topik konkret yang sedang dibahas di riwayat.
3. Topik diambil dari PERTANYAAN pengguna di riwayat. Jangan menyalin nomor pasal, angka, atau isi jawaban asisten.
4. Jangan menjawab. Jangan menambah hal yang tidak ditanyakan.
5. Gunakan bahasa yang sama dengan pertanyaan terakhir. Keluarkan satu kalimat pertanyaan saja.

Contoh:
Riwayat: Pengguna: berapa lama batas waktu menghapus data pribadi?
Pertanyaan terakhir: kalau dilanggar dendanya berapa?
Hasil: Berapa denda jika batas waktu menghapus data pribadi dilanggar?

Riwayat: Pengguna: apa kewajiban pengendali data pribadi?
Pertanyaan terakhir: apa yang dimaksud dengan data biometrik?
Hasil: apa yang dimaksud dengan data biometrik?`

// jawaban asisten bisa panjang; potongan awalnya sudah cukup untuk menangkap topik
const rewriteHistoryRunes = 500

func buildRewriteMessages(history []ai.Message, question string) []ai.Message {
	var sb strings.Builder
	for _, m := range history {
		speaker := "Pengguna"
		if m.Role == entity.RoleAssistant {
			speaker = "Asisten"
		}
		content := []rune(m.Content)
		if len(content) > rewriteHistoryRunes {
			content = content[:rewriteHistoryRunes]
		}
		fmt.Fprintf(&sb, "%s: %s\n", speaker, string(content))
	}

	return []ai.Message{
		{Role: "system", Content: rewritePrompt},
		{Role: "user", Content: "Riwayat:\n" + sb.String() + "\nPertanyaan terakhir: " + question},
	}
}
