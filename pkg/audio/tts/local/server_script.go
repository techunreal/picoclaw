package local

// serverScript is a small, dependency-light OpenAI-compatible TTS server for
// the curated tiny-voca models. It is written to ~/.picoclaw/tts/server.py and
// run in the shared Python venv. PicoClaw owns this code; the model repository
// is never executed as-is.
//
// Adapters:
//
//	clarvia    wite-tech/clarvia-tts-tiny (Turkish, clarvia_phonemizer)
//	tiny-voca  n4ze3m/tiny-voca (English, dizione)
const serverScript = `#!/usr/bin/env python3
"""PicoClaw local TTS server for curated tiny-voca ONNX models."""
import argparse
import io
import json
import os
import re
import sys
import threading
import wave
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import numpy as np
import onnxruntime as ort

PEAK = 0.99
VERSION = "2"
_SENTENCE_END = re.compile(r"(?<=[.!?…])\s+(?=[\"'(\[]?[A-ZÇĞİÖŞÜ0-9])")


def sentences(text, language="tr"):
    text = re.sub(r"\s+", " ", text).strip()
    if text and not re.search(r"[.!?…,;:][\"'”’)\]»]*$", text):
        text += "."
    parts = _SENTENCE_END.split(text)
    if language != "tr":
        # English sentence starts after a period followed by a capital letter.
        parts = re.split(r"(?<=[.!?])\s+(?=[\"'(\[]?[A-Z0-9])", text)
    return [part for part in parts if re.search(r"\w", part)]


class BaseEngine:
    language = "tr"
    sample_rate = 24000

    def __init__(self, model_dir, threads=1):
        self.model_dir = Path(model_dir)
        self.config = json.loads((self.model_dir / "config.json").read_text(encoding="utf-8"))
        options = ort.SessionOptions()
        options.intra_op_num_threads = max(1, threads)
        self.session = ort.InferenceSession(
            str(self.model_dir / "onnx" / "model.onnx"),
            options,
            providers=["CPUExecutionProvider"],
        )
        self.lock = threading.Lock()
        self.voices = {}
        self.default_voice = ""
        self.max_tokens = 512

    def _max_tokens(self):
        return self.max_tokens

    def _split_long(self, sentence):
        parts = re.split(r"(?<=[,;:])\s+", sentence)
        if len(parts) == 1:
            words = sentence.split()
            if len(words) > 1:
                half = len(words) // 2
                parts = [" ".join(words[:half]), " ".join(words[half:])]
            else:
                parts = []
        return parts

    def _normalize(self, audio):
        audio = np.asarray(audio, dtype=np.float32).reshape(-1)
        loudest = float(np.abs(audio).max()) if audio.size else 0.0
        if loudest > PEAK:
            audio = audio * (PEAK / loudest)
        return audio

    def speak(self, text, voice=None, speed=1.0):
        raise NotImplementedError

    def voices_payload(self):
        return [{"id": name, "label": name} for name in self.voices]


class ClarviaEngine(BaseEngine):
    language = "tr"

    def __init__(self, model_dir, threads=1):
        super().__init__(model_dir, threads)
        import clarvia_phonemizer

        self.phonemizer = clarvia_phonemizer
        self.sample_rate = int(self.config.get("sample_rate") or 24000)
        public = set(self.config.get("public_voices") or self.config["voices"])
        self.voices = {
            name: index
            for index, name in enumerate(self.config["voices"])
            if name in public
        }
        self.default_voice = self.config.get("default_voice") or next(iter(self.voices))
        self.max_tokens = int(self.config.get("max_tokens") or 512)

    def speak(self, text, voice=None, speed=1.0):
        voice = voice or self.default_voice
        if voice not in self.voices:
            raise ValueError(
                "unknown voice %r; choose from %s"
                % (voice, ", ".join(sorted(self.voices)))
            )
        chunks = []
        pending = sentences(text, "tr")
        while pending:
            sentence = pending.pop(0)
            try:
                phones = self.phonemizer.phonemize(sentence)
            except ValueError:
                continue
            if len(phones.ids) > self._max_tokens() - 12:
                pending = self._split_long(sentence) + pending
                continue
            with self.lock:
                audio, _ = self.session.run(
                    None,
                    {
                        "tokens": np.asarray([phones.ids], dtype=np.int64),
                        "stress": np.asarray([phones.stress], dtype=np.int64),
                        "voice": np.asarray([self.voices[voice]], dtype=np.int64),
                        "speed": np.asarray([speed], dtype=np.float32),
                        "pitch_shift": np.asarray([0.0], dtype=np.float32),
                        "pitch_range": np.asarray([1.0], dtype=np.float32),
                    },
                )
            chunks.append(audio.reshape(-1))
        if not chunks:
            raise ValueError("nothing in the text could be read")
        return self._normalize(np.concatenate(chunks))


class TinyVocaEngine(BaseEngine):
    language = "en"

    def __init__(self, model_dir, threads=1):
        super().__init__(model_dir, threads)
        # dizione reads its data directory at import time.
        os.environ["DIZIONE_DATA"] = str(self.model_dir / "phonemizer")
        import dizione

        self.phonemizer = dizione
        self.sample_rate = int(
            self.config.get("sample_rate")
            or self.config.get("model", {}).get("sample_rate")
            or 24000
        )
        self.max_tokens = int(self.config.get("model", {}).get("max_tokens") or 512)
        self.voices = {
            name: index for index, name in enumerate(self.config.get("voices") or ["F2", "M2"])
        }
        self.default_voice = self.config.get("default_voice") or "F2"

    def speak(self, text, voice=None, speed=1.0):
        voice = voice or self.default_voice
        if voice not in self.voices:
            raise ValueError(
                "unknown voice %r; choose from %s"
                % (voice, ", ".join(sorted(self.voices)))
            )
        chunks = []
        pending = sentences(text, "en")
        while pending:
            sentence = pending.pop(0)
            try:
                phones = self.phonemizer.phonemize(sentence)
            except Exception:
                continue
            if len(phones.ids) > self._max_tokens() - 12:
                pending = self._split_long(sentence) + pending
                continue
            with self.lock:
                audio, _ = self.session.run(
                    None,
                    {
                        "tokens": np.asarray([phones.ids], dtype=np.int64),
                        "stress": np.asarray([phones.stress], dtype=np.int64),
                        "voice": np.asarray([self.voices[voice]], dtype=np.int64),
                        "speed": np.asarray([speed], dtype=np.float32),
                        "pitch_shift": np.asarray([0.0], dtype=np.float32),
                        "pitch_range": np.asarray([1.0], dtype=np.float32),
                    },
                )
            chunks.append(audio.reshape(-1))
        if not chunks:
            raise ValueError("nothing in the text could be read")
        return self._normalize(np.concatenate(chunks))


def to_wav(samples, sample_rate):
    pcm = (np.clip(samples, -1.0, 1.0) * 32767.0).astype("<i2").tobytes()
    buf = io.BytesIO()
    with wave.open(buf, "wb") as out:
        out.setnchannels(1)
        out.setsampwidth(2)
        out.setframerate(sample_rate)
        out.writeframes(pcm)
    return buf.getvalue()


class Handler(BaseHTTPRequestHandler):
    server_version = "PicoClawTTS/2.0"

    def log_message(self, fmt, *args):
        sys.stdout.write("%s - %s\n" % (self.address_string(), fmt % args))
        sys.stdout.flush()

    def _send_json(self, status, payload):
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _authorized(self):
        token = getattr(self.server, "token", "")
        if not token:
            return True
        return self.headers.get("Authorization", "") == "Bearer " + token

    def do_GET(self):
        path = self.path.split("?")[0]
        if path == "/v1/models":
            if not self._authorized():
                self._send_json(401, {"error": "unauthorized"})
                return
            self._send_json(200, {
                "object": "list",
                "data": [{
                    "id": getattr(self.server, "model_name", ""),
                    "object": "model",
                    "owned_by": "picoclaw-local",
                }],
            })
            return
        if path != "/health":
            self._send_json(404, {"error": "not found"})
            return
        if not self._authorized():
            self._send_json(401, {"error": "unauthorized"})
            return
        self._send_json(200, {
            "status": "ok",
            "version": VERSION,
            "model": getattr(self.server, "model_name", ""),
            "language": getattr(self.server, "tts").language,
            "voices": getattr(self.server, "tts").voices_payload(),
        })

    def do_POST(self):
        if self.path.split("?")[0] != "/v1/audio/speech":
            self._send_json(404, {"error": "not found"})
            return
        if not self._authorized():
            self._send_json(401, {"error": "unauthorized"})
            return
        try:
            length = int(self.headers.get("Content-Length") or 0)
            if length <= 0 or length > 1_000_000:
                raise ValueError("invalid request size")
            request = json.loads(self.rfile.read(length).decode("utf-8"))
            text = str(request.get("input") or "").strip()
            if not text:
                raise ValueError("input is required")
            if len(text) > 12000:
                raise ValueError("input is too long (max 12000 characters)")
            voice = str(request.get("voice") or self.server.tts.default_voice)
            speed = float(request.get("speed") or 1.0)
            samples = self.server.tts.speak(text, voice=voice, speed=speed)
            audio = to_wav(samples, int(self.server.tts.sample_rate))
            self.send_response(200)
            self.send_header("Content-Type", "audio/wav")
            self.send_header("Content-Length", str(len(audio)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(audio)
        except ValueError as exc:
            self._send_json(400, {"error": str(exc)})
        except Exception as exc:  # keep the server alive on unexpected input
            self._send_json(500, {"error": "%s: %s" % (type(exc).__name__, exc)})


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--model-dir", required=True)
    parser.add_argument("--port", type=int, default=0)
    parser.add_argument("--token", default="")
    parser.add_argument("--adapter", choices=["clarvia", "tiny-voca"], default="clarvia")
    parser.add_argument("--threads", type=int, default=1)
    args = parser.parse_args()

    if args.adapter == "tiny-voca":
        tts = TinyVocaEngine(args.model_dir, threads=args.threads)
    else:
        tts = ClarviaEngine(args.model_dir, threads=args.threads)

    httpd = ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    httpd.daemon_threads = True
    httpd.tts = tts
    httpd.token = args.token
    httpd.model_name = Path(args.model_dir).name
    print(
        "PicoClaw TTS (%s) listening on 127.0.0.1:%d (%s)"
        % (tts.language, httpd.server_address[1], httpd.model_name),
        flush=True,
    )
    httpd.serve_forever()


if __name__ == "__main__":
    main()
`
