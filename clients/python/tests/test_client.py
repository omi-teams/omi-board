import io
import json
import pathlib
import sys
import unittest
from email.message import Message
from unittest.mock import patch
from urllib.error import HTTPError

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
from omi_board_client import Client, Health, LoginRequest, ProblemError, Session, User
from omi_board_client._generated import _decode


class FakeResponse(io.BytesIO):
    """A urlopen response: a body plus the headers that cookie handling reads."""
    def __init__(self, body=b'', set_cookie=None):
        super().__init__(body)
        self.headers = Message()
        if set_cookie:
            self.headers['Set-Cookie'] = set_cookie

    def info(self):
        return self.headers


class ClientTests(unittest.TestCase):
    @patch('omi_board_client._generated.urlopen')
    def test_health(self, open_url):
        open_url.return_value.__enter__.return_value = FakeResponse(b'{"status":"ok"}')
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

    @patch('omi_board_client._generated.urlopen')
    def test_login_session_and_logout(self, open_url):
        client = Client('https://api.example.com')
        body = b'{"user":{"id":1,"email":"ann@example.com"},"csrf_token":"abc"}'
        open_url.return_value.__enter__.return_value = FakeResponse(body, 'omi_session=tok; Path=/; Secure; HttpOnly')
        session = client.postV1AuthLogin(LoginRequest('ann@example.com', 'pw'))
        self.assertEqual(session, Session(csrf_token='abc', user=User(email='ann@example.com', id=1)))
        request = open_url.call_args.args[0]
        self.assertEqual((request.get_method(), request.full_url), ('POST', 'https://api.example.com/v1/auth/login'))
        self.assertEqual(request.get_header('Content-type'), 'application/json')
        self.assertEqual(json.loads(request.data), {'email': 'ann@example.com', 'password': 'pw'})

        client.headers['X-CSRF-Token'] = session.csrf_token
        open_url.return_value.__enter__.return_value = FakeResponse(set_cookie='omi_session=; Path=/; Max-Age=0')
        self.assertIsNone(client.postV1AuthLogout())
        request = open_url.call_args.args[0]
        self.assertEqual(request.get_header('Cookie'), 'omi_session=tok')
        self.assertEqual(request.get_header('X-csrf-token'), 'abc')
        self.assertEqual(len(client.cookies), 0)

    @patch('omi_board_client._generated.urlopen')
    def test_unauthorized_problem(self, open_url):
        headers = Message()
        headers['Content-Type'] = 'application/problem+json'
        open_url.side_effect = HTTPError('http://localhost', 401, 'Unauthorized', headers,
            io.BytesIO(b'{"type":"about:blank","title":"Invalid email or password","status":401,"code":"invalid_credentials"}'))
        with self.assertRaises(ProblemError) as caught:
            Client('http://localhost').postV1AuthLogin(LoginRequest('a@b.c', 'x'))
        self.assertEqual(caught.exception.problem.status, 401)

    def test_invalid_response(self):
        for data in ({}, {'status': 1}, None):
            with self.assertRaises(ValueError):
                _decode(Health, data)
        with self.assertRaises(ValueError):
            _decode(Session, {'csrf_token': 'abc', 'user': {'id': 1}})


if __name__ == '__main__':
    unittest.main()
