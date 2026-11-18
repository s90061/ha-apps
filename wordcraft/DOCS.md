# Home Assistant App: WordCraft

WordCraft is a fast, open-source word processor built in pure Rust ([`storytold/wordcraft`](https://github.com/storytold/wordcraft)) with a familiar ribbon workflow, styles, tables, Track Changes, references, mail merge, and native `.docx` / `.odt` / `.rtf` / `.pdf` / `.md` support.

This Home Assistant App packages **WordCraft WebAssembly (`v0.18.1`)** together with a lightweight **Home Assistant Server Storage Bridge**:

- **Zero Server CPU / RAM Overhead**: All document layout and rendering (`WebGPU` / `WebGL2`) run directly inside your browser while the Raspberry Pi only serves pre-compressed assets and stores files.
- **Dual Storage Workflow (`/share/wordcraft` + Local Computer)**:
  - Pressing `Ctrl+S` (**File › Save** / **Export**) or clicking **💾 存至 HA** in the top toolbar automatically saves the document directly to `/share/wordcraft` on your Home Assistant server.
  - Clicking **📂 HA 伺服器文件庫** opens the server document manager to browse, open, upload, rename, download, or delete documents stored on Home Assistant across any device.
  - Clicking **💻 開啟本機檔** or **⬇ 下載本機** (or toggling **同步下載**) lets you open and download files directly to/from your local computer.

## Configuration Options

| Option | Default | Description |
| :--- | :--- | :--- |
| `tz` | `Asia/Taipei` | Container timezone. |
| `storage_dir` | `/share/wordcraft` | Directory on Home Assistant where documents are stored (must be under `/share`, `/config`, or `/media`). |
| `default_language` | `zh-hant` | Default WordCraft interface language applied on first launch (`zh-hant`, `zh-hans`, `en`, `ja`, `de`, `fr`, `es`, or `auto`). |
| `author_name` | `Home Assistant` | Default author name for comments and Track Changes. |
| `also_download_on_save` | `false` | When `true`, saving in WordCraft both saves to `storage_dir` on Home Assistant and triggers a browser download to your local computer. |
