import unittest
from prepare_release import next_version


class VersionTests(unittest.TestCase):
    def test_first_release(self):
        self.assertEqual(next_version("2.0.3", []), ("2.0.3", 2000003))

    def test_every_published_update_increases(self):
        self.assertEqual(next_version("2.0.3", ["v2.0.3"]), ("2.0.4", 2000004))

    def test_failed_reserved_build_does_not_skip_version(self):
        self.assertEqual(next_version("2.0.4", ["v2.0.3"]), ("2.0.4", 2000004))

    def test_rollback_is_rejected(self):
        with self.assertRaises(ValueError):
            next_version("2.0.3", ["v2.0.3"], "2.0.3")

    def test_nonrelease_tags_ignored(self):
        self.assertEqual(next_version("2.0.3", ["preview", "v2.0.3-beta"]), ("2.0.3", 2000003))

    def test_rollover_keeps_android_code_increasing(self):
        self.assertEqual(next_version("2.0.999", ["v2.0.999"]), ("2.1.0", 2001000))


if __name__ == "__main__":
    unittest.main()