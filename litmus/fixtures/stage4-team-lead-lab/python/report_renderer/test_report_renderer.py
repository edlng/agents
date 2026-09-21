import unittest

from report_renderer import render_report


class ReportRendererTests(unittest.TestCase):
    def test_renders_counts_and_deterministic_severity_order(self):
        report = render_report(
            [
                {"severity": "low", "title": "Whitespace", "owner": None},
                {"severity": "critical", "title": "Token leak", "owner": "api"},
                {"severity": "high", "title": "Open redirect", "owner": "web"},
            ]
        )
        self.assertIn("# Findings Report", report)
        self.assertIn("Total: 3", report)
        self.assertLess(report.index("Token leak"), report.index("Open redirect"))
        self.assertLess(report.index("Open redirect"), report.index("Whitespace"))
        self.assertIn("Unassigned", report)

    def test_escapes_markdown_table_delimiters_and_rejects_unknown_severity(self):
        report = render_report(
            [{"severity": "medium", "title": "A | B", "owner": "core"}]
        )
        self.assertIn(r"A \| B", report)
        with self.assertRaises(ValueError):
            render_report([{"severity": "urgent", "title": "X", "owner": "core"}])


if __name__ == "__main__":
    unittest.main()
