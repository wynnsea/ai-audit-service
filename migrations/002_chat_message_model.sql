-- model yang menyusun jawaban; kosong untuk pesan user dan jawaban "tidak ditemukan"
alter table chat_messages add column model text;
