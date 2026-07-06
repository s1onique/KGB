#!/usr/bin/env python3
"""Unit tests for tovarisch_status_rss_canary HTTP request helper."""

import sys, unittest
from unittest.mock import patch

from tovarisch_status_rss_canary_test_support import SCRIPTS_DIR
sys.path.insert(0, str(SCRIPTS_DIR))

import tovarisch_status_rss_canary_lib as canary


class TestHttpGet(unittest.TestCase):
    """Test HTTP request helper."""

    def test_success_returns_body(self):
        """Test successful GET returns body."""
        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_response = unittest.mock.MagicMock()
            mock_response.status = 200
            mock_response.read.return_value = b'{"status": "ok"}'
            mock_response.__enter__ = unittest.mock.MagicMock(return_value=mock_response)
            mock_response.__exit__ = unittest.mock.MagicMock(return_value=False)
            mock_urlopen.return_value = mock_response

            success, msg = canary.http_get("http://example.com/status", 2.0)

            self.assertTrue(success)
            self.assertEqual(msg, '{"status": "ok"}')

    def test_empty_body_returns_false(self):
        """Test empty body returns failure."""
        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_response = unittest.mock.MagicMock()
            mock_response.status = 200
            mock_response.read.return_value = b""
            mock_response.__enter__ = unittest.mock.MagicMock(return_value=mock_response)
            mock_response.__exit__ = unittest.mock.MagicMock(return_value=False)
            mock_urlopen.return_value = mock_response

            success, msg = canary.http_get("http://example.com/status", 2.0)

            self.assertFalse(success)
            self.assertEqual(msg, "empty_body")

    def test_non_2xx_returns_failure(self):
        """Test non-2xx response returns failure."""
        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_response = unittest.mock.MagicMock()
            mock_response.status = 404
            mock_response.read.return_value = b"Not Found"
            mock_response.__enter__ = unittest.mock.MagicMock(return_value=mock_response)
            mock_response.__exit__ = unittest.mock.MagicMock(return_value=False)
            mock_urlopen.return_value = mock_response

            success, msg = canary.http_get("http://example.com/status", 2.0)

            self.assertFalse(success)
            self.assertEqual(msg, "http_404")

    def test_http_error_returns_failure(self):
        """Test HTTPError returns failure with code."""
        import urllib.error

        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_urlopen.side_effect = urllib.error.HTTPError(
                "http://example.com/status", 500, "Internal Server Error", {}, None
            )

            success, msg = canary.http_get("http://example.com/status", 2.0)

            self.assertFalse(success)
            self.assertEqual(msg, "http_500")

    def test_url_error_returns_failure(self):
        """Test URLError returns failure with reason."""
        import urllib.error

        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_urlopen.side_effect = urllib.error.URLError("Connection refused")

            success, msg = canary.http_get("http://example.com/status", 2.0)

            self.assertFalse(success)
            self.assertEqual(msg, "url_error_Connection refused")

    def test_timeout_returns_failure(self):
        """Test TimeoutError returns failure."""
        with patch("urllib.request.urlopen") as mock_urlopen:
            mock_urlopen.side_effect = TimeoutError()

            success, msg = canary.http_get("http://example.com/status", 2.0)

            self.assertFalse(success)
            self.assertEqual(msg, "timeout")


if __name__ == "__main__":
    unittest.main(verbosity=2)
