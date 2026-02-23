"""Tests for db module — query building and identifier quoting."""

from wrds_dl.db import build_query, quote_ident


class TestQuoteIdent:
    def test_simple(self):
        assert quote_ident("foo") == '"foo"'

    def test_double_quotes(self):
        assert quote_ident('foo"bar') == '"foo""bar"'

    def test_empty(self):
        assert quote_ident("") == '""'

    def test_spaces(self):
        assert quote_ident("my table") == '"my table"'


class TestBuildQuery:
    def test_basic(self):
        q = build_query("crsp", "dsf")
        assert q == 'SELECT * FROM "crsp"."dsf"'

    def test_columns(self):
        q = build_query("crsp", "dsf", columns="permno,date,prc")
        assert q == 'SELECT "permno", "date", "prc" FROM "crsp"."dsf"'

    def test_where(self):
        q = build_query("crsp", "dsf", where="date = '2020-01-02'")
        assert q == """SELECT * FROM "crsp"."dsf" WHERE date = '2020-01-02'"""

    def test_limit(self):
        q = build_query("crsp", "dsf", limit=100)
        assert q == 'SELECT * FROM "crsp"."dsf" LIMIT 100'

    def test_all_options(self):
        q = build_query("comp", "funda", columns="gvkey,sale", where="fyear >= 2020", limit=1000)
        assert q == 'SELECT "gvkey", "sale" FROM "comp"."funda" WHERE fyear >= 2020 LIMIT 1000'

    def test_star_columns(self):
        q = build_query("crsp", "dsf", columns="*")
        assert q == 'SELECT * FROM "crsp"."dsf"'
