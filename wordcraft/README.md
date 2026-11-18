# Home Assistant App: WordCraft

Open-source pure-Rust word processor ([`storytold/wordcraft`](https://github.com/storytold/wordcraft) `v0.18.1`) running via WebAssembly (`WebGPU` / `WebGL2`) with integrated Home Assistant server document storage (`/share/wordcraft`) and local computer upload/download.

## Features

- **Full WordCraft Engine in Browser**: Ribbon interface, live Styles gallery, tables, Track Changes, comments, table of contents, citations, mail merge, and `.docx` / `.odt` / `.rtf` / `.pdf` / `.md` support.
- **Home Assistant Server Document Library (`/share/wordcraft`)**: Save directly to Home Assistant with `Ctrl+S` (**💾 存至 HA**), browse/open/rename/delete server documents from **📂 HA 伺服器文件庫**, and optionally download to your local device at the same time.
- **Home Assistant Ingress & Direct Port (`8088/tcp`)**: Works seamlessly inside the Home Assistant sidebar on desktop and mobile/tablet browsers.
- **Supported Architectures**: `aarch64`, `amd64`.
