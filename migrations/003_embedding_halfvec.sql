-- kolom lama vector(1024) belum pernah terisi, aman dihapus (index-nya ikut terhapus)
alter table document_chunks drop column embedding;
alter table document_chunks add column embedding halfvec(2560);

create index on document_chunks using hnsw (embedding halfvec_cosine_ops);
