(function () {
  "use strict";

  const canvas = document.getElementById("wordcraft_canvas");
  if (!canvas) return;

  // Enable WordCraft same-document host bridge so all Save / Export actions flow through wordcraft:save
  canvas.setAttribute("data-host", "page");
  document.body.classList.add("ha-bar-open");

  let currentDocName = "Document.docx";
  let isDirty = false;
  let storageDir = "/share/wordcraft";
  let alsoDownload = localStorage.getItem("ha_wc_also_download") === "true";
  let pendingLocalDownload = false;
  let lastSavedBuffer = null;
  let toastTimer = null;

  // Build top control bar, expand pill, modal, and toast
  const topbar = document.createElement("div");
  topbar.id = "ha_topbar";
  topbar.innerHTML = `
    <div class="ha-bar-left">
      <span class="ha-brand">WordCraft</span>
      <span id="ha_doc_name" class="ha-doc-pill" title="目前開啟的文件名稱">Document.docx</span>
      <span id="ha_dirty_badge" class="ha-status-badge ha-status-synced">✓ 已就緒</span>
      <span id="ha_path_badge" class="ha-path-badge">/share/wordcraft</span>
    </div>
    <div class="ha-bar-right">
      <button id="ha_btn_server_files" class="ha-btn ha-btn-primary" title="瀏覽、開啟或管理 Home Assistant 伺服器上的文件">
        📂 HA 伺服器文件庫 (<span id="ha_file_count">0</span>)
      </button>
      <button id="ha_btn_open_local" class="ha-btn" title="從本機電腦選取文件載入至 WordCraft">
        💻 開啟本機檔
      </button>
      <button id="ha_btn_save_ha" class="ha-btn ha-btn-emerald" title="立即將目前文件儲存至 Home Assistant 伺服器 (Ctrl+S)">
        💾 存至 HA
      </button>
      <button id="ha_btn_download_local" class="ha-btn" title="儲存並下載目前文件至本機電腦">
        ⬇ 下載本機
      </button>
      <label class="ha-toggle-label" title="勾選後，每次在 WordCraft 按下 Ctrl+S 或匯出時，除了存入 HA 伺服器也會同步下載至本機電腦">
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
  expandPill.title = "展開 HA 雲端文件庫工具列";
  expandPill.innerHTML = "☁ HA 文件庫 ▼";
  document.body.appendChild(expandPill);

  const modalBackdrop = document.createElement("div");
  modalBackdrop.id = "ha_modal_backdrop";
  modalBackdrop.innerHTML = `
    <div class="ha-modal" role="dialog" aria-modal="true">
      <div class="ha-modal-header">
        <div>
          <h3 class="ha-modal-title">📂 Home Assistant 伺服器文件庫</h3>
          <div id="ha_modal_storage_path" class="ha-modal-sub">/share/wordcraft</div>
        </div>
        <button id="ha_modal_close" class="ha-btn">✕ 關閉</button>
      </div>
      <div class="ha-modal-toolbar">
        <div style="display:flex;gap:6px;flex-wrap:wrap;">
          <button id="ha_modal_upload_btn" class="ha-btn ha-btn-primary">📤 上傳本機檔案至 HA</button>
          <button id="ha_modal_save_current_btn" class="ha-btn ha-btn-emerald">💾 將目前編輯文件存入 HA</button>
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
  hiddenLocalOpenInput.accept = ".docx,.docm,.dotx,.dotm,.doc,.dot,.odt,.ott,.fodt,.rtf,.txt,.md,.html,.htm,.tex,.json,.xml";
  document.body.appendChild(hiddenLocalOpenInput);

  const hiddenServerUploadInput = document.createElement("input");
  hiddenServerUploadInput.type = "file";
  hiddenServerUploadInput.multiple = true;
  hiddenServerUploadInput.style.display = "none";
  document.body.appendChild(hiddenServerUploadInput);

  const docNameEl = document.getElementById("ha_doc_name");
  const dirtyBadgeEl = document.getElementById("ha_dirty_badge");
  const pathBadgeEl = document.getElementById("ha_path_badge");
  const fileCountEl = document.getElementById("ha_file_count");
  const chkAlsoDl = document.getElementById("ha_chk_also_dl");
  const fileListEl = document.getElementById("ha_file_list");
  const modalPathEl = document.getElementById("ha_modal_storage_path");

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

  function triggerBrowserDownload(name, arrayBuffer) {
    const blob = new Blob([arrayBuffer], { type: "application/octet-stream" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = name || "Document.docx";
    a.style.display = "none";
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 10000);
  }

  async function saveBytesToServer(name, arrayBuffer) {
    updateStatusBadge("ha-status-saving", "⟳ 儲存至 HA 中...");
    const cleanName = (name || currentDocName || "Document.docx").trim();
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

  // 1. Register WordCraft Host Events BEFORE wasm init resolves
  canvas.addEventListener("wordcraft:ready", async () => {
    try {
      const cfgRes = await fetch("./api/config");
      if (cfgRes.ok) {
        const cfg = await cfgRes.json();
        if (cfg.storage_dir) {
          storageDir = cfg.storage_dir;
          pathBadgeEl.textContent = storageDir;
          modalPathEl.textContent = storageDir;
        }
        if (localStorage.getItem("ha_wc_also_download") === null && typeof cfg.also_download_on_save === "boolean") {
          alsoDownload = cfg.also_download_on_save;
          chkAlsoDl.checked = alsoDownload;
        }
        if (window.wordcraft && typeof window.wordcraft.execute === "function") {
          if (cfg.author_name) {
            window.wordcraft.execute("file.setAuthor", { author: cfg.author_name, name: cfg.author_name }).catch(() => {});
          }
          if (cfg.default_language && cfg.default_language !== "auto" && !localStorage.getItem("ha_wc_lang_init")) {
            window.wordcraft
              .execute("ui.language", { value: cfg.default_language })
              .then(() => localStorage.setItem("ha_wc_lang_init", "1"))
              .catch(() => {});
          }
        }
      }
    } catch {
      // ignore config fetch error
    }
    refreshServerFileList(false);
  });

  canvas.addEventListener("wordcraft:save", async (event) => {
    const detail = event.detail || {};
    const name = detail.name || currentDocName || "Document.docx";
    const saveAs = Boolean(detail.saveAs);
    const bytes = detail.bytes;
    if (!bytes) return;

    lastSavedBuffer = { name, bytes: bytes.slice ? bytes.slice(0) : bytes };
    if (saveAs || /\.(docx|odt|rtf|md|txt)$/i.test(name)) {
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

  canvas.addEventListener("wordcraft:dirty", (event) => {
    isDirty = Boolean(event.detail && event.detail.dirty);
    if (isDirty) {
      updateStatusBadge("ha-status-dirty", "● 未儲存變更");
    } else {
      updateStatusBadge("ha-status-synced", "✓ 已同步");
    }
  });

  canvas.addEventListener("wordcraft:failed", (event) => {
    const err = (event.detail && event.detail.error) || "WebGPU/WebGL2 初始化失敗";
    showToast(`❌ WordCraft 啟動失敗: ${err}`);
  });

  window.addEventListener("beforeunload", (e) => {
    if (isDirty) {
      e.preventDefault();
      e.returnValue = "";
    }
  });

  // 2. Trigger WordCraft Save programmatically
  async function requestWordCraftSave(downloadAfterSave) {
    pendingLocalDownload = Boolean(downloadAfterSave);
    if (window.wordcraft && typeof window.wordcraft.execute === "function") {
      try {
        await window.wordcraft.execute("file.save", {});
        return;
      } catch {
        // Fallback to simulated Ctrl+S via control protocol
        try {
          await window.wordcraft.request("ui.key", { key: "S", cmd: true });
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

  // 3. Open file bytes into WordCraft canvas
  function openBytesInWordCraft(name, arrayBuffer) {
    setDocName(name);
    lastSavedBuffer = { name, bytes: arrayBuffer.slice ? arrayBuffer.slice(0) : arrayBuffer };
    canvas.dispatchEvent(
      new CustomEvent("wordcraft:open", {
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

      fileListEl.innerHTML = "";
      if (files.length === 0) {
        fileListEl.innerHTML = `<li class="ha-empty-hint">目前 ${storageDir} 尚無文件。<br>您可以在 WordCraft 按下 <b>Ctrl+S (💾 存至 HA)</b> 或點擊上方「📤 上傳本機檔案至 HA」。</li>`;
        return;
      }

      for (const f of files) {
        const li = document.createElement("li");
        li.className = "ha-file-row";
        li.innerHTML = `
          <div class="ha-file-info">
            <div class="ha-file-name" title="點擊在 WordCraft 中開啟 ${f.name}">📄 ${f.name}</div>
            <div class="ha-file-meta">${formatBytes(f.size)} · ${formatTime(f.mtime)}</div>
          </div>
          <div class="ha-file-actions">
            <button class="ha-btn ha-btn-primary" data-action="open">📖 開啟</button>
            <button class="ha-btn" data-action="download">⬇ 下載</button>
            <button class="ha-btn" data-action="rename">✏ 改名</button>
            <button class="ha-btn" data-action="delete" style="color:#f87171;">🗑</button>
          </div>
        `;

        const openHandler = async () => {
          try {
            const r = await fetch(`./api/files/${encodeURIComponent(f.name)}`);
            if (!r.ok) throw new Error(`HTTP ${r.status}`);
            const buf = await r.arrayBuffer();
            openBytesInWordCraft(f.name, buf);
            modalBackdrop.classList.remove("open");
          } catch (err) {
            showToast(`❌ 開啟失敗: ${err.message || err}`);
          }
        };

        li.querySelector(".ha-file-name").addEventListener("click", openHandler);
        li.querySelector('[data-action="open"]').addEventListener("click", openHandler);

        li.querySelector('[data-action="download"]').addEventListener("click", () => {
          const a = document.createElement("a");
          a.href = `./api/files/${encodeURIComponent(f.name)}?download=1`;
          a.download = f.name;
          document.body.appendChild(a);
          a.click();
          a.remove();
        });

        li.querySelector('[data-action="rename"]').addEventListener("click", async () => {
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

        li.querySelector('[data-action="delete"]').addEventListener("click", async () => {
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
    requestWordCraftSave(false);
  });

  document.getElementById("ha_btn_save_ha").addEventListener("click", () => {
    requestWordCraftSave(false);
  });

  document.getElementById("ha_btn_download_local").addEventListener("click", () => {
    requestWordCraftSave(true);
  });

  chkAlsoDl.addEventListener("change", () => {
    alsoDownload = chkAlsoDl.checked;
    localStorage.setItem("ha_wc_also_download", String(alsoDownload));
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
    openBytesInWordCraft(file.name, buf);
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
      openBytesInWordCraft(lastUploaded.name, lastUploaded.buf);
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
