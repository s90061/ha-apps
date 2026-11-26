# Changelog

## 0.1.0

- Initial Home Assistant App release packaging [`storytold/gridcraft`](https://github.com/storytold/gridcraft) `v0.19.1` (`aarch64` & `amd64`) built with Taiwan Traditional Chinese (`zh-tw` / `zh-hant`, Microsoft Excel Taiwan terminology) and embedded **Noto Sans TC Regular** font.
- Integrated GridCraft's `data-host="page"` bridge and `window.gridcraft` control API with a lightweight Python HTTP server and top toolbar (`📂 HA 伺服器試算表庫`, `💾 存至 HA`, `💻 開啟本機檔`, `⬇ 下載本機`, and `同步下載`).
- Pre-compressed WebAssembly (`.wasm.gz`) and JS assets for zero-CPU static serving over Home Assistant Ingress and direct port `8090/tcp`.
- Added configurable `storage_dir` (default `/share/gridcraft`), `default_language` (default `zh-hant`), `author_name`, and `also_download_on_save` options with English and Traditional Chinese translations.
