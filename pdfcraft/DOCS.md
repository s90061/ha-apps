# Home Assistant App: PdfCraft

PdfCraft is a fast, open-source PDF reader and editor built from scratch in pure Rust ([`storytold/pdfcraft`](https://github.com/storytold/pdfcraft)) supporting page organization, text & image editing, annotations, AcroForm filling, digital signatures, redaction, PDF/A verification, and local browser-based ONNX OCR.

This Home Assistant App packages **PdfCraft WebAssembly (`v0.21.1`)** together with a lightweight **Home Assistant Server Storage Bridge**:

- **Zero Server CPU / RAM Overhead**: All PDF rendering (`WebGL`), editing, and OCR worker execution run directly inside your browser while the Raspberry Pi only serves pre-compressed assets and stores PDF files.
- **Dual Storage Workflow (`/share/pdfcraft` + Local Computer)**:
  - Pressing `Ctrl+S` (**File › Save**) or clicking **💾 存至 HA** in the top toolbar automatically saves the PDF directly to `/share/pdfcraft` on your Home Assistant server.
  - Clicking **📂 HA 伺服器 PDF 庫** opens the server document manager to browse, open, upload, rename, download, or delete PDFs stored on Home Assistant across any device.
  - Clicking **💻 開啟本機 PDF** or **⬇ 下載本機** (or toggling **同步下載**) lets you open and download files directly to/from your local computer.

## Configuration Options

| Option | Default | Description |
| :--- | :--- | :--- |
| `tz` | `Asia/Taipei` | Container timezone. |
| `storage_dir` | `/share/pdfcraft` | Directory on Home Assistant where PDF files are stored (must be under `/share`, `/config`, or `/media`). |
| `default_language` | `zh-hant` | Default PdfCraft interface language applied on first launch (`zh-hant`, `zh-hans`, `en`, `ja`, `de`, `fr`, `es`, or `auto`). |
| `author_name` | `Home Assistant` | Default author name for PDF comments and annotations. |
| `also_download_on_save` | `false` | When `true`, saving in PdfCraft both saves to `storage_dir` on Home Assistant and triggers a browser download to your local computer. |
