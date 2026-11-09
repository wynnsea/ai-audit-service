package controller

import (
	"ai-audit-service/internal/entity"
	"ai-audit-service/internal/pkg/response"
	"ai-audit-service/internal/repository"
	"ai-audit-service/internal/storage"
	"ai-audit-service/internal/usecase"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

const maxUploadSize = 40 << 20 // 40 MB

type DocumentController struct {
	uc *usecase.DocumentUsecase
}

func NewDocumentController(uc *usecase.DocumentUsecase) *DocumentController {
	return &DocumentController{uc: uc}
}

func (c *DocumentController) Upload(w http.ResponseWriter, r *http.Request) {
	// +1MB buat field form lain & boundary multipart
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+1<<20)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid form or file larger than 40MB")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		response.Error(w, http.StatusBadRequest, "field 'file' is required")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "failed to read file")
		return
	}

	doc, err := c.uc.Upload(r.Context(), usecase.UploadInput{
		Title:          r.FormValue("title"),
		RegulationType: r.FormValue("regulation_type"),
		FileName:       header.Filename,
		Content:        content,
		OCRModel:       r.FormValue("ocr_model"),
	})
	if errors.Is(err, usecase.ErrUnsupportedFile) || errors.Is(err, usecase.ErrUnknownOCRModel) {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		log.Printf("upload document: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	response.JSON(w, http.StatusAccepted, doc)
}

func (c *DocumentController) List(w http.ResponseWriter, r *http.Request) {
	docs, err := c.uc.List(r.Context())
	if err != nil {
		log.Printf("list documents: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.JSON(w, http.StatusOK, docs)
}

func (c *DocumentController) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		response.Error(w, http.StatusNotFound, "document not found")
		return
	}

	doc, err := c.uc.Get(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		response.Error(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		log.Printf("get document: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.JSON(w, http.StatusOK, doc)
}

// format yang bisa ditampilkan langsung oleh browser; selain ini (DOCX, XLSX) diunduh
var inlineTypes = map[string]bool{
	"application/pdf": true,
	"image/png":       true,
	"image/jpeg":      true,
	"text/plain":      true,
}

func (c *DocumentController) File(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		response.Error(w, http.StatusNotFound, "document not found")
		return
	}

	f, err := c.uc.OpenFile(r.Context(), id)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(w, http.StatusNotFound, "document not found")
		return
	case errors.Is(err, storage.ErrNotFound):
		response.Error(w, http.StatusNotFound, "original file is no longer in storage")
		return
	case err != nil:
		log.Printf("open document file: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer f.Body.Close()

	contentType := f.ContentType
	if contentType == "text/plain" {
		contentType += "; charset=utf-8"
	}
	disposition := "attachment"
	if inlineTypes[f.ContentType] {
		disposition = "inline"
	}

	w.Header().Set("Content-Type", contentType)
	// FormatMediaType otomatis memakai filename* untuk nama file non-ASCII
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Name}))
	// browser tidak boleh menebak tipe lain dari isi file (misal file .txt berisi HTML)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	if f.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}

	if _, err := io.Copy(w, f.Body); err != nil {
		// header sudah terkirim, jadi status tidak bisa diubah lagi; cukup dicatat
		log.Printf("stream document file %s: %v", id, err)
	}
}

func (c *DocumentController) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		response.Error(w, http.StatusNotFound, "document not found")
		return
	}

	err := c.uc.Delete(r.Context(), id)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		response.Error(w, http.StatusNotFound, "document not found")
	case err != nil:
		log.Printf("delete document: %v", err)
		response.Error(w, http.StatusInternalServerError, "internal error")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (c *DocumentController) ListOCRModels(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, c.uc.OCRModels())
}

var _ = entity.DocumentStatusReady // hapus baris ini kalau "entity" nggak kepakai di file ini
