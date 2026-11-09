package chunker

import (
	"ai-audit-service/internal/entity"
	"strings"
	"testing"
)

func sections(chunks []entity.Chunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Section
	}
	return out
}

func TestGenericHeadings(t *testing.T) {
	text := strings.Join([]string{
		"Halaman judul laporan",
		"DAFTAR ISI",
		"1.1 Latar Belakang ............................ 1",
		"BAB II ......................................... 9",
		"BAB I",
		"PENDAHULUAN",
		"1.1 Latar Belakang",
		"Isi latar belakang panjang.",
		"1. Menyiapkan peralatan",
		"2. Kalimat biasa yang diakhiri titik.",
		"1.2 Tujuan",
		"(1) Isi tujuan pertama.",
		"BAB II",
		"TINJAUAN PUSTAKA",
		"2.1 Landasan Teori",
		"2.1.1 Definisi",
		"Isi definisi.",
		"2.2 Penelitian Terdahulu",
		"Isi penelitian.",
		"A.8.2 Kontrol Akses",
		"Isi klausul ISO.",
		"LMS PPVSGI dipakai di sini.",
	}, "\n")

	got := sections(Split([]entity.Page{{Number: 1, Text: text}}))
	want := []string{
		"Awal dokumen",
		"BAB I PENDAHULUAN > 1.1 Latar Belakang",
		"BAB I PENDAHULUAN > 1.2 Tujuan",
		"BAB II TINJAUAN PUSTAKA > 2.1 Landasan Teori > 2.1.1 Definisi",
		"BAB II TINJAUAN PUSTAKA > 2.2 Penelitian Terdahulu",
		"BAB II TINJAUAN PUSTAKA > 2.2 Penelitian Terdahulu > A.8.2 Kontrol Akses",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("section salah:\n%s", strings.Join(got, "\n"))
	}
}

func TestGenericIgnoresFalseHeadings(t *testing.T) {
	text := strings.Join([]string{
		"BAB III",
		"METODE PENELITIAN",
		"3.1 Objek Penelitian",
		"Isi objek.",
		"1. Rancangan Awal Homepage",
		"Isi rancangan satu.",
		"2. Rancangan Awal Dashboard",
		"Isi rancangan dua.",
		"PRIVASI | SYARAT & KEBIJAKAN | KONTAK",
		"TULISAN KAPITAL DI DALAM GAMBAR",
		"Isi lanjutan rancangan dua.",
		"3.2 Teknik Analisis",
		"Isi analisis.",
		"DAFTAR PUSTAKA",
		"JUDUL ARTIKEL KAPITAL DALAM KUTIPAN",
		"Isi pustaka.",
	}, "\n")

	got := sections(Split([]entity.Page{{Number: 1, Text: text}}))
	want := []string{
		"BAB III METODE PENELITIAN > 3.1 Objek Penelitian",
		"BAB III METODE PENELITIAN > 3.1 Objek Penelitian > 1. Rancangan Awal Homepage",
		"BAB III METODE PENELITIAN > 3.1 Objek Penelitian > 2. Rancangan Awal Dashboard",
		"BAB III METODE PENELITIAN > 3.2 Teknik Analisis",
		"DAFTAR PUSTAKA JUDUL ARTIKEL KAPITAL DALAM KUTIPAN",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("section salah:\n%s", strings.Join(got, "\n"))
	}
}

func TestGenericLongSectionHasNoAyatLabel(t *testing.T) {
	var lines []string
	for range 80 {
		lines = append(lines, "(1) Kalimat isi yang cukup panjang supaya bagian ini terpecah jadi beberapa chunk.")
	}
	text := "2.1 Landasan Teori\n" + strings.Join(lines, "\n")

	chunks := Split([]entity.Page{{Number: 1, Text: text}})
	if len(chunks) < 2 {
		t.Fatalf("harus terpecah, dapat %d chunk", len(chunks))
	}
	if chunks[0].Section != "2.1 Landasan Teori" || chunks[1].Section != "2.1 Landasan Teori (lanjutan)" {
		t.Fatalf("label salah: %q, %q", chunks[0].Section, chunks[1].Section)
	}
}

func TestRegulationStillSplitByPasal(t *testing.T) {
	text := strings.Join([]string{
		"BAB I",
		"KETENTUAN UMUM",
		"Pasal 1",
		"(1) Isi pasal satu.",
		"Pasal 2",
		"Isi pasal dua.",
		"Pasal 3",
		"Isi pasal tiga.",
	}, "\n")

	got := sections(Split([]entity.Page{{Number: 1, Text: text}}))
	want := "BAB I KETENTUAN UMUM > Pasal 1\nBAB I KETENTUAN UMUM > Pasal 2\nBAB I KETENTUAN UMUM > Pasal 3"
	if strings.Join(got, "\n") != want {
		t.Fatalf("section regulasi berubah:\n%s", strings.Join(got, "\n"))
	}
}

func TestShortOCRDocumentStaysWhole(t *testing.T) {
	text := "KARTU KELUARGA\nNama Kepala Keluarga : M ARSYAD\n1 M ARSYAD 7602020401760001 LAKI-LAKI\nTIDAK TAHU 2 HUSNA YANTI 7602027112810019 PEREMPUAN"
	chunks := Split([]entity.Page{{Text: text, OCR: true}})
	if len(chunks) != 1 || chunks[0].Section != "KARTU KELUARGA" || !strings.Contains(chunks[0].Content, "HUSNA YANTI") {
		t.Fatalf("got %d chunks: %+v", len(chunks), chunks)
	}
}

func TestDataLinesAreNotHeadings(t *testing.T) {
	var b strings.Builder
	b.WriteString("DAFTAR ANGGOTA\n")
	for i := 0; i < 60; i++ {
		b.WriteString("| 1 | HUSNA YANTI | 7602027112810019 | PEREMPUAN |\n")
	}
	b.WriteString("TIDAK TAHU 2 HUSNA YANTI 7602027112810019 PEREMPUAN\n")
	for _, c := range Split([]entity.Page{{Text: b.String()}}) {
		if strings.Contains(c.Section, "HUSNA") {
			t.Fatalf("data jadi judul: %q", c.Section)
		}
	}
}
