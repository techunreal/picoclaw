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
//	espeak     eSpeak NG front end described by picoclaw.json (data only)
const serverScript = `#!/usr/bin/env python3
"""PicoClaw local TTS server for curated tiny-voca ONNX models."""
import argparse
import io
import json
import os
import re
import shutil
import subprocess
import sys
import threading
import unicodedata
import wave
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

try:
    import numpy as np
    import onnxruntime as ort
except ImportError:  # --frontend-test only needs the standard library
    np = None
    ort = None

PEAK = 0.99
VERSION = "3"
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


class NothingToPronounce(ValueError):
    pass


_QUOTES = str.maketrans({
    "‘": "'", "’": "'", "‚": "'", "‛": "'", chr(96): "'", "´": "'",
    "“": '"', "”": '"', "„": '"', "‟": '"', "«": '"', "»": '"',
    "–": "—", "―": "—", "[": "(", "{": "(", "]": ")", "}": ")",
})
_LANGUAGE_TAG = re.compile(r"\((?:_?[a-z0-9]_?){2,3}(?:-(?:_?[a-z0-9]_?)+)*\)")
_WORD_BEFORE = re.compile(r"([^\W\d_]+)$")
_STRESS_MARKS = {"ˈ": 1, "ˌ": 2}


class EspeakFrontend:
    """Data-driven eSpeak NG front end.

    The phoneme inventory, fallback table and text rules come from the model
    manifest. No code from the model repository is executed.
    """

    def __init__(self, spec, config):
        self.spec = spec or {}
        self.config = config or {}
        symbols = self.spec.get("symbols") or self.config.get("symbols") or []
        if len(symbols) < 5:
            raise ValueError("frontend symbols must contain at least five specials")
        self.symbols = list(symbols)
        self.pad, self.bos, self.eos, self.sp, self.unk = self.symbols[:5]
        self.symbol_to_id = {symbol: index for index, symbol in enumerate(self.symbols)}
        self.punctuation = list(self.spec.get("punctuation") or (
            ",", ".", "?", "!", ";", ":", '"', "'", "(", ")", "—", "…", "¿", "¡"))
        self.punctuation_set = frozenset(self.punctuation)
        self.fallback = {
            key: tuple(value) for key, value in (self.spec.get("fallback") or {}).items()
        }
        self.vowels = frozenset(self.spec.get("vowels") or ())
        self.abbreviations = frozenset(
            item.lower() for item in (self.spec.get("abbreviations") or ()))
        self.not_an_end = tuple(self.spec.get("not_an_end") or ())
        self.sentence_start = self.spec.get("sentence_start") or "A-ZÇĞİÖŞÜ0-9"
        self.voice = self.spec.get("espeak_voice") or self.config.get("language") or "en"
        self.phones = frozenset(
            symbol for symbol in self.symbols
            if symbol not in (self.pad, self.bos, self.eos, self.sp, self.unk)
            and symbol not in self.punctuation_set
        )
        self.phone_ids = frozenset(self.symbol_to_id[p] for p in self.phones) | {
            self.symbol_to_id[self.unk]
        }
        self._sentence_end = re.compile(
            r"(?:(?<=[.!?…])|(?<=[.!?…][\"')\]]))\s+(?=[\"'(\[]?[" +
            self.sentence_start + r"])")
        self._not_an_end = re.compile(
            r"(?:\b(?:" + "|".join(re.escape(word) for word in self.not_an_end) +
            r")|\b[^\W\d_])\.$", re.IGNORECASE)
        self._clause_end = re.compile(r"(?<=[,;:—])\s+")

    def normalize_text(self, text):
        text = unicodedata.normalize("NFKC", text).translate(_QUOTES)
        text = re.sub(r"\.{3,}", "…", text)
        text = re.sub(r"-{2,}|\s-\s", "—", text)
        return re.sub(r"\s+", " ", text).strip()

    def chunks(self, text):
        start = 0
        for i, char in enumerate(text):
            if char not in self.punctuation_set:
                continue
            previous = text[i - 1] if i else ""
            following = text[i + 1] if i + 1 < len(text) else ""
            if char == "'" and previous.isalnum() and following.isalnum():
                continue
            if char in ".,:" and previous.isdigit() and following.isdigit():
                continue
            if char == ".":
                word = _WORD_BEFORE.search(text[:i])
                if word and word[1].lower() in self.abbreviations:
                    continue
                if previous.isalpha() and following.isalpha():
                    continue
            if text[start:i].strip():
                yield False, text[start:i].strip()
            yield True, char
            start = i + 1
        if text[start:].strip():
            yield False, text[start:].strip()

    def espeak_words(self, piece):
        executable = shutil.which("espeak-ng")
        if executable is None:
            raise RuntimeError(
                "espeak-ng is not installed; install eSpeak NG 1.52.0 and restart the sidecar")
        result = subprocess.run(
            [executable, "-q", "--ipa", "--sep=_", "-v", self.voice, "--stdin"],
            input=piece.rstrip("\n") + "\n", text=True, capture_output=True,
            check=True, timeout=30)
        return self.parse_espeak(result.stdout)

    def parse_espeak(self, output):
        words = []
        for word in _LANGUAGE_TAG.sub("", output).split():
            units = []
            for unit in word.split("_"):
                stress = 0
                for mark, level in _STRESS_MARKS.items():
                    if mark in unit:
                        stress = stress or level
                        unit = unit.replace(mark, "")
                unit = "".join(
                    c for c in unit
                    if unicodedata.category(c) != "Cf" and c not in "()")
                if unit:
                    units.append((unit, stress))
            if units:
                words.append(units)
        return words

    def canonical(self, phone):
        if phone in self.phones:
            return (phone,)
        if phone in self.fallback:
            return self.fallback[phone]
        stripped = "".join(
            c for c in unicodedata.normalize("NFD", phone)
            if not unicodedata.combining(c))
        for candidate in (stripped, stripped.replace("ː", ""), stripped + "ː"):
            if candidate != phone and candidate in self.phones:
                return (candidate,)
            if candidate != phone and candidate in self.fallback:
                return self.fallback[candidate]
        return None

    def tokenize(self, pieces):
        symbols, stress, unknown = [self.bos], [0], []
        for punctuation, value in pieces:
            if punctuation:
                if symbols[-1] != value:
                    symbols.append(value)
                    stress.append(0)
                continue
            for word in value:
                if symbols[-1] in self.phones or symbols[-1] == self.unk:
                    symbols.append(self.sp)
                    stress.append(0)
                for phone, level in word:
                    mapped = self.canonical(phone)
                    if mapped is None:
                        unknown.append(phone)
                        mapped = (self.unk,)
                    carrier = next(
                        (i for i, item in enumerate(mapped) if item in self.vowels), 0)
                    for index, item in enumerate(mapped):
                        symbols.append(item)
                        stress.append(level if index == carrier else 0)
        symbols.append(self.eos)
        stress.append(0)
        if not any(s in self.phones or s == self.unk for s in symbols):
            raise NothingToPronounce("the text has nothing to pronounce")
        ids = [self.symbol_to_id[symbol] for symbol in symbols]
        return ids, stress, tuple(unknown)

    def phonemize(self, text):
        if not isinstance(text, str) or not text.strip():
            raise ValueError("text must not be empty")
        pieces = []
        for punctuation, piece in self.chunks(self.normalize_text(text)):
            pieces.append((True, piece) if punctuation else (False, self.espeak_words(piece)))
        return self.tokenize(pieces)

    def sentences(self, text):
        parts, current = [], ""
        for piece in self._sentence_end.split(re.sub(r"\s+", " ", text).strip()):
            current = (current + " " + piece) if current else piece
            if not self._not_an_end.search(current):
                parts.append(current)
                current = ""
        if current:
            parts.append(current)
        return [part for part in parts if any(char.isalnum() for char in part)]

    def finished(self, text):
        body = re.sub(r"\s+$", "", text)
        if not body:
            return text
        last = re.split(r"\s+", body)[-1]
        if any(char.isalnum() for char in last):
            marked = last.rstrip("\"'”’)]»")[-1:] in tuple(".!?…,;:")
        else:
            marked = any(char in ".!?…" for char in last) or last in (",", ";", ":")
        return text if marked else body + "."

    def shorter(self, text):
        pieces = self._clause_end.split(text)
        if len(pieces) > 1:
            return pieces
        words = text.split(" ")
        if len(words) < 2:
            return []
        half = (len(words) + 1) // 2
        return [" ".join(words[:half]), " ".join(words[half:])]


class BaseEngine:
    language = "tr"
    sample_rate = 24000

    def __init__(self, model_dir, threads=1):
        self.model_dir = Path(model_dir)
        self.profile = self._read_profile()
        config_name = self.profile.get("config_file") or "config.json"
        self.config = json.loads(
            (self.model_dir / config_name).read_text(encoding="utf-8"))
        model_name = self.profile.get("model_file") or "onnx/model.onnx"
        options = ort.SessionOptions()
        options.intra_op_num_threads = max(1, threads)
        self.session = ort.InferenceSession(
            str(self.model_dir / model_name),
            options,
            providers=["CPUExecutionProvider"],
        )
        self.lock = threading.Lock()
        self.voices = {}
        self.default_voice = ""
        self.max_tokens = 512

    def _read_profile(self):
        path = self.model_dir / "picoclaw.json"
        if not path.exists():
            return {}
        return json.loads(path.read_text(encoding="utf-8"))

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


class EspeakEngine(BaseEngine):
    language = "es"

    def __init__(self, model_dir, threads=1):
        super().__init__(model_dir, threads)
        self.language = str(self.profile.get("language") or "es")
        self.sample_rate = int(self.profile.get("sample_rate") or 24000)
        self.max_tokens = int(self.profile.get("max_tokens") or 512)
        self.frontend = EspeakFrontend(self.profile.get("frontend") or {}, self.config)

        declared = self.profile.get("voices") or []
        if declared and isinstance(declared[0], dict):
            names = [str(item.get("id")) for item in declared]
            self.labels = {str(item.get("id")): str(item.get("label") or item.get("id"))
                           for item in declared}
        else:
            names = [str(item) for item in (declared or self.config.get("voices")
                                            or self.config.get("speakers") or [])]
            self.labels = {name: name for name in names}
        self.voices = {name: index for index, name in enumerate(names)}
        if not self.voices:
            raise ValueError("the local profile does not declare any voices")
        self.default_voice = str(self.profile.get("default_voice") or names[0])

    def voices_payload(self):
        return [{"id": name, "label": self.labels.get(name, name)}
                for name in self.voices]

    def speak(self, text, voice=None, speed=1.0):
        voice = voice or self.default_voice
        if voice not in self.voices:
            raise ValueError(
                "unknown voice %r; choose from %s"
                % (voice, ", ".join(sorted(self.voices)))
            )
        chunks = []
        pending = self.frontend.sentences(self.frontend.finished(text))
        while pending:
            sentence = pending.pop(0)
            try:
                ids, stress, _ = self.frontend.phonemize(sentence)
            except NothingToPronounce:
                continue
            except ValueError:
                continue
            if len(ids) > self._max_tokens() - 12:
                pending = self.frontend.shorter(sentence) + pending
                continue
            with self.lock:
                audio, _ = self.session.run(
                    None,
                    {
                        "tokens": np.asarray([ids], dtype=np.int64),
                        "stress": np.asarray([stress], dtype=np.int64),
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
    server_version = "PicoClawTTS/3.0"

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


def run_frontend_test(path):
    with open(path, "r", encoding="utf-8") as handle:
        suite = json.load(handle)
    frontend = EspeakFrontend(suite.get("frontend") or {}, {})
    results = []
    failures = []

    def check(name, actual, expected):
        ok = actual == expected
        results.append({"name": name, "ok": ok})
        if not ok:
            failures.append({"name": name, "actual": actual, "expected": expected})

    for case in suite.get("cases") or []:
        op = case.get("op")
        name = case.get("name") or op
        if op == "normalize":
            check(name, frontend.normalize_text(case["text"]), case["expect"])
        elif op == "chunks":
            pieces = [[flag, value] for flag, value in frontend.chunks(
                frontend.normalize_text(case["text"]))]
            check(name, pieces, case["expect"])
        elif op == "parse":
            actual = [[[phone, stress] for phone, stress in word]
                      for word in frontend.parse_espeak(case["espeak"])]
            check(name, actual, case["expect"])
        elif op == "canonical":
            mapped = frontend.canonical(case["phone"])
            check(name, list(mapped) if mapped is not None else None, case["expect"])
        elif op == "tokenize":
            pieces = []
            for flag, value in case["pieces"]:
                pieces.append((flag, value if flag else frontend.parse_espeak(value)))
            ids, stress, unknown = frontend.tokenize(pieces)
            symbols = [frontend.symbols[index] for index in ids]
            check(name + ":symbols", symbols, case["expect_symbols"])
            check(name + ":stress", stress, case["expect_stress"])
            if "expect_unknown" in case:
                check(name + ":unknown", list(unknown), case["expect_unknown"])
        elif op == "sentences":
            check(name, frontend.sentences(case["text"]), case["expect"])
        elif op == "finished":
            check(name, frontend.finished(case["text"]), case["expect"])
        else:
            failures.append({"name": name, "error": "unknown op %r" % (op,)})

    print(json.dumps({"ok": not failures, "results": results, "failures": failures},
                     ensure_ascii=False))
    if failures:
        raise SystemExit(1)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--model-dir", default="")
    parser.add_argument("--port", type=int, default=0)
    parser.add_argument("--token", default="")
    parser.add_argument(
        "--adapter", choices=["clarvia", "tiny-voca", "espeak"], default="clarvia")
    parser.add_argument("--threads", type=int, default=1)
    parser.add_argument("--frontend-test", default="")
    args = parser.parse_args()

    if args.frontend_test:
        run_frontend_test(args.frontend_test)
        return
    if not args.model_dir:
        parser.error("--model-dir is required")
    if np is None or ort is None:
        parser.error("numpy and onnxruntime are required")

    if args.adapter == "espeak":
        tts = EspeakEngine(args.model_dir, threads=args.threads)
    elif args.adapter == "tiny-voca":
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
