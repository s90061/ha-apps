(function () {
  "use strict";

  const canvas = document.getElementById("gridcraft_canvas");
  if (!canvas) return;

  // Apply ?author= synchronously from /api/config.js BEFORE wasm init() runs
  const bootConfig = window.__HA_GRIDCRAFT_CONFIG__ || null;
  if (bootConfig && bootConfig.author_name) {
    try {
      const curUrl = new URL(window.location.href);
      if (!curUrl.searchParams.has("author")) {
        curUrl.searchParams.set("author", bootConfig.author_name);
        window.history.replaceState(null, "", curUrl.toString());
      }
    } catch {
      // ignore URL replaceState errors
    }
  }

  // Enable GridCraft same-document host bridge so all Save / Export actions flow through gridcraft:save
  canvas.setAttribute("data-host", "page");
  document.body.classList.add("ha-bar-open");

  let currentDocName = "Book1.xlsx";
  let isDirty = false;
  let storageDir = (bootConfig && bootConfig.storage_dir) || "/share/gridcraft";
  let alsoDownload =
    localStorage.getItem("ha_gc_also_download") !== null
      ? localStorage.getItem("ha_gc_also_download") === "true"
      : Boolean(bootConfig && bootConfig.also_download_on_save);
  let pendingLocalDownload = false;
  let lastSavedBuffer = null;
  let toastTimer = null;

  // Build top control bar, expand pill, modal, and toast
  const topbar = document.createElement("div");
  topbar.id = "ha_topbar";
  topbar.innerHTML = `
    <div class="ha-bar-left">
      <span class="ha-brand">GridCraft</span>
      <span id="ha_doc_name" class="ha-doc-pill" title="目前開啟的試算表名稱">Book1.xlsx</span>
      <span id="ha_dirty_badge" class="ha-status-badge ha-status-synced">✓ 已就緒</span>
      <span id="ha_path_badge" class="ha-path-badge"></span>
    </div>
    <div class="ha-bar-right">
      <button id="ha_btn_server_files" class="ha-btn ha-btn-primary" title="瀏覽、開啟或管理 Home Assistant 伺服器上的試算表">
        📂 HA 伺服器試算表庫 (<span id="ha_file_count">0</span>)
      </button>
      <button id="ha_btn_open_local" class="ha-btn" title="從本機電腦選取試算表載入至 GridCraft">
        💻 開啟本機檔
      </button>
      <button id="ha_btn_save_ha" class="ha-btn ha-btn-emerald" title="立即將目前試算表儲存至 Home Assistant 伺服器 (Ctrl+S)">
        💾 存至 HA
      </button>
      <button id="ha_btn_download_local" class="ha-btn" title="儲存並下載目前試算表至本機電腦">
        ⬇ 下載本機
      </button>
      <label class="ha-toggle-label" title="勾選後，每次在 GridCraft 按下 Ctrl+S 或匯出時，除了存入 HA 伺服器也會同步下載至本機電腦">
        <input type="checkbox" id="ha_chk_also_dl" />
        同步下載
      </label>
      <button id="ha_btn_collapse" class="ha-btn" title="收合上方工具列以獲得 100% 全螢幕">
        ▲
      </button>
    </div>
  `;
  document.body.appendChild(topbar);

  const expandPill = document.createElement("button");
  expandPill.id = "ha_expand_pill";
  expandPill.title = "展開 HA 雲端試算表庫工具列";
  expandPill.innerHTML = "☁ HA 試算表庫 ▼";
  document.body.appendChild(expandPill);

  const modalBackdrop = document.createElement("div");
  modalBackdrop.id = "ha_modal_backdrop";
  modalBackdrop.innerHTML = `
    <div class="ha-modal" role="dialog" aria-modal="true">
      <div class="ha-modal-header">
        <div>
          <h3 class="ha-modal-title">📂 Home Assistant 伺服器試算表庫</h3>
          <div id="ha_modal_storage_path" class="ha-modal-sub"></div>
        </div>
        <button id="ha_modal_close" class="ha-btn">✕ 關閉</button>
      </div>
      <div class="ha-modal-toolbar">
        <div style="display:flex;gap:6px;flex-wrap:wrap;">
          <button id="ha_modal_upload_btn" class="ha-btn ha-btn-primary">📤 上傳本機檔案至 HA</button>
          <button id="ha_modal_save_current_btn" class="ha-btn ha-btn-emerald">💾 將目前編輯試算表存入 HA</button>
        </div>
        <button id="ha_modal_refresh_btn" class="ha-btn">↻ 重新整理</button>
      </div>
      <div class="ha-modal-body">
        <ul id="ha_file_list" class="ha-file-list"></ul>
      </div>
    </div>
  `;
  document.body.appendChild(modalBackdrop);

  const toastEl = document.createElement("div");
  toastEl.id = "ha_toast";
  document.body.appendChild(toastEl);

  const hiddenLocalOpenInput = document.createElement("input");
  hiddenLocalOpenInput.type = "file";
  hiddenLocalOpenInput.style.display = "none";
  hiddenLocalOpenInput.accept = ".xlsx,.xlsm,.xls,.xlsb,.ods,.csv,.tsv,.txt,.slk,.json";
  document.body.appendChild(hiddenLocalOpenInput);

  const hiddenServerUploadInput = document.createElement("input");
  hiddenServerUploadInput.type = "file";
  hiddenServerUploadInput.multiple = true;
  hiddenServerUploadInput.style.display = "none";
  hiddenServerUploadInput.accept = ".xlsx,.xlsm,.xls,.xlsb,.ods,.csv,.tsv,.txt,.slk,.json";
  document.body.appendChild(hiddenServerUploadInput);

  const docNameEl = document.getElementById("ha_doc_name");
  const dirtyBadgeEl = document.getElementById("ha_dirty_badge");
  const pathBadgeEl = document.getElementById("ha_path_badge");
  const fileCountEl = document.getElementById("ha_file_count");
  const chkAlsoDl = document.getElementById("ha_chk_also_dl");
  const fileListEl = document.getElementById("ha_file_list");
  const modalPathEl = document.getElementById("ha_modal_storage_path");

  pathBadgeEl.textContent = storageDir;
  modalPathEl.textContent = storageDir;
  chkAlsoDl.checked = alsoDownload;

  function showToast(msg) {
    toastEl.textContent = msg;
    toastEl.classList.add("show");
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      toastEl.classList.remove("show");
    }, 3200);
  }

  function updateStatusBadge(state, text) {
    dirtyBadgeEl.className = "ha-status-badge " + state;
    dirtyBadgeEl.textContent = text;
  }

  function setDocName(name) {
    if (!name) return;
    currentDocName = name;
    docNameEl.textContent = name;
    docNameEl.title = name;
  }

  function formatBytes(bytes) {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / (1024 * 1024)).toFixed(2) + " MB";
  }

  function formatTime(epochSec) {
    if (!epochSec) return "";
    const d = new Date(epochSec * 1000);
    return d.toLocaleString();
  }

  function waitForGridCraftApi(timeoutMs = 2000) {
    return new Promise((resolve) => {
      const start = Date.now();
      function check() {
        if (window.gridcraft && typeof window.gridcraft.execute === "function") {
          resolve(window.gridcraft);
          return;
        }
        if (Date.now() - start >= timeoutMs) {
          resolve(null);
          return;
        }
        setTimeout(check, 50);
      }
      check();
    });
  }

  function triggerBrowserDownload(name, arrayBuffer) {
    const blob = new Blob([arrayBuffer], { type: "application/octet-stream" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = name || "Book1.xlsx";
    a.style.display = "none";
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10000);
  }

  async function saveBytesToServer(name, arrayBuffer) {
    updateStatusBadge("ha-status-saving", "⟳ 儲存至 HA 中...");
    const cleanName = (name || currentDocName || "Book1.xlsx").trim();
    const res = await fetch(`./api/files/${encodeURIComponent(cleanName)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/octet-stream" },
      body: arrayBuffer,
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      updateStatusBadge("ha-status-dirty", "⚠ 儲存失敗");
      throw new Error(data.error || `HTTP ${res.status}`);
    }
    updateStatusBadge("ha-status-synced", "✓ 已存至 HA");
    showToast(`✓ 已儲存至 HA: ${storageDir}/${cleanName} (${formatBytes(data.size || arrayBuffer.byteLength)})`);
    refreshServerFileList(false);
    return data;
  }

  // 1. Register GridCraft Host Events BEFORE wasm init resolves
  canvas.addEventListener("gridcraft:ready", async () => {
    try {
      const cfgRes = await fetch("./api/config");
      if (cfgRes.ok) {
        const cfg = await cfgRes.json();
        if (cfg.storage_dir) {
          storageDir = cfg.storage_dir;
          pathBadgeEl.textContent = storageDir;
          modalPathEl.textContent = storageDir;
        }
        if (localStorage.getItem("ha_gc_also_download") === null && typeof cfg.also_download_on_save === "boolean") {
          alsoDownload = cfg.also_download_on_save;
          chkAlsoDl.checked = alsoDownload;
        }
        // GridCraft attaches window.gridcraft right after bridge.post_ready(), so wait briefly.
        const api = await waitForGridCraftApi();
        if (api) {
          if (cfg.default_language && cfg.default_language !== "auto" && !localStorage.getItem("ha_gc_lang_init")) {
            api
              .execute("app.language.set", { language: cfg.default_language })
              .then(() => localStorage.setItem("ha_gc_lang_init", "1"))
              .catch(() => {});
          }
        }
      }
    } catch {
      // ignore config fetch error
    }
    refreshServerFileList(false);
  });

  canvas.addEventListener("gridcraft:save", async (event) => {
    const detail = event.detail || {};
    const name = detail.name || currentDocName || "Book1.xlsx";
    const saveAs = Boolean(detail.saveAs);
    const bytes = detail.bytes;
    if (!bytes) return;

    lastSavedBuffer = { name, bytes: bytes.slice ? bytes.slice(0) : bytes };
    if (saveAs || /\.(xlsx|xlsm|csv|tsv|json|html)$/i.test(name)) {
      setDocName(name);
    }

    const shouldDownloadLocal = alsoDownload || pendingLocalDownload;
    pendingLocalDownload = false;

    try {
      await saveBytesToServer(name, lastSavedBuffer.bytes);
      isDirty = false;
    } catch (err) {
      showToast(`❌ 儲存至 HA 失敗: ${err.message || err}`);
    }

    if (shouldDownloadLocal) {
      triggerBrowserDownload(name, lastSavedBuffer.bytes);
    }
  });

  canvas.addEventListener("gridcraft:dirty", (event) => {
    isDirty = Boolean(event.detail && event.detail.dirty);
    if (isDirty) {
      updateStatusBadge("ha-status-dirty", "● 未儲存變更");
    } else {
      updateStatusBadge("ha-status-synced", "✓ 已同步");
    }
  });

  canvas.addEventListener("gridcraft:failed", (event) => {
    const err = (event.detail && event.detail.error) || "WebGPU/WebGL2 初始化失敗";
    showToast(`❌ GridCraft 啟動失敗: ${err}`);
  });

  window.addEventListener("beforeunload", (e) => {
    if (isDirty) {
      e.preventDefault();
      e.returnValue = "";
    }
  });

  // 2. Trigger GridCraft Save programmatically
  async function requestGridCraftSave(downloadAfterSave) {
    pendingLocalDownload = Boolean(downloadAfterSave);
    if (window.gridcraft && typeof window.gridcraft.execute === "function") {
      try {
        await window.gridcraft.execute("file.save", {});
        return;
      } catch {
        // Fallback to simulated Ctrl+S via control protocol
        try {
          await window.gridcraft.request("ui.key", { key: "S", cmd: true });
          return;
        } catch {
          // ignore
        }
      }
    }
    if (downloadAfterSave && lastSavedBuffer) {
      pendingLocalDownload = false;
      triggerBrowserDownload(lastSavedBuffer.name, lastSavedBuffer.bytes);
    } else {
      showToast("請於編輯器按下 Ctrl+S 或 File › Save 儲存");
    }
  }

  // 3. Open file bytes into GridCraft canvas
  function openBytesInGridCraft(name, arrayBuffer) {
    setDocName(name);
    lastSavedBuffer = { name, bytes: arrayBuffer.slice ? arrayBuffer.slice(0) : arrayBuffer };
    canvas.dispatchEvent(
      new CustomEvent("gridcraft:open", {
        detail: { name, bytes: arrayBuffer },
      })
    );
    isDirty = false;
    updateStatusBadge("ha-status-synced", "✓ 已載入");
    showToast(`📖 已開啟: ${name}`);
  }

  // 4. Server File List Modal Management
  async function refreshServerFileList(renderModal = true) {
    try {
      const res = await fetch("./api/files");
      if (!res.ok) return;
      const data = await res.json();
      const files = Array.isArray(data.files) ? data.files : [];
      if (data.storage_dir) {
        storageDir = data.storage_dir;
        pathBadgeEl.textContent = storageDir;
        modalPathEl.textContent = storageDir;
      }
      fileCountEl.textContent = String(files.length);
      if (!renderModal && !modalBackdrop.classList.contains("open")) return;

      fileListEl.textContent = "";
      if (files.length === 0) {
        const emptyLi = document.createElement("li");
        emptyLi.className = "ha-empty-hint";
        emptyLi.appendChild(document.createTextNode(`目前 ${storageDir} 尚無試算表。`));
        emptyLi.appendChild(document.createElement("br"));
        emptyLi.appendChild(document.createTextNode("您可以在 GridCraft 按下 "));
        const bold = document.createElement("b");
        bold.textContent = "Ctrl+S (💾 存至 HA)";
        emptyLi.appendChild(bold);
        emptyLi.appendChild(document.createTextNode(" 或點擊上方「📤 上傳本機檔案至 HA」。"));
        fileListEl.appendChild(emptyLi);
        return;
      }

      for (const f of files) {
        const li = document.createElement("li");
        li.className = "ha-file-row";

        const infoDiv = document.createElement("div");
        infoDiv.className = "ha-file-info";

        const nameDiv = document.createElement("div");
        nameDiv.className = "ha-file-name";
        nameDiv.title = `點擊在 GridCraft 中開啟 ${f.name}`;
        nameDiv.textContent = `📊 ${f.name}`;

        const metaDiv = document.createElement("div");
        metaDiv.className = "ha-file-meta";
        metaDiv.textContent = `${formatBytes(f.size)} · ${formatTime(f.mtime)}`;

        infoDiv.appendChild(nameDiv);
        infoDiv.appendChild(metaDiv);

        const actionsDiv = document.createElement("div");
        actionsDiv.className = "ha-file-actions";

        const openBtn = document.createElement("button");
        openBtn.className = "ha-btn ha-btn-primary";
        openBtn.textContent = "📖 開啟";

        const dlBtn = document.createElement("button");
        dlBtn.className = "ha-btn";
        dlBtn.textContent = "⬇ 下載";

        const renameBtn = document.createElement("button");
        renameBtn.className = "ha-btn";
        renameBtn.textContent = "✏ 改名";

        const delBtn = document.createElement("button");
        delBtn.className = "ha-btn";
        delBtn.style.color = "#f87171";
        delBtn.textContent = "🗑";

        actionsDiv.appendChild(openBtn);
        actionsDiv.appendChild(dlBtn);
        actionsDiv.appendChild(renameBtn);
        actionsDiv.appendChild(delBtn);

        li.appendChild(infoDiv);
        li.appendChild(actionsDiv);

        const openHandler = async () => {
          try {
            const r = await fetch(`./api/files/${encodeURIComponent(f.name)}`);
            if (!r.ok) throw new Error(`HTTP ${r.status}`);
            const buf = await r.arrayBuffer();
            openBytesInGridCraft(f.name, buf);
            modalBackdrop.classList.remove("open");
          } catch (err) {
            showToast(`❌ 開啟失敗: ${err.message || err}`);
          }
        };

        nameDiv.addEventListener("click", openHandler);
        openBtn.addEventListener("click", openHandler);

        dlBtn.addEventListener("click", () => {
          const a = document.createElement("a");
          a.href = `./api/files/${encodeURIComponent(f.name)}?download=1`;
          a.download = f.name;
          document.body.appendChild(a);
          a.click();
          a.remove();
        });

        renameBtn.addEventListener("click", async () => {
          const nextName = window.prompt("請輸入新的檔案名稱：", f.name);
          if (!nextName || !nextName.trim() || nextName.trim() === f.name) return;
          try {
            const r = await fetch(`./api/files/${encodeURIComponent(f.name)}/rename`, {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ new_name: nextName.trim() }),
            });
            const d = await r.json().catch(() => ({}));
            if (!r.ok) throw new Error(d.error || `HTTP ${r.status}`);
            if (currentDocName === f.name) setDocName(d.name || nextName.trim());
            showToast(`✓ 已重新命名為 ${d.name || nextName.trim()}`);
            refreshServerFileList(true);
          } catch (err) {
            showToast(`❌ 改名失敗: ${err.message || err}`);
          }
        });

        delBtn.addEventListener("click", async () => {
          if (!window.confirm(`確定要從 HA 伺服器刪除「${f.name}」嗎？`)) return;
          try {
            const r = await fetch(`./api/files/${encodeURIComponent(f.name)}`, { method: "DELETE" });
            const d = await r.json().catch(() => ({}));
            if (!r.ok) throw new Error(d.error || `HTTP ${r.status}`);
            showToast(`🗑 已刪除 ${f.name}`);
            refreshServerFileList(true);
          } catch (err) {
            showToast(`❌ 刪除失敗: ${err.message || err}`);
          }
        });

        fileListEl.appendChild(li);
      }
    } catch {
      // ignore fetch error
    }
  }

  // 5. Wire up UI controls
  document.getElementById("ha_btn_server_files").addEventListener("click", () => {
    modalBackdrop.classList.add("open");
    refreshServerFileList(true);
  });

  document.getElementById("ha_modal_close").addEventListener("click", () => {
    modalBackdrop.classList.remove("open");
  });

  modalBackdrop.addEventListener("click", (e) => {
    if (e.target === modalBackdrop) {
      modalBackdrop.classList.remove("open");
    }
  });

  document.getElementById("ha_modal_refresh_btn").addEventListener("click", () => {
    refreshServerFileList(true);
  });

  document.getElementById("ha_modal_save_current_btn").addEventListener("click", () => {
    requestGridCraftSave(false);
  });

  document.getElementById("ha_btn_save_ha").addEventListener("click", () => {
    requestGridCraftSave(false);
  });

  document.getElementById("ha_btn_download_local").addEventListener("click", () => {
    requestGridCraftSave(true);
  });

  chkAlsoDl.addEventListener("change", () => {
    alsoDownload = chkAlsoDl.checked;
    localStorage.setItem("ha_gc_also_download", String(alsoDownload));
    showToast(alsoDownload ? "✓ 已開啟「存檔時同步下載至本機」" : "已設為「存檔僅寫入 HA 伺服器」");
  });

  document.getElementById("ha_btn_open_local").addEventListener("click", () => {
    hiddenLocalOpenInput.value = "";
    hiddenLocalOpenInput.click();
  });

  hiddenLocalOpenInput.addEventListener("change", async () => {
    const file = hiddenLocalOpenInput.files && hiddenLocalOpenInput.files[0];
    if (!file) return;
    const buf = await file.arrayBuffer();
    openBytesInGridCraft(file.name, buf);
  });

  document.getElementById("ha_modal_upload_btn").addEventListener("click", () => {
    hiddenServerUploadInput.value = "";
    hiddenServerUploadInput.click();
  });

  hiddenServerUploadInput.addEventListener("change", async () => {
    const files = hiddenServerUploadInput.files;
    if (!files || files.length === 0) return;
    let lastUploaded = null;
    for (let i = 0; i < files.length; i++) {
      const f = files[i];
      const buf = await f.arrayBuffer();
      try {
        await saveBytesToServer(f.name, buf);
        lastUploaded = { name: f.name, buf };
      } catch (err) {
        showToast(`❌ 上傳 ${f.name} 失敗: ${err.message || err}`);
      }
    }
    await refreshServerFileList(true);
    if (lastUploaded && files.length === 1) {
      openBytesInGridCraft(lastUploaded.name, lastUploaded.buf);
      modalBackdrop.classList.remove("open");
    }
  });

  document.getElementById("ha_btn_collapse").addEventListener("click", () => {
    document.body.classList.remove("ha-bar-open");
    document.body.classList.add("ha-bar-collapsed");
    window.dispatchEvent(new Event("resize"));
  });

  expandPill.addEventListener("click", () => {
    document.body.classList.remove("ha-bar-collapsed");
    document.body.classList.add("ha-bar-open");
    window.dispatchEvent(new Event("resize"));
  });
})();
