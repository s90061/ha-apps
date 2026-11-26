"""Lightweight static WebAssembly + Home Assistant file bridge server for GridCraft."""

import json
import mimetypes
import os
import posixpath
import shutil
import tempfile
import urllib.parse
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

PORT = int(os.environ.get("PORT", "8090"))
WEB_DIR = Path(os.environ.get("WEB_DIR", "/app/web")).resolve()
STORAGE_DIR = Path(os.environ.get("STORAGE_DIR", "/share/gridcraft")).resolve()
DEFAULT_LANGUAGE = os.environ.get("DEFAULT_LANGUAGE", "zh-hant")
AUTHOR_NAME = os.environ.get("AUTHOR_NAME", "Home Assistant")
ALSO_DOWNLOAD_ON_SAVE = os.environ.get("ALSO_DOWNLOAD_ON_SAVE", "false").lower() == "true"
GRIDCRAFT_VERSION = os.environ.get("GRIDCRAFT_VERSION", "0.19.1")
MAX_FILE_BYTES = 512 * 1024 * 1024  # 512 MiB

STORAGE_DIR.mkdir(parents=True, exist_ok=True)

MIME_MAP = {
    ".wasm": "application/wasm",
    ".js": "text/javascript; charset=utf-8",
    ".mjs": "text/javascript; charset=utf-8",
    ".html": "text/html; charset=utf-8",
    ".css": "text/css; charset=utf-8",
    ".json": "application/json; charset=utf-8",
    ".ico": "image/x-icon",
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".svg": "image/svg+xml",
    ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    ".xlsm": "application/vnd.ms-excel.sheet.macroEnabled.12",
    ".xls": "application/vnd.ms-excel",
    ".xlsb": "application/vnd.ms-excel.sheet.binary.macroEnabled.12",
    ".ods": "application/vnd.oasis.opendocument.spreadsheet",
    ".csv": "text/csv; charset=utf-8",
    ".tsv": "text/tab-separated-values; charset=utf-8",
    ".slk": "text/plain; charset=utf-8",
    ".pdf": "application/pdf",
    ".txt": "text/plain; charset=utf-8",
}


def build_config_payload() -> dict[str, object]:
    return {
        "storage_dir": str(STORAGE_DIR),
        "default_language": DEFAULT_LANGUAGE,
        "author_name": AUTHOR_NAME,
        "also_download_on_save": ALSO_DOWNLOAD_ON_SAVE,
        "app_version": GRIDCRAFT_VERSION,
    }


def sanitize_filename(raw_name: str) -> str:
    """Validate and clean a single filename inside STORAGE_DIR."""
    name = urllib.parse.unquote(raw_name or "").strip()
    name = os.path.basename(name.replace("\\", "/")).strip()
    if not name or name in (".", "..") or "/" in name or "\\" in name or "\x00" in name:
        raise ValueError("無效的檔案名稱")
    if len(name) > 255:
        name = name[:255]
    target = (STORAGE_DIR / name).resolve()
    if target.parent != STORAGE_DIR:
        raise ValueError("不允許跨目錄存取")
    return name


class GridCraftHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt: str, *args) -> None:
        # Suppress noisy static asset logs while keeping error messages
        if len(args) >= 2 and str(args[1]).startswith(("4", "5")):
            super().log_message(fmt, *args)

    def send_json(self, status: int, payload: object) -> None:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:
        parsed = urllib.parse.urlsplit(self.path)
        path = parsed.path

        if path == "/api/health":
            self.send_json(
                HTTPStatus.OK,
                {
                    "status": "ok",
                    "version": GRIDCRAFT_VERSION,
                    "storage_dir": str(STORAGE_DIR),
                },
            )
            return

        if path == "/api/config":
            self.send_json(HTTPStatus.OK, build_config_payload())
            return

        if path == "/api/config.js":
            serialized = json.dumps(build_config_payload(), ensure_ascii=False).replace("</", "<\\/")
            body = f"window.__HA_GRIDCRAFT_CONFIG__ = {serialized};\n".encode("utf-8")
            self.send_response(HTTPStatus.OK)
            self.send_header("Content-Type", "text/javascript; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Cache-Control", "no-store")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.end_headers()
            self.wfile.write(body)
            return

        if path == "/api/files":
            items = []
            try:
                for entry in os.scandir(STORAGE_DIR):
                    if not entry.is_file() or entry.name.startswith("."):
                        continue
                    st = entry.stat()
                    ext = entry.name.rsplit(".", 1)[-1].lower() if "." in entry.name else ""
                    items.append(
                        {
                            "name": entry.name,
                            "size": st.st_size,
                            "mtime": int(st.st_mtime),
                            "ext": ext,
                        }
                    )
                items.sort(key=lambda x: x["mtime"], reverse=True)
                self.send_json(HTTPStatus.OK, {"files": items, "storage_dir": str(STORAGE_DIR)})
            except Exception as exc:
                self.send_json(HTTPStatus.INTERNAL_SERVER_ERROR, {"error": str(exc)})
            return

        if path.startswith("/api/files/"):
            raw_name = path[len("/api/files/") :]
            try:
                fname = sanitize_filename(raw_name)
            except ValueError as exc:
                self.send_json(HTTPStatus.BAD_REQUEST, {"error": str(exc)})
                return

            fpath = STORAGE_DIR / fname
            if not fpath.is_file():
                self.send_json(HTTPStatus.NOT_FOUND, {"error": "找不到該檔案"})
                return

            try:
                data = fpath.read_bytes()
            except Exception as exc:
                self.send_json(HTTPStatus.INTERNAL_SERVER_ERROR, {"error": str(exc)})
                return

            ext = fpath.suffix.lower()
            ctype = MIME_MAP.get(ext) or mimetypes.guess_type(fname)[0] or "application/octet-stream"
            query = urllib.parse.parse_qs(parsed.query)
            is_download = query.get("download", ["0"])[0] == "1"

            self.send_response(HTTPStatus.OK)
            self.send_header("Content-Type", ctype)
            self.send_header("Content-Length", str(len(data)))
            self.send_header("Cache-Control", "no-store")
            if is_download:
                quoted = urllib.parse.quote(fname)
                self.send_header(
                    "Content-Disposition",
                    f"attachment; filename=\"{quoted}\"; filename*=UTF-8''{quoted}",
                )
            self.end_headers()
            self.wfile.write(data)
            return

        self.serve_static(path)

    def do_PUT(self) -> None:
        self.handle_file_upload()

    def do_POST(self) -> None:
        parsed = urllib.parse.urlsplit(self.path)
        path = parsed.path

        if path.startswith("/api/files/") and path.endswith("/rename"):
            raw_name = path[len("/api/files/") : -len("/rename")]
            try:
                old_name = sanitize_filename(raw_name)
                length = int(self.headers.get("Content-Length", "0"))
                body = json.loads(self.rfile.read(length).decode("utf-8"))
                new_name = sanitize_filename(body.get("new_name", ""))
            except Exception as exc:
                self.send_json(HTTPStatus.BAD_REQUEST, {"error": str(exc)})
                return

            src = STORAGE_DIR / old_name
            dst = STORAGE_DIR / new_name
            if not src.is_file():
                self.send_json(HTTPStatus.NOT_FOUND, {"error": "找不到原始檔案"})
                return
            if dst.exists() and src != dst:
                self.send_json(HTTPStatus.CONFLICT, {"error": "目標檔名已存在"})
                return

            try:
                src.rename(dst)
                st = dst.stat()
                self.send_json(
                    HTTPStatus.OK,
                    {"ok": True, "name": new_name, "size": st.st_size, "mtime": int(st.st_mtime)},
                )
            except Exception as exc:
                self.send_json(HTTPStatus.INTERNAL_SERVER_ERROR, {"error": str(exc)})
            return

        self.handle_file_upload()

    def do_DELETE(self) -> None:
        parsed = urllib.parse.urlsplit(self.path)
        path = parsed.path
        if not path.startswith("/api/files/"):
            self.send_json(HTTPStatus.NOT_FOUND, {"error": "不支援的路徑"})
            return

        raw_name = path[len("/api/files/") :]
        try:
            fname = sanitize_filename(raw_name)
        except ValueError as exc:
            self.send_json(HTTPStatus.BAD_REQUEST, {"error": str(exc)})
            return

        fpath = STORAGE_DIR / fname
        if not fpath.is_file():
            self.send_json(HTTPStatus.NOT_FOUND, {"error": "找不到該檔案"})
            return

        try:
            fpath.unlink()
            self.send_json(HTTPStatus.OK, {"ok": True, "deleted": fname})
        except Exception as exc:
            self.send_json(HTTPStatus.INTERNAL_SERVER_ERROR, {"error": str(exc)})

    def handle_file_upload(self) -> None:
        parsed = urllib.parse.urlsplit(self.path)
        path = parsed.path
        if not path.startswith("/api/files/"):
            self.send_json(HTTPStatus.NOT_FOUND, {"error": "不支援的路徑"})
            return

        raw_name = path[len("/api/files/") :]
        try:
            fname = sanitize_filename(raw_name)
        except ValueError as exc:
            self.send_json(HTTPStatus.BAD_REQUEST, {"error": str(exc)})
            return

        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            self.send_json(HTTPStatus.BAD_REQUEST, {"error": "無效的 Content-Length"})
            return

        if length < 0 or length > MAX_FILE_BYTES:
            self.send_json(HTTPStatus.REQUEST_ENTITY_TOO_LARGE, {"error": "檔案大小超過 512 MiB 上限"})
            return

        dest = STORAGE_DIR / fname
        fd, tmp_path = tempfile.mkstemp(prefix=".gc_tmp_", dir=str(STORAGE_DIR))
        try:
            remaining = length
            with os.fdopen(fd, "wb") as f:
                while remaining > 0:
                    chunk = self.rfile.read(min(65536, remaining))
                    if not chunk:
                        break
                    f.write(chunk)
                    remaining -= len(chunk)
                f.flush()
                os.fsync(f.fileno())
            os.replace(tmp_path, dest)
            st = dest.stat()
            self.send_json(
                HTTPStatus.OK,
                {
                    "ok": True,
                    "name": fname,
                    "size": st.st_size,
                    "mtime": int(st.st_mtime),
                    "storage_dir": str(STORAGE_DIR),
                },
            )
        except Exception as exc:
            try:
                if os.path.exists(tmp_path):
                    os.unlink(tmp_path)
            except OSError:
                pass
            self.send_json(HTTPStatus.INTERNAL_SERVER_ERROR, {"error": str(exc)})

    def serve_static(self, raw_path: str) -> None:
        norm = posixpath.normpath(urllib.parse.unquote(raw_path)).lstrip("/")
        if not norm or norm == ".":
            norm = "index.html"

        target = (WEB_DIR / norm).resolve()
        if not target.is_relative_to(WEB_DIR) or not target.is_file():
            self.send_json(HTTPStatus.NOT_FOUND, {"error": "Not found"})
            return

        ext = target.suffix.lower()
        ctype = MIME_MAP.get(ext) or mimetypes.guess_type(target.name)[0] or "application/octet-stream"
        accept_enc = self.headers.get("Accept-Encoding", "")

        use_gzip = False
        serve_path = target
        gz_candidate = Path(str(target) + ".gz")
        if "gzip" in accept_enc and gz_candidate.is_file():
            use_gzip = True
            serve_path = gz_candidate

        st = serve_path.stat()
        self.send_response(HTTPStatus.OK)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(st.st_size))
        self.send_header("X-Content-Type-Options", "nosniff")
        if use_gzip:
            self.send_header("Content-Encoding", "gzip")
            self.send_header("Vary", "Accept-Encoding")

        if ext in (".wasm", ".js") and "ha-bridge" not in target.name:
            self.send_header("Cache-Control", "public, max-age=31536000, immutable")
        else:
            self.send_header("Cache-Control", "no-cache")

        self.end_headers()
        with serve_path.open("rb") as f:
            shutil.copyfileobj(f, self.wfile, length=65536)


def main() -> None:
    server = ThreadingHTTPServer(("0.0.0.0", PORT), GridCraftHandler)
    print(f"[gridcraft-server] Listening on 0.0.0.0:{PORT} (WEB_DIR={WEB_DIR}, STORAGE_DIR={STORAGE_DIR})", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
