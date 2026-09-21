import tempfile
import unittest
from pathlib import Path

from archive_path import safe_archive_path


class ArchivePathTests(unittest.TestCase):
    def test_returns_a_contained_destination(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.assertEqual(
                safe_archive_path(root, "reports/2026/result.json"),
                root / "reports" / "2026" / "result.json",
            )

    def test_rejects_traversal_absolute_windows_and_nul_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in (
                "../secret",
                "reports/../../secret",
                "reports//result.json",
                "/etc/passwd",
                r"C:\Windows\system.ini",
                r"folder\file.txt",
                "bad\0name",
            ):
                with self.subTest(name=name), self.assertRaises(ValueError):
                    safe_archive_path(root, name)


if __name__ == "__main__":
    unittest.main()
