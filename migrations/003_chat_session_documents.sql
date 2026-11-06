-- dokumen yang boleh dipakai tiap sesi chat; NULL = semua dokumen
alter table chat_sessions add column document_ids uuid[];
