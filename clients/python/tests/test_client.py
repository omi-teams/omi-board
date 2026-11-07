import io
import pathlib
import sys
import unittest
from email.message import Message
from unittest.mock import patch
from urllib.error import HTTPError

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
from omi_board_client import Client, Health, ProblemError
from omi_board_client._generated import _decode


class ClientTests(unittest.TestCase):
    @patch('omi_board_client._generated.urlopen')
    def test_health(self, open_url):
        open_url.return_value.__enter__.return_value = io.BytesIO(b'{"status":"ok"}')
        self.assertEqual(Client('http://localhost/').getV1Healthz(), Health('ok'))
        request = open_url.call_args.args[0]
        self.assertEqual(request.full_url, 'http://localhost/v1/healthz')
        self.assertEqual(open_url.call_args.kwargs['timeout'], 5)

    @patch('omi_board_client._generated.urlopen')
    def test_problem(self, open_url):
        headers = Message()
        headers['Content-Type'] = 'application/problem+json'
        open_url.side_effect = HTTPError('http://localhost', 503, 'Unavailable', headers,
            io.BytesIO(b'{"type":"about:blank","title":"Unavailable","status":503,"code":"database_unavailable"}'))
        with self.assertRaises(ProblemError) as caught:
            Client('http://localhost').getV1Healthz()
        self.assertEqual(caught.exception.problem.code, 'database_unavailable')
        self.assertEqual(caught.exception.problem.status, 503)

    @patch('omi_board_client._generated.urlopen')
    def test_non_problem_error(self, open_url):
        open_url.side_effect = HTTPError('http://localhost', 502, 'Bad gateway', Message(), io.BytesIO(b'oops'))
        with self.assertRaises(HTTPError):
            Client('http://localhost').getV1Healthz()

    def test_invalid_response(self):
        for data in ({}, {'status': 1}, None):
            with self.assertRaises(ValueError):
                _decode(Health, data)


if __name__ == '__main__':
    unittest.main()
