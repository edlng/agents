import unittest

from dedupe_records import dedupe_records


class DedupeRecordTests(unittest.TestCase):
    def test_keeps_newest_record_but_preserves_first_id_order(self):
        records = [
            {"id": "b", "updated_at": "2026-01-01T00:00:00Z", "value": 1},
            {"id": "a", "updated_at": "2026-01-03T00:00:00Z", "value": 2},
            {"id": "b", "updated_at": "2026-01-02T00:00:00Z", "value": 3},
        ]
        self.assertEqual(
            dedupe_records(records),
            [
                {"id": "b", "updated_at": "2026-01-02T00:00:00Z", "value": 3},
                {"id": "a", "updated_at": "2026-01-03T00:00:00Z", "value": 2},
            ],
        )
        self.assertEqual(records[0]["value"], 1)

    def test_rejects_missing_ids_and_invalid_timestamps(self):
        with self.assertRaises(ValueError):
            dedupe_records([{"updated_at": "2026-01-01T00:00:00Z"}])
        with self.assertRaises(ValueError):
            dedupe_records([{"id": "a", "updated_at": "yesterday"}])


if __name__ == "__main__":
    unittest.main()
