# Home Assistant App: GridCraft

Open-source pure-Rust spreadsheet ([`storytold/gridcraft`](https://github.com/storytold/gridcraft) `v0.19.1`) running via WebAssembly (`WebGPU` / `WebGL2`) with integrated Home Assistant server workbook storage (`/share/gridcraft`) and local computer upload/download.

## Features

- **Full GridCraft Engine in Browser**: Ribbon interface, formula bar, 500+ worksheet functions (dynamic arrays, `LET`, `LAMBDA`, `XLOOKUP`), Pivot Tables, 17 chart types, conditional formatting, sparklines, tables, AutoFilter, Page Break Preview, and `.xlsx` / `.xlsm` / `.xls` / `.xlsb` / `.ods` / `.csv` / `.tsv` / `.pdf` support.
- **Home Assistant Server Workbook Library (`/share/gridcraft`)**: Save directly to Home Assistant with `Ctrl+S` (**💾 存至 HA**), browse/open/rename/delete server workbooks from **📂 HA 伺服器試算表庫**, and optionally download to your local device at the same time.
- **Home Assistant Ingress & Direct Port (`8090/tcp`)**: Works seamlessly inside the Home Assistant sidebar on desktop and mobile/tablet browsers.
- **Supported Architectures**: `aarch64`, `amd64`.
