SELECT g AS i,
       md5(g::text) || repeat('é', g % 50) AS s,
       CASE WHEN g % 7 = 0 THEN NULL ELSE (g * 0.5)::numeric(12,1) END AS x,
       CASE WHEN g % 1000 = 0 THEN repeat('long ', 20000) END AS big_text,
       date '2000-01-01' + g AS d
FROM generate_series(1, 30005) g ORDER BY g
