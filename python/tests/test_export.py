"""Tests for export module — type mapping and format helpers."""

import pyarrow as pa

from wrds_dl.export import _arrow_type_for_oid, _format_row


class TestArrowTypeForOid:
    def test_bool(self):
        assert _arrow_type_for_oid(16) == pa.bool_()

    def test_int2(self):
        assert _arrow_type_for_oid(21) == pa.int32()

    def test_int4(self):
        assert _arrow_type_for_oid(23) == pa.int32()

    def test_int8(self):
        assert _arrow_type_for_oid(20) == pa.int64()

    def test_float4(self):
        assert _arrow_type_for_oid(700) == pa.float32()

    def test_float8(self):
        assert _arrow_type_for_oid(701) == pa.float64()

    def test_date(self):
        assert _arrow_type_for_oid(1082) == pa.date32()

    def test_timestamp(self):
        assert _arrow_type_for_oid(1114) == pa.timestamp("us")

    def test_timestamptz(self):
        assert _arrow_type_for_oid(1184) == pa.timestamp("us", tz="UTC")

    def test_unknown_defaults_to_string(self):
        assert _arrow_type_for_oid(9999) == pa.string()


class TestFormatRow:
    def test_basic(self):
        assert _format_row((1, "hello", None, 3.14)) == ["1", "hello", "", "3.14"]

    def test_empty(self):
        assert _format_row(()) == []

    def test_all_none(self):
        assert _format_row((None, None)) == ["", ""]
