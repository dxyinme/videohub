(() => {
  const listEl = document.getElementById("video-list");
  const statusEl = document.getElementById("status");
  const uploadStatusEl = document.getElementById("upload-status");
  const selectionHintEl = document.getElementById("selection-hint");
  const player = document.getElementById("player");
  const nowPlaying = document.getElementById("now-playing");
  const refreshBtn = document.getElementById("refresh");
  const uploadForm = document.getElementById("upload-form");
  const fileInput = document.getElementById("file-input");
  const folderInput = document.getElementById("folder-input");
  const uploadBtn = document.getElementById("upload-btn");

  let videos = [];
  let currentId = "";

  function formatSize(bytes) {
    if (!Number.isFinite(bytes) || bytes < 0) return "";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let n = bytes;
    let i = 0;
    while (n >= 1024 && i < units.length - 1) {
      n /= 1024;
      i += 1;
    }
    return `${n.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
  }

  function streamURL(id) {
    const parts = id.split("/").map(encodeURIComponent);
    return `/api/stream/${parts.join("/")}`;
  }

  function setStatus(text, isError = false) {
    statusEl.textContent = text;
    statusEl.classList.toggle("error", isError);
  }

  function setUploadStatus(text, isError = false) {
    uploadStatusEl.textContent = text;
    uploadStatusEl.classList.toggle("error", isError);
  }

  function isMp4(file) {
    return /\.mp4$/i.test(file.name);
  }

  function uploadPath(file) {
    const rel = (file.webkitRelativePath || "").replace(/\\/g, "/").replace(/^\/+/, "");
    if (rel && rel.includes("/")) {
      return rel;
    }
    return file.name;
  }

  function collectMp4Files() {
    const selected = [];
    const seen = new Set();
    for (const input of [fileInput, folderInput]) {
      const list = input.files ? Array.from(input.files) : [];
      for (const file of list) {
        if (!isMp4(file)) continue;
        const key = `${uploadPath(file)}::${file.size}::${file.lastModified}`;
        if (seen.has(key)) continue;
        seen.add(key);
        selected.push(file);
      }
    }
    return selected;
  }

  function updateSelectionHint() {
    const files = collectMp4Files();
    const folderCount = folderInput.files ? folderInput.files.length : 0;
    const skipped = folderCount > 0
      ? Math.max(0, folderCount - Array.from(folderInput.files || []).filter(isMp4).length)
      : 0;
    if (files.length === 0) {
      selectionHintEl.textContent =
        folderCount > 0
          ? `文件夹中未找到 MP4（已忽略 ${folderCount} 个非视频/其他文件）`
          : "未选择文件";
      return;
    }
    const total = files.reduce((sum, f) => sum + f.size, 0);
    let text = `已选 ${files.length} 个 MP4，共 ${formatSize(total)}`;
    if (skipped > 0) {
      text += `（已自动忽略 ${skipped} 个非 MP4）`;
    }
    selectionHintEl.textContent = text;
  }

  function selectVideo(id, { autoplay = true, updateURL = true } = {}) {
    const item = videos.find((v) => v.id === id);
    if (!item) return;

    currentId = id;
    nowPlaying.textContent = item.path || item.name;
    player.src = streamURL(id);
    if (autoplay) {
      player.play().catch(() => {
        /* autoplay may be blocked; controls remain available */
      });
    }

    for (const btn of listEl.querySelectorAll("button[data-id]")) {
      btn.classList.toggle("active", btn.dataset.id === id);
    }

    if (updateURL) {
      const url = new URL(window.location.href);
      url.searchParams.set("v", id);
      history.replaceState(null, "", url);
    }
  }

  function renderList() {
    listEl.innerHTML = "";
    if (videos.length === 0) {
      setStatus("未找到 MP4 视频，请上传或检查视频目录配置。");
      return;
    }

    setStatus(`共 ${videos.length} 个视频`);
    for (const v of videos) {
      const li = document.createElement("li");
      const btn = document.createElement("button");
      btn.type = "button";
      btn.dataset.id = v.id;
      btn.innerHTML = `<span class="name"></span><span class="meta"></span>`;
      btn.querySelector(".name").textContent = v.path || v.name;
      btn.querySelector(".meta").textContent = formatSize(v.size);
      btn.addEventListener("click", () => selectVideo(v.id));
      if (v.id === currentId) btn.classList.add("active");
      li.appendChild(btn);
      listEl.appendChild(li);
    }
  }

  async function loadVideos({ preferId = "" } = {}) {
    setStatus("加载中…");
    try {
      const res = await fetch("/api/videos");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      videos = Array.isArray(data.videos) ? data.videos : [];
      renderList();

      const wanted =
        preferId || new URL(window.location.href).searchParams.get("v");
      if (wanted && videos.some((v) => v.id === wanted)) {
        selectVideo(wanted, {
          autoplay: Boolean(preferId),
          updateURL: Boolean(preferId),
        });
      }
    } catch (err) {
      console.error(err);
      videos = [];
      listEl.innerHTML = "";
      setStatus("加载视频列表失败。", true);
    }
  }

  async function uploadOne(file) {
    const path = uploadPath(file);
    const body = new FormData();
    // Separate path field: Go's multipart FileName() strips directories (RFC 7578).
    body.append("path", path);
    body.append("file", file, file.name);
    const res = await fetch("/api/videos", { method: "POST", body });
    const text = await res.text();
    let payload = null;
    try {
      payload = text ? JSON.parse(text) : null;
    } catch {
      /* non-json error body */
    }
    if (!res.ok) {
      const msg =
        (payload && payload.error) ||
        text ||
        `上传失败（HTTP ${res.status}）`;
      throw new Error(`${path}: ${msg.trim()}`);
    }
    return payload && payload.video ? payload.video : null;
  }

  refreshBtn.addEventListener("click", () => {
    loadVideos();
  });

  fileInput.addEventListener("change", () => {
    updateSelectionHint();
  });
  folderInput.addEventListener("change", () => {
    updateSelectionHint();
  });

  uploadForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const files = collectMp4Files();
    if (files.length === 0) {
      setUploadStatus("请选择 MP4 文件或包含 MP4 的文件夹。", true);
      return;
    }

    uploadBtn.disabled = true;
    fileInput.disabled = true;
    folderInput.disabled = true;

    let ok = 0;
    let failed = 0;
    let lastId = "";
    const errors = [];

    for (let i = 0; i < files.length; i += 1) {
      const file = files[i];
      const path = uploadPath(file);
      setUploadStatus(
        `正在上传 ${i + 1}/${files.length}：${path}（${formatSize(file.size)}）…`
      );
      try {
        const video = await uploadOne(file);
        ok += 1;
        if (video && video.id) lastId = video.id;
      } catch (err) {
        failed += 1;
        errors.push(err.message || String(err));
        console.error(err);
      }
    }

    if (failed === 0) {
      setUploadStatus(`全部上传成功：${ok} 个文件`);
      uploadForm.reset();
      updateSelectionHint();
    } else {
      setUploadStatus(
        `完成：成功 ${ok}，失败 ${failed}。${errors.slice(0, 3).join("；")}`,
        true
      );
    }

    await loadVideos({ preferId: lastId });
    uploadBtn.disabled = false;
    fileInput.disabled = false;
    folderInput.disabled = false;
  });

  updateSelectionHint();
  loadVideos();
})();
