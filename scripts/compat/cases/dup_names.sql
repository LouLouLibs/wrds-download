SELECT a.id, b.id, a.id AS id_1, a.ch, b.ch FROM types a JOIN types b USING (id) ORDER BY a.id
