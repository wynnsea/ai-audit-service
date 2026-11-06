-- Skema awal database (menggantikan 002_chunks_fts dan 003_embedding_halfvec).
-- Jalankan sekali di database kosong:
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/001_init.sql

begin;

-- schema "extensions" sudah ada di Supabase; di Postgres biasa dibuat di sini
create schema if not exists extensions;
create extension if not exists vector schema extensions;
set local search_path = public, extensions;

create table documents (
  id              uuid primary key default gen_random_uuid(),
  title           text not null,
  file_name       text not null,
  storage_path    text not null,
  regulation_type text,
  status          text not null default 'processing'
                  check (status in ('processing', 'ready', 'failed')),
  error_message   text,
  created_at      timestamptz not null default now()
);

create table document_chunks (
  id          uuid primary key default gen_random_uuid(),
  document_id uuid not null references documents (id) on delete cascade,
  chunk_index integer not null,
  section     text,
  page        integer,
  content     text not null,
  -- halfvec karena index hnsw untuk vector biasa dibatasi 2000 dimensi
  embedding   halfvec(2560),
  -- dihitung otomatis oleh Postgres tiap insert/update, Go nggak perlu ngisi
  fts         tsvector generated always as (
                to_tsvector('indonesian', coalesce(section, '') || ' ' || content)
              ) stored,
  unique (document_id, chunk_index)
);

create index on document_chunks using gin (fts);
create index on document_chunks using hnsw (embedding halfvec_cosine_ops);

create table chat_sessions (
  id         uuid primary key default gen_random_uuid(),
  title      text,
  created_at timestamptz not null default now()
);

create table chat_messages (
  id         uuid primary key default gen_random_uuid(),
  session_id uuid not null references chat_sessions (id) on delete cascade,
  role       text not null check (role in ('user', 'assistant')),
  content    text not null,
  sources    jsonb,
  created_at timestamptz not null default now()
);

create index on chat_messages (session_id, created_at);

-- tanpa policy: Data API Supabase (anon/authenticated) tidak bisa baca atau tulis.
-- Go konek sebagai pemilik tabel, jadi tidak terkena RLS.
alter table documents       enable row level security;
alter table document_chunks enable row level security;
alter table chat_sessions   enable row level security;
alter table chat_messages   enable row level security;

commit;
