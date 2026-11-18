# Home Assistant App: PdfCraft

Open-source pure-Rust PDF reader and editor ([`storytold/pdfcraft`](https://github.com/storytold/pdfcraft) `v0.21.1`) running via WebAssembly (`WebGL` + Web Worker ONNX OCR) with integrated Home Assistant server PDF storage (`/share/pdfcraft`) and local computer upload/download.

## Features

- **Full PdfCraft Engine in Browser**: Read, edit text & images, merge/split/reorder pages, fill forms, sign, redact, run built-in OCR, and verify PDF/A compliance.
- **Home Assistant Server PDF Library (`/share/pdfcraft`)**: Save directly to Home Assistant with `Ctrl+S` (**💾 存至 HA**), browse/open/rename/delete server PDFs from **📂 HA 伺服器 PDF 庫**, and optionally download to your local device at the same time.
- **Home Assistant Ingress & Direct Port (`8089/tcp`)**: Works seamlessly inside the Home Assistant sidebar on desktop and mobile/tablet browsers.
- **Supported Architectures**: `aarch64`, `amd64`.
