import unittest
from datetime import datetime, timezone

from retry_after import parse_retry_after


class RetryAfterTests(unittest.TestCase):
    def setUp(self):
        self.now = datetime(2026, 9, 15, 12, 0, tzinfo=timezone.utc)

    def test_parses_seconds_and_http_date(self):
        self.assertEqual(parse_retry_after("120", self.now), 120)
        self.assertEqual(
            parse_retry_after("Tue, 15 Sep 2026 12:01:30 GMT", self.now), 90
        )

    def test_clamps_past_dates_and_rejects_invalid_values(self):
        self.assertEqual(
            parse_retry_after("Tue, 15 Sep 2026 11:59:00 GMT", self.now), 0
        )
        for value in ("", "-2", "tomorrow", "1.5"):
            with self.assertRaises(ValueError):
                parse_retry_after(value, self.now)


if __name__ == "__main__":
    unittest.main()
