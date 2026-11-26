# Home Assistant App: GridCraft

GridCraft is a fast, open-source spreadsheet built in pure Rust ([`storytold/gridcraft`](https://github.com/storytold/gridcraft)) with a familiar ribbon workflow, 500+ worksheet functions, dynamic arrays, Pivot Tables, 17 chart types, conditional formatting, sparklines, and native `.xlsx` / `.csv` / `.tsv` / `.xls` / `.xlsb` / `.ods` / `.pdf` support.

This Home Assistant App packages **GridCraft WebAssembly (`v0.19.1`)** together with a lightweight **Home Assistant Server Storage Bridge**:

- **Zero Server CPU / RAM Overhead**: All spreadsheet calculation and rendering (`WebGPU` / `WebGL2`) run directly inside your browser while the Home Assistant host only serves pre-compressed assets and stores files.
- **Taiwan Traditional Chinese Excel Localization & Embedded Font**: Built with a complete Taiwan Traditional Chinese (`zh-hant` / `zh-tw`) translation catalog aligned with Microsoft Excel Taiwan terminology (常用、版面配置、資料、校閱、檢視、儲存格、列/欄、樞紐分析表、跨欄置中、自動換列、資料剖析、設定格式化的條件等) and embedded **Noto Sans TC Regular** (思源黑體台灣繁體版) so both the UI and cell text render crisply in WebAssembly.
- **Dual Storage Workflow (`/share/gridcraft` + Local Computer)**:
  - Pressing `Ctrl+S` (**檔案 › 儲存檔案** / **匯出**) or clicking **💾 存至 HA** in the top toolbar automatically saves the workbook directly to `/share/gridcraft` on your Home Assistant server.
  - Clicking **📂 HA 伺服器試算表庫** opens the server workbook manager to browse, open, upload, rename, download, or delete spreadsheets stored on Home Assistant across any device.
  - Clicking **💻 開啟本機檔** or **⬇ 下載本機** (or toggling **同步下載**) lets you open and download files directly to/from your local computer.

## Configuration Options

| Option | Default | Description |
| :--- | :--- | :--- |
| `tz` | `Asia/Taipei` | Container timezone. |
| `storage_dir` | `/share/gridcraft` | Directory on Home Assistant where workbooks are stored (must be under `/share`, `/config`, or `/media`). |
| `default_language` | `zh-hant` | Default GridCraft interface language applied on first launch (`zh-hant` / `zh-tw` for Taiwan Traditional Chinese Excel UI, `zh`, `en`, `ja`, etc., or `auto`). |
| `author_name` | `Home Assistant` | Default author name for new workbooks, notes, and comments. |
| `also_download_on_save` | `false` | When `true`, saving in GridCraft both saves to `storage_dir` on Home Assistant and triggers a browser download to your local computer. |
