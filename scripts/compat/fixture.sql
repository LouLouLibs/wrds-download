-- Fixture for the parquet reader-compatibility harness (see README.md).
-- Each row of `types` exercises one family of values across every column.

CREATE TABLE types (
  id int4, b bool, i2 int2, i4 int4, i8 int8, f4 real, f8 double precision,
  n numeric, n10_2 numeric(10,2), n16_4 numeric(16,4), n17_4 numeric(17,4),
  d date, ts timestamp, tstz timestamptz, tm time,
  s text, vc varchar(8), ch char(4),
  j jsonb, u uuid, arr int4[], iv interval,
  allnull int4,
  "Mixed Case" text, "with space" int4, "é" text, "a.b" int4
);

INSERT INTO types VALUES
 -- 1: typical values
 (1, true, 1, 1, 1, 1.5, 0.1, 1.5, 12.34, 123456789012.3456, 1234567890123.4567,
  '2020-01-02', '2020-01-02 03:04:05.123456', '2020-01-02 10:00:00-05', '09:30:00.000001',
  'héllo', 'abc', 'AB', '{"a": 1}', 'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', '{1,2,NULL}', '1 day 2 hours',
  NULL, 'x', 1, 'é', 1),
 -- 2: zeros and empty values (must not become NULL)
 (2, false, 0, 0, 0, 0, 0, 0, 0, 0, 0,
  '1970-01-01', '1970-01-01 00:00:00', '1970-01-01 00:00:00+00', '00:00:00',
  '', '', '', '[]', '00000000-0000-0000-0000-000000000000', '{}', '0',
  NULL, '', 0, '', 0),
 -- 3: maxima and far-future values
 (3, true, 32767, 2147483647, 9223372036854775807, 3.4028235e38, 'Infinity', 'NaN',
  99999999.99, 999999999999.9999, 9999999999999.9999,
  '9999-12-31', '9999-12-31 23:59:59.999999', '9999-12-31 23:59:59.999999+00', '23:59:59.999999',
  E'tab\there\nnewline \\ backslash Ω≈ç√∫ 漢字 🎉', 'abcdefgh', 'ABCD',
  '{"nested": {"k": [1, "two", null]}}', 'ffffffff-ffff-ffff-ffff-ffffffffffff', '{{1,2},{3,4}}',
  '-3 mons -1 days', NULL, 'Z', 2147483647, 'Ω', -1),
 -- 4: minima, negatives and far-past values
 (4, false, -32768, -2147483648, -9223372036854775808, '-Infinity', '-0', '-Infinity',
  -99999999.99, -0.0001, -9999999999999.9999,
  '0001-01-01', '0001-01-01 00:00:00', '1899-12-31 23:59:59-05', '00:00:00.000001',
  ' leading and trailing ', ' ', '  a ', 'null', NULL, '{NULL}', '-00:00:00.000001',
  NULL, ' ', -2147483648, ' ', 0),
 -- 5: non-finite values
 (5, NULL, NULL, NULL, NULL, 'NaN', 'NaN', 'Infinity',
  NULL, NULL, NULL,
  'infinity', '-infinity', 'infinity', NULL,
  NULL, NULL, NULL, NULL, NULL, NULL, NULL,
  NULL, NULL, NULL, NULL, NULL),
 -- 6: all NULL
 (6, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL,
  NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL);

-- Wide table: 1000 columns alternating int8 / text / numeric(10,2) (comp.funda has ~1000).
DO $$
DECLARE cols text;
BEGIN
  SELECT string_agg(format('c%s %s', g, CASE g % 3 WHEN 0 THEN 'int8' WHEN 1 THEN 'text' ELSE 'numeric(10,2)' END), ', ' ORDER BY g)
    INTO cols FROM generate_series(1, 1000) g;
  EXECUTE format('CREATE TABLE wide (%s)', cols);
  FOR r IN 1..3 LOOP
    EXECUTE format('INSERT INTO wide VALUES (%s)', (
      SELECT string_agg(CASE g % 3 WHEN 0 THEN (g * r)::text
                                   WHEN 1 THEN quote_literal('v' || g || '_' || r)
                                   ELSE ((g * r) / 100.0)::text END, ', ' ORDER BY g)
      FROM generate_series(1, 1000) g));
  END LOOP;
END $$;
