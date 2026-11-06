package usecase

import (
	"ai-audit-service/internal/ai"
	"ai-audit-service/internal/entity"
	"ai-audit-service/internal/repository"
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	// kandidat dari vector search sebelum di-rerank
	searchCandidates = 20
	// kandidat teratas yang rujukan pasalnya ikut diambil
	expandFromTop = 5
	// batas kandidat tambahan dari rujukan pasal
	maxExpanded = 15
	// jumlah kutipan maksimal yang dikirim ke LLM; pertanyaan "apa saja" bisa butuh banyak pasal
	maxSources = 10
	// batas total isi kutipan (±3-4rb token) supaya prompt tetap jauh di bawah context 16k
	maxContextChars = 12000
	// hasil tes UU PDP: chunk relevan dapat skor >= 0.9, pertanyaan di luar dokumen <= 0.02
	minRerankScore = 0.05
	// pesan sebelumnya yang ikut dikirim supaya bisa tanya lanjutan
	historyMessages = 6
	// Qwen3-Embedding dilatih dengan instruksi di sisi query (dokumen tanpa instruksi)
	queryInstruction = "Instruct: Given a question about a regulation, retrieve the article that answers it\nQuery: "
	maxTitleRunes    = 60
)

var (
	// rujukan di isi pasal, contoh "pelanggaran terhadap ketentuan Pasal 32 ayat (1)"
	pasalRef = regexp.MustCompile(`\bPasal (\d+[A-Z]?)\b`)
	// nomor pasal dari section, contoh "BAB VI ... > Pasal 40 ayat (2)"
	pasalSection = regexp.MustCompile(`(?:^|> )Pasal (\d+[A-Z]?)`)
	// nomor sitasi di jawaban, contoh "[3]"
	citation = regexp.MustCompile(`\[(\d+)\]`)
)

var (
	ErrEmptyQuestion   = errors.New("question is required")
	ErrSessionNotFound = errors.New("chat session not found")
	ErrUnknownModel    = errors.New("model is not available")
	ErrEmptyTitle      = errors.New("title is required")
)

type ChunkSearcher interface {
	SearchSimilar(ctx context.Context, embedding []float32, limit int, documentIDs []string) ([]entity.SearchResult, error)
	FindByPasal(ctx context.Context, documentID string, numbers []string) ([]entity.SearchResult, error)
	FindReferencing(ctx context.Context, documentID, number string) ([]entity.SearchResult, error)
}

type ChatRepository interface {
	CreateSession(ctx context.Context, title string, documentIDs []string) (*entity.ChatSession, error)
	SetSessionDocuments(ctx context.Context, id string, documentIDs []string) error
	FindSession(ctx context.Context, id string) (*entity.ChatSession, error)
	ListSessions(ctx context.Context) ([]entity.ChatSession, error)
	RenameSession(ctx context.Context, id, title string) error
	DeleteSession(ctx context.Context, id string) error
	TouchSession(ctx context.Context, id string) error
	AddMessage(ctx context.Context, m *entity.ChatMessage) error
	ListMessages(ctx context.Context, sessionID string) ([]entity.ChatMessage, error)
}

type Reranker interface {
	Rerank(ctx context.Context, query string, documents []string) ([]float64, error)
}

type LLM interface {
	Chat(ctx context.Context, messages []ai.Message) (string, error)
	ChatWith(ctx context.Context, model string, messages []ai.Message) (string, error)
}

type AskInput struct {
	SessionID string
	Question  string
	Model     string // kosong = model default
	// dokumen yang boleh dipakai; kosong = semua dokumen
	DocumentIDs []string
}

type AskOutput struct {
	SessionID   string          `json:"session_id"`
	SearchQuery string          `json:"search_query"`
	Answer      string          `json:"answer"`
	Sources     []entity.Source `json:"sources"`
	Model       string          `json:"model,omitempty"`
}

type ChatUsecase struct {
	chats    ChatRepository
	chunks   ChunkSearcher
	embedder Embedder
	reranker Reranker
	llm      LLM
	models   []string // model yang boleh dipilih user; yang pertama = default
}

func NewChatUsecase(chats ChatRepository, chunks ChunkSearcher, embedder Embedder, reranker Reranker, llm LLM, models []string) *ChatUsecase {
	return &ChatUsecase{chats: chats, chunks: chunks, embedder: embedder, reranker: reranker, llm: llm, models: models}
}

func (u *ChatUsecase) Ask(ctx context.Context, in AskInput) (*AskOutput, error) {
	question := strings.TrimSpace(in.Question)
	if question == "" {
		return nil, ErrEmptyQuestion
	}

	model := in.Model
	if model == "" {
		model = u.models[0]
	}
	// daftar ini mencegah user memanggil model lain yang ada di gateway
	if !slices.Contains(u.models, model) {
		return nil, ErrUnknownModel
	}

	timer := newStepTimer()

	sessionID, history, err := u.prepareSession(ctx, in.SessionID, question, in.DocumentIDs)
	timer.lap("session")
	if err != nil {
		return nil, err
	}

	searchQuery := u.standaloneQuestion(ctx, history, question)
	timer.lap("rewrite")

	sources, err := u.retrieve(ctx, searchQuery, in.DocumentIDs, timer)
	if err != nil {
		return nil, err
	}
	if sources == nil {
		sources = []entity.Source{} // biar JSON-nya [] bukan null
	}

	// tanpa sumber yang relevan, LLM tidak dipanggil sama sekali, jadi tidak ada peluang mengarang
	answer, answeredBy := notFoundAnswer, ""
	if len(sources) > 0 {
		answeredBy = model
		answer, err = u.llm.ChatWith(ctx, model, buildMessages(history, sources, question))
		if err != nil {
			return nil, fmt.Errorf("generate answer: %w", err)
		}
		answer = removeInvalidCitations(answer, len(sources))
	}
	timer.lap("llm")

	if err := u.saveExchange(ctx, sessionID, question, answer, answeredBy, sources); err != nil {
		return nil, err
	}
	timer.lap("save")

	// log.Printf("chat ask: %s sources=%d query=%q", timer, len(sources), searchQuery)
	log.Printf("chat ask: %s model=%q documents=%d sources=%d query=%q", timer, answeredBy, len(in.DocumentIDs), len(sources), searchQuery)
	return &AskOutput{SessionID: sessionID, Answer: answer, Sources: sources, SearchQuery: searchQuery, Model: answeredBy}, nil
}

// prepareSession membuat sesi baru atau mengambil riwayat sesi yang ada, lalu mencatat
// dokumen yang dipilih supaya pilihannya kembali saat sesi dibuka lagi
func (u *ChatUsecase) prepareSession(ctx context.Context, sessionID, question string, documentIDs []string) (string, []ai.Message, error) {
	if sessionID == "" {
		s, err := u.chats.CreateSession(ctx, sessionTitle(question), documentIDs)
		if err != nil {
			return "", nil, fmt.Errorf("create session: %w", err)
		}
		return s.ID, nil, nil
	}

	session, err := u.chats.FindSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", nil, ErrSessionNotFound
		}
		return "", nil, fmt.Errorf("find session: %w", err)
	}
	if !slices.Equal(session.DocumentIDs, documentIDs) {
		if err := u.chats.SetSessionDocuments(ctx, sessionID, documentIDs); err != nil {
			return "", nil, fmt.Errorf("update session documents: %w", err)
		}
	}

	messages, err := u.chats.ListMessages(ctx, sessionID)
	if err != nil {
		return "", nil, err
	}
	messages = messages[max(0, len(messages)-historyMessages):]

	history := make([]ai.Message, len(messages))
	for i, m := range messages {
		history[i] = ai.Message{Role: m.Role, Content: m.Content}
	}
	return sessionID, history, nil
}

// standaloneQuestion menulis ulang pertanyaan lanjutan ("sanksinya apa?") jadi pertanyaan lengkap
// supaya pencarian dokumen tidak kehilangan topik. Gagal menulis ulang bukan alasan menggagalkan chat.
func (u *ChatUsecase) standaloneQuestion(ctx context.Context, history []ai.Message, question string) string {
	if len(history) == 0 {
		return question
	}

	rewritten, err := u.llm.Chat(ctx, buildRewriteMessages(history, question))
	if err != nil {
		log.Printf("rewrite question: %v (pakai pertanyaan asli)", err)
		return question
	}

	// model kadang ikut menulis label dari contoh di prompt
	rewritten = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rewritten), "Hasil:"))
	if rewritten == "" {
		return question
	}
	return rewritten
}

// retrieve: embed pertanyaan → vector search → rerank → ambil yang relevan
func (u *ChatUsecase) retrieve(ctx context.Context, question string, documentIDs []string, timer *stepTimer) ([]entity.Source, error) {
	vectors, err := u.embedder.Embed(ctx, []string{queryInstruction + question})
	if err != nil {
		return nil, fmt.Errorf("embed question: %w", err)
	}
	timer.lap("embed")

	candidates, err := u.chunks.SearchSimilar(ctx, vectors[0], searchCandidates, documentIDs)
	if err != nil {
		return nil, err
	}
	timer.lap("search")
	if len(candidates) == 0 {
		return nil, nil
	}
	candidates, err = u.expandReferences(ctx, candidates)
	if err != nil {
		return nil, err
	}
	timer.lap("expand")

	docs := make([]string, len(candidates))
	for i, c := range candidates {
		docs[i] = EmbeddingText(c.Chunk) // teks yang sama dengan waktu ingest
	}
	scores, err := u.reranker.Rerank(ctx, question, docs)
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}
	timer.lap("rerank")

	order := make([]int, len(candidates))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return scores[order[a]] > scores[order[b]] })

	var sources []entity.Source
	usedChars := 0
	for _, i := range order {
		if scores[i] < minRerankScore || len(sources) == maxSources {
			break
		}
		c := candidates[i]
		// lewati chunk yang bikin konteks kepanjangan, coba chunk berikutnya yang lebih pendek
		if usedChars+len(c.Content) > maxContextChars {
			continue
		}
		usedChars += len(c.Content)

		sources = append(sources, entity.Source{
			Number:        len(sources) + 1,
			ChunkID:       c.ChunkID,
			DocumentID:    c.DocumentID,
			DocumentTitle: c.DocumentTitle,
			Section:       c.Section,
			Page:          c.Page,
			Content:       c.Content,
			Score:         scores[i],
		})
	}
	return sources, nil
}

// expandReferences menambah kandidat dari rujukan antar pasal. Contoh: Pasal 32 (kewajiban memberi akses)
// tidak menyebut sanksi, tapi Pasal 57 menyebut "pelanggaran terhadap Pasal 32 ... dikenai sanksi
// administratif". Kandidat tambahan tetap dinilai reranker, jadi yang tidak relevan tersaring.
func (u *ChatUsecase) expandReferences(ctx context.Context, candidates []entity.SearchResult) ([]entity.SearchResult, error) {
	seen := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		seen[c.ChunkID] = true
	}

	var extra []entity.SearchResult
	add := func(found []entity.SearchResult) {
		for _, f := range found {
			if !seen[f.ChunkID] && len(extra) < maxExpanded {
				seen[f.ChunkID] = true
				extra = append(extra, f)
			}
		}
	}

	for _, c := range candidates[:min(expandFromTop, len(candidates))] {
		own := pasalNumber(c.Section)

		// rujukan keluar: pasal yang disebut di isi chunk ini
		var refs []string
		for _, m := range pasalRef.FindAllStringSubmatch(c.Content, -1) {
			if m[1] != own {
				refs = append(refs, m[1])
			}
		}
		if len(refs) > 0 {
			found, err := u.chunks.FindByPasal(ctx, c.DocumentID, refs)
			if err != nil {
				return nil, err
			}
			add(found)
		}

		// rujukan masuk: pasal lain yang menyebut pasal ini
		if own != "" {
			found, err := u.chunks.FindReferencing(ctx, c.DocumentID, own)
			if err != nil {
				return nil, err
			}
			add(found)
		}
	}
	return append(candidates, extra...), nil
}

// pasalNumber mengambil nomor pasal dari section; bagian Penjelasan diabaikan
// karena isinya menjelaskan pasal, bukan pasal itu sendiri
func pasalNumber(section string) string {
	if strings.HasPrefix(section, "Penjelasan") {
		return ""
	}
	if m := pasalSection.FindStringSubmatch(section); m != nil {
		return m[1]
	}
	return ""
}

func (u *ChatUsecase) saveExchange(ctx context.Context, sessionID, question, answer, model string, sources []entity.Source) error {
	userMsg := &entity.ChatMessage{SessionID: sessionID, Role: entity.RoleUser, Content: question}
	if err := u.chats.AddMessage(ctx, userMsg); err != nil {
		return fmt.Errorf("save question: %w", err)
	}
	botMsg := &entity.ChatMessage{SessionID: sessionID, Role: entity.RoleAssistant, Content: answer, Sources: sources, Model: model}
	if err := u.chats.AddMessage(ctx, botMsg); err != nil {
		return fmt.Errorf("save answer: %w", err)
	}
	if err := u.chats.TouchSession(ctx, sessionID); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (u *ChatUsecase) ChatModels() []string {
	return u.models
}

func (u *ChatUsecase) ListSessions(ctx context.Context) ([]entity.ChatSession, error) {
	return u.chats.ListSessions(ctx)
}

func (u *ChatUsecase) RenameSession(ctx context.Context, id, title string) (string, error) {
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return "", ErrEmptyTitle
	}
	if r := []rune(title); len(r) > maxTitleRunes {
		title = string(r[:maxTitleRunes])
	}
	if err := u.chats.RenameSession(ctx, id, title); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", ErrSessionNotFound
		}
		return "", err
	}
	return title, nil
}

func (u *ChatUsecase) DeleteSession(ctx context.Context, id string) error {
	if err := u.chats.DeleteSession(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrSessionNotFound
		}
		return err
	}
	return nil
}

func (u *ChatUsecase) ListMessages(ctx context.Context, sessionID string) ([]entity.ChatMessage, error) {
	if _, err := u.chats.FindSession(ctx, sessionID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	return u.chats.ListMessages(ctx, sessionID)
}

// removeInvalidCitations membuang [n] yang tidak menunjuk ke sumber mana pun, supaya UI tidak
// menampilkan rujukan palsu. Kejadiannya dicatat di log sebagai bahan evaluasi prompt.
func removeInvalidCitations(answer string, numSources int) string {
	return citation.ReplaceAllStringFunc(answer, func(m string) string {
		n, _ := strconv.Atoi(m[1 : len(m)-1])
		if n >= 1 && n <= numSources {
			return m
		}
		log.Printf("invalid citation %s removed (sources: %d)", m, numSources)
		return ""
	})
}

// judul sesi = potongan pertanyaan pertama
func sessionTitle(question string) string {
	r := []rune(question)
	if len(r) <= maxTitleRunes {
		return question
	}
	return string(r[:maxTitleRunes]) + "..."
}
