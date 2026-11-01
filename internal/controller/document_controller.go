package controller

import (
	"ai-audit-service/internal/entity"
	"ai-audit-service/internal/pkg/response"
	"ai-audit-service/internal/repository"
	"ai-audit-service/internal/usecase"
	"errors"
	"io"
	"log"
	"net/http"

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
	})
	if errors.Is(err, usecase.ErrUnsupportedFile) {
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

var _ = entity.DocumentStatusReady // hapus baris ini kalau "entity" nggak kepakai di file ini
