# Changelog

## 0.1.0

- Initial Home Assistant App release packaging [`storytold/wordcraft`](https://github.com/storytold/wordcraft) `v0.18.1` (`aarch64` & `amd64`).
- Integrated WordCraft's `data-host="page"` bridge and `window.wordcraft` control API with a lightweight Python HTTP server and top toolbar (`📂 HA 伺服器文件庫`, `💾 存至 HA`, `💻 開啟本機檔`, `⬇ 下載本機`, and `同步下載`).
- Pre-compressed WebAssembly (`.wasm.gz`) and JS assets for zero-CPU static serving over Home Assistant Ingress and direct port `8088/tcp`.
- Added configurable `storage_dir` (default `/share/wordcraft`), `default_language` (default `zh-hant`), `author_name`, and `also_download_on_save` options with English and Traditional Chinese translations.
