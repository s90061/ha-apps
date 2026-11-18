# Changelog

## 0.1.0

- Initial Home Assistant App release packaging [`storytold/pdfcraft`](https://github.com/storytold/pdfcraft) `v0.21.1` (`aarch64` & `amd64`).
- Integrated PdfCraft's `data-host="page"` bridge and `window.pdfcraft` (`?control=1`) control API with a lightweight Python HTTP server and top toolbar (`📂 HA 伺服器 PDF 庫`, `💾 存至 HA`, `💻 開啟本機 PDF`, `⬇ 下載本機`, and `同步下載`).
- Pre-compressed WebAssembly (`.wasm.gz`), OCR worker, JS, and ONNX OCR models (`.onnx.gz`) for zero-CPU static serving over Home Assistant Ingress and direct port `8089/tcp`.
- Added configurable `storage_dir` (default `/share/pdfcraft`), `default_language` (default `zh-hant`), `author_name`, and `also_download_on_save` options with English and Traditional Chinese translations.
