def test_parse_pair_contract():
    assert parse_pair("orphan_key") == ("orphan_key", None)
    assert parse_pair("key=value") == ("key", "value")
