# S90061 Home Assistant Apps

[![Open your Home Assistant instance and show the app store with a specific repository URL pre-filled.](https://my.home-assistant.io/badges/supervisor_store.svg)](https://my.home-assistant.io/redirect/supervisor_store/?repository_url=https%3A%2F%2Fgithub.com%2Fs90061%2Fha-apps)

[繁體中文](#繁體中文說明) | [English](#english)

---

## 繁體中文說明

這是一個符合 Home Assistant Supervisor 規範的公開多 App 儲存庫（Public Repository），支援讓 Home Assistant 訂閱、安裝預先建置好的多架構（`amd64` / `aarch64`）容器映像檔，並自動檢查版本更新。

### 安裝與訂閱步驟

1. 點擊上方 **Open your Home Assistant instance...** 按鈕一鍵匯入；或在 Home Assistant 中前往 **設定 > 應用程式（Add-ons）> 應用程式商店 > 右上角選單 ⋮ > 儲存庫（Repositories）**。
2. 加入以下儲存庫網址並按下 **新增**：
   `https://github.com/s90061/ha-apps`
3. 在商店清單找到 **S90061 Home Assistant Apps**，即可安裝與啟用各個 App；日後 `main` 分支升版時，Home Assistant Supervisor 會自動檢查並提示更新。

### 目前收錄的 Apps

#### [Hello World](./hello_world)

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

最小可運作的範本 App（使用 `ghcr.io/home-assistant/base` 與 `s6-overlay v3`），可透過 UI 設定訊息與週期輸出至日誌，用來驗證容器啟動、中英文設定介面與自動更新流程。

#### [Webtop (Ubuntu XFCE)](./webtop_ubuntu_xfce)

![Supports aarch64 Architecture][aarch64-shield]

封裝 `linuxserver/webtop:ubuntu-xfce`（嚴格釘選 `aarch64` digest `sha256:12b00cce8f60ee8f4553f2726d35c5d48d34d6c87513b1044a20619a0a2e3c3b`），支援 Home Assistant Ingress 側邊欄與 HTTPS `3001` 直連桌面，並自動整合 `/config`（`app_config:rw`）、`/share`、UI 設定欄位與 Chromium `--disable-dev-shm-usage`。

#### [aMule](./amule)

![Supports aarch64 Architecture][aarch64-shield]

封裝 `ngosang/amule`（`3.2.0-1`，嚴格釘選 multi-arch OCI digest `sha256:4b1a4246d57e7994b31c723b8c55474654e328c1b8eb0f9deb7f1843673bd515`，支援 `aarch64`），透過內建 Nginx Proxy 完整支援 Home Assistant Ingress 側邊欄登入與 SSE 即時更新，整合 `/config`（`app_config:rw`）、`/share` 與 `/media`，並預設開啟 ED2K / Kademlia P2P 連接埠（`4662`、`4665`、`4672`）。

#### [PiStream](./pistream)

![Supports aarch64 Architecture][aarch64-shield]

專為 Raspberry Pi 5（`aarch64`）打造的輕量遠端影音伺服器，支援智慧三級串流（HTTP 206 原生直連、`-c:v copy` 免重編碼極速換殼、滑動視窗 HLS 省流轉碼）、外掛與內嵌字幕快取、多使用者獨立觀看進度，並同時支援 Home Assistant Ingress 側邊欄與 `8080` 直連埠（可直接讀取 `/media` 或 `/share/amule/incoming`）。

#### [WordCraft](./wordcraft)

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

封裝純 Rust 開源文書處理器 [`storytold/wordcraft`](https://github.com/storytold/wordcraft)（`v0.18.1` WebAssembly + WebGPU/WebGL2），提供完整 Ribbon 介面、`.docx` / `.odt` / `.rtf` / `.pdf` / `.md` 讀寫、追蹤修訂與目錄索引；所有渲染皆於瀏覽器端完成（樹莓派 0% CPU 負載），並整合 **HA 伺服器雲端文件庫（`/share/wordcraft`）** 與本機電腦上下載雙模式。

#### [PdfCraft](./pdfcraft)

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

封裝純 Rust 開源 PDF 閱讀與編輯器 [`storytold/pdfcraft`](https://github.com/storytold/pdfcraft)（`v0.21.1` WebAssembly + WebGL + 瀏覽器端 ONNX OCR），支援頁面重組、文字與圖片編輯、表單填寫、簽章、遮蔽（Redaction）、內建 OCR 文字辨識與 PDF/A 驗證；所有運算皆於瀏覽器端完成（樹莓派 0% CPU 負載），並整合 **HA 伺服器雲端 PDF 庫（`/share/pdfcraft`）** 與本機電腦上下載雙模式。

#### [GridCraft](./gridcraft)

![Supports aarch64 Architecture][aarch64-shield]
![Supports amd64 Architecture][amd64-shield]

封裝純 Rust 開源試算表 [`storytold/gridcraft`](https://github.com/storytold/gridcraft)（`v0.19.1` WebAssembly + WebGPU/WebGL2），提供完整 Ribbon 介面、500+ 公式函數（動態陣列、`LET`、`LAMBDA`、`XLOOKUP`）、樞紐分析表、17 種圖表、條件格式與 `.xlsx` / `.csv` / `.tsv` / `.xls` / `.xlsb` / `.ods` / `.pdf` 支援；所有運算與渲染皆於瀏覽器端完成（樹莓派 0% CPU 負載），並整合 **HA 伺服器雲端試算表庫（`/share/gridcraft`）** 與本機電腦上下載雙模式。

### 開發與維護規範（防呆說明）

* **`main` 分支僅作穩定發布**：請在開發分支修改與測試後再開 PR 合併至 `main`。合併時若要觸發 Home Assistant 更新與 GHCR 重新建置，請務必同步更新 `<app>/config.yaml` 的 `version` 字串（例如 `"0.1.1"`）與 `CHANGELOG.md`。
* **強制 LF 換行與執行權限**：本儲存庫透過 `.gitattributes`（`* text=auto eol=lf`）、`.editorconfig`、`.githooks/pre-commit` 及 GitHub Actions CI 四重防護，確保所有腳本與無副檔名的 `s6-overlay` 服務檔案維持 LF 換行及 `100755` 執行權限。
* **本機開發初始化（Windows 必看）**：clone 本儲存庫後建議執行一次：
  ```bash
  git config --local core.autocrlf false
  git config --local core.eol lf
  git config --local core.hooksPath .githooks
  ```
* **首次 GHCR 設定提醒**：每個 App 首次推送映像檔至 GitHub Container Registry（`ghcr.io/s90061/ha-apps-<slug>`）時預設為 Private，需在 GitHub Package 設定中將其 Visibility 切換為 **Public**，Home Assistant 主機才能直接拉取。

---

## English

A public multi-app repository compliant with Home Assistant Supervisor standards, providing pre-built multi-architecture (`amd64` and `aarch64`) container images via GHCR with automatic update detection.

### Installation

1. Click the **Open your Home Assistant instance...** badge above, or in Home Assistant go to **Settings > Apps (Add-ons) > App Store > ⋮ > Repositories**.
2. Add `https://github.com/s90061/ha-apps`.
3. Install any app from **S90061 Home Assistant Apps**. Supervisor polls `main` automatically and notifies you whenever a newer `version` is published.

### Included Apps

* **[Hello World](./hello_world)** — Minimal starter app using Home Assistant Alpine Base + `s6-overlay v3` + `bashio`, with localized English & Traditional Chinese configuration UI.
* **[Webtop (Ubuntu XFCE)](./webtop_ubuntu_xfce)** — Ubuntu XFCE browser desktop (`linuxserver/webtop:ubuntu-xfce`, pinned `aarch64` digest) with HA Ingress, optional HTTPS port `3001`, persistent `app_config` (`/config`), and localized options.
* **[aMule](./amule)** — eMule / ED2K and Kademlia P2P client (`ngosang/amule:3.2.0-1`, `aarch64`) with HA Ingress cookie-rewriting proxy for `amuleapi` Web UI, persistent `/config`, `/share` and `/media` storage, and P2P port mappings.
* **[PiStream](./pistream)** — Lightweight Raspberry Pi 5 media server (`aarch64`) with HTTP 206 Direct Play, zero-copy Remux, sliding-window HLS transcoding, HA Ingress + direct port `8080`, and read-only `/media` & `/share` integration.
* **[WordCraft](./wordcraft)** — Pure-Rust word processor (`storytold/wordcraft` `v0.18.1`, `aarch64` & `amd64`) running via WebAssembly (`WebGPU` / `WebGL2`) with integrated Home Assistant server document library (`/share/wordcraft`) and local computer upload/download.
* **[PdfCraft](./pdfcraft)** — Pure-Rust PDF reader, editor, and ONNX OCR tool (`storytold/pdfcraft` `v0.21.1`, `aarch64` & `amd64`) running via WebAssembly (`WebGL`) with integrated Home Assistant server PDF library (`/share/pdfcraft`) and local computer upload/download.
* **[GridCraft](./gridcraft)** — Pure-Rust spreadsheet (`storytold/gridcraft` `v0.19.1`, `aarch64` & `amd64`) running via WebAssembly (`WebGPU` / `WebGL2`) with integrated Home Assistant server workbook library (`/share/gridcraft`) and local computer upload/download.

### Adding a New App

1. Create a directory `<slug>/` whose name matches the `slug` field in `<slug>/config.yaml`.
2. Set `image: "ghcr.io/s90061/ha-apps-<slug>"` and `arch: [aarch64, amd64]` in `config.yaml` (avoid repeating schema default values so `action-app-linter` passes).
3. Ensure every script under `rootfs/` uses Unix LF line endings and executable permissions (`git update-index --chmod=+x <file>`).
4. Open a pull request targeting `main`; CI will run CRLF checks, permission checks, ShellCheck, yamllint, app linting, and dry-run multi-arch builds before merge.

[aarch64-shield]: https://img.shields.io/badge/aarch64-yes-green.svg
[amd64-shield]: https://img.shields.io/badge/amd64-yes-green.svg
