import unittest

from safe_argv import build_argv


class SafeArgvTests(unittest.TestCase):
    def test_builds_an_argv_vector_without_shell_interpolation(self):
        self.assertEqual(
            build_argv("git", ["status", "--short", "name with spaces"]),
            ["git", "status", "--short", "name with spaces"],
        )
        self.assertEqual(build_argv("python3", ["-c", "print('ok')"]), [
            "python3", "-c", "print('ok')"
        ])

    def test_enforces_allowlist_strings_and_control_character_rejection(self):
        for executable in ("bash", "/bin/sh", "curl"):
            with self.subTest(executable=executable), self.assertRaises(ValueError):
                build_argv(executable, ["anything"])
        with self.assertRaises(TypeError):
            build_argv("git", "status")
        for argument in ("line\nbreak", "nul\0byte"):
            with self.subTest(argument=argument), self.assertRaises(ValueError):
                build_argv("git", [argument])


if __name__ == "__main__":
    unittest.main()
