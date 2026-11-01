select cfgname from pg_ts_config where cfgname = 'indonesian';

-- dihitung otomatis oleh Postgres tiap insert/update, Go nggak perlu ngisi
alter table document_chunks
  add column fts tsvector
  generated always as (
    to_tsvector('indonesian', coalesce(section, '') || ' ' || content)
  ) stored;

create index on document_chunks using gin (fts);