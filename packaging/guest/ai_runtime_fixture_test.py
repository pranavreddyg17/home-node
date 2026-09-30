import io
import json
import unittest
import ai_runtime_fixture as fixture


def stream(content="hello", finish="stop"):
    messages = [{"choices": [{"delta": {"content": content}, "finish_reason": None}]},
                {"choices": [{"delta": {}, "finish_reason": finish}]}]
    return b"".join(b"data: " + json.dumps(m).encode() + b"\n\n" for m in messages) + b"data: [DONE]\n\n"


class InferenceStreamTests(unittest.TestCase):
    def test_complete_stream_with_token_limit(self):
        self.assertEqual(fixture.streaming_result(io.BytesIO(stream())), 5)
        self.assertEqual(fixture.streaming_result(io.BytesIO(stream("héllo", "length"))), 6)

    def test_pinned_server_initial_assistant_null_content(self):
        initial = b'data: {"choices":[{"delta":{"role":"assistant","content":null},"finish_reason":null}]}\n\n'
        self.assertEqual(fixture.streaming_result(io.BytesIO(initial + stream())), 5)
        for prefix in (initial + initial, initial.replace(b'assistant', b'tool'),
                       initial.replace(b'"role":"assistant",', b'')):
            with self.subTest(prefix=prefix), self.assertRaises(ValueError):
                fixture.streaming_result(io.BytesIO(prefix + stream()))

    def test_partial_empty_oversized_or_unapproved_output_refused(self):
        cases = [stream().replace(b"data: [DONE]\n\n", b""),
                 b"data: [DONE]\n\n", stream(""), stream("x" * 32769), stream(finish="tool_calls"),
                 stream(content=["unexpected"]), b"x" * 65537,
                 b"data: {\"choices\":[],\"choices\":[]}\n\n", b"data: {\"choices\":[]}\n\n"]
        for content in cases:
            with self.subTest(content=content[:40]), self.assertRaises(ValueError):
                fixture.streaming_result(io.BytesIO(content))


if __name__ == "__main__":
    unittest.main()
