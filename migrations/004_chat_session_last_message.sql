-- waktu pesan terakhir; riwayat diurutkan dari sini supaya sesi lama yang dipakai lagi naik ke atas.
-- Rename tidak mengubahnya, jadi mengganti judul tidak menggeser urutan.
begin;

alter table chat_sessions add column last_message_at timestamptz not null default now();

update chat_sessions s
set last_message_at = coalesce(
  (select max(m.created_at) from chat_messages m where m.session_id = s.id),
  s.created_at
);

create index on chat_sessions (last_message_at desc);

commit;
