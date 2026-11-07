"""Tool resolution regressions for the generation checker."""
import unittest
from unittest.mock import call, patch

import check_generated


class ToolResolutionTests(unittest.TestCase):
    @patch('check_generated.subprocess.run')
    @patch('shutil.which')
    def test_resolved_tool_paths(self, which, run):
        paths = {
            'go': r'C:\Program Files\Go\bin\go.exe',
            'npm': r'C:\Program Files\nodejs\npm.cmd',
        }
        which.side_effect = paths.get

        check_generated.generate()

        self.assertEqual(which.call_args_list, [call('go'), call('npm')])
        self.assertEqual(run.call_args_list, [
            call([paths['go'], 'generate', './...'],
                 cwd=check_generated.ROOT / 'server', check=True),
            call([paths['npm'], 'run', 'gen'],
                 cwd=check_generated.ROOT / 'web', check=True),
        ])

    @patch('check_generated.subprocess.run')
    @patch('shutil.which')
    def test_missing_tool(self, which, run):
        for missing in ('go', 'npm'):
            with self.subTest(tool=missing):
                which.side_effect = lambda name: None if name == missing else '/tools/' + name
                with self.assertRaisesRegex(SystemExit, 'Required tool not found on PATH: ' + missing):
                    check_generated.generate()
                run.assert_not_called()


if __name__ == '__main__':
    unittest.main()
