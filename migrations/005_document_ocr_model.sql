-- model vision yang membaca halaman scan/gambar; NULL = dokumen tidak memakai OCR
alter table documents add column ocr_model text;
