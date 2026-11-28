import subprocess
import unittest
from unittest.mock import Mock

from check_migration_status import ROOT, check_current_status, status_is_current


class MigrationStatusTest(unittest.TestCase):
    def test_matching_current_and_target_versions_pass(self):
        self.assertTrue(status_is_current(0, "Versi database: 38; target: 38\n"))
        self.assertTrue(status_is_current(0, "Versi database: 39; target: 39\n"))

    def test_database_behind_target_fails(self):
        self.assertFalse(status_is_current(0, "Versi database: 38; target: 39\n"))

    def test_invalid_or_ambiguous_format_fails(self):
        self.assertFalse(status_is_current(0, "status tidak diketahui\n"))
        self.assertFalse(status_is_current(0, "Versi database: 39; target: 39\nextra\n"))
        self.assertFalse(status_is_current(0, "Versi database: x; target: 39\n"))
        self.assertFalse(status_is_current(0, "\nVersi database: 39; target: 39\n"))
        self.assertFalse(status_is_current(0, " Versi database: 39; target: 39\n"))

    def test_migrator_failure_fails_even_with_matching_output(self):
        self.assertFalse(status_is_current(1, "Versi database: 39; target: 39\n"))
        runner = Mock(return_value=subprocess.CompletedProcess([], 2, "", "error"))
        self.assertEqual(check_current_status(runner), (False, ""))
        runner.assert_called_once_with(
            ["go", "run", "./cmd/migrate", "status"],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=False,
        )


if __name__ == "__main__":
    unittest.main()
