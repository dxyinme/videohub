(() => {
  const listEl = document.getElementById("entry-list");
  const statusEl = document.getElementById("status");
  const uploadStatusEl = document.getElementById("upload-status");
  const selectionHintEl = document.getElementById("selection-hint");
  const breadcrumbEl = document.getElementById("breadcrumb");
  const player = document.getElementById("player");
  const nowPlaying = document.getElementById("now-playing");
  const refreshBtn = document.getElementById("refresh");
  const uploadForm = document.getElementById("upload-form");
  const fileInput = document.getElementById("file-input");
  const folderInput = document.getElementById("folder-input");
  const uploadBtn = document.getElementById("upload-btn");
  const searchForm = document.getElementById("search-form");
  const searchInput = document.getElementById("search-input");
  const clearSearchBtn = document.getElementById("clear-search");

  /** @type {"browse"|"search"} */
  let mode = "browse";
  let currentPath = "";
  let searchQuery = "";
  let currentId = "";
  /** @type {{id:string,name:string,path:string,size:number}[]} */
  let visibleVideos = [];

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

  function syncURL({ preferId } = {}) {
    const url = new URL(window.location.href);
    if (mode === "search" && searchQuery) {
      url.searchParams.set("q", searchQuery);
      url.searchParams.delete("path");
    } else {
      url.searchParams.delete("q");
      if (currentPath) {
        url.searchParams.set("path", currentPath);
      } else {
        url.searchParams.delete("path");
      }
    }
    const id = preferId || currentId;
    if (id) {
      url.searchParams.set("v", id);
    }
    history.replaceState(null, "", url);
  }

  function selectVideo(video, { autoplay = true, updateURL = true } = {}) {
    if (!video || !video.id) return;
    currentId = video.id;
    nowPlaying.textContent = video.path || video.name;
    player.src = streamURL(video.id);
    if (autoplay) {
      player.play().catch(() => {});
    }
    for (const btn of listEl.querySelectorAll("button[data-id]")) {
      btn.classList.toggle("active", btn.dataset.id === video.id);
    }
    if (updateURL) syncURL({ preferId: video.id });
  }

  function renderBreadcrumb() {
    breadcrumbEl.innerHTML = "";
    breadcrumbEl.hidden = mode === "search";
    if (mode === "search") return;

    const rootBtn = document.createElement("button");
    rootBtn.type = "button";
    rootBtn.className = "crumb";
    rootBtn.textContent = "根目录";
    rootBtn.addEventListener("click", () => enterPath(""));
    breadcrumbEl.appendChild(rootBtn);

    if (!currentPath) return;
    const parts = currentPath.split("/");
    let acc = "";
    for (const part of parts) {
      acc = acc ? `${acc}/${part}` : part;
      const sep = document.createElement("span");
      sep.className = "crumb-sep";
      sep.textContent = "/";
      breadcrumbEl.appendChild(sep);

      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "crumb";
      btn.textContent = part;
      const target = acc;
      btn.addEventListener("click", () => enterPath(target));
      breadcrumbEl.appendChild(btn);
    }
  }

  function appendFolderRow(folder) {
    const li = document.createElement("li");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "entry entry-folder";
    btn.innerHTML =
      `<span class="icon" aria-hidden="true">DIR</span>` +
      `<span class="body"><span class="name"></span><span class="meta">文件夹</span></span>`;
    btn.querySelector(".name").textContent = folder.name;
    btn.addEventListener("click", () => enterPath(folder.path));
    li.appendChild(btn);
    listEl.appendChild(li);
  }

  function appendVideoRow(video, { showPath = false } = {}) {
    const li = document.createElement("li");
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "entry entry-video";
    btn.dataset.id = video.id;
    btn.innerHTML =
      `<span class="icon" aria-hidden="true">MP4</span>` +
      `<span class="body"><span class="name"></span><span class="meta"></span></span>`;
    btn.querySelector(".name").textContent = showPath ? video.path || video.name : video.name;
    const metaParts = [formatSize(video.size)];
    if (showPath && video.path && video.path.includes("/")) {
      const dir = video.path.slice(0, video.path.lastIndexOf("/"));
      metaParts.push(dir);
    }
    btn.querySelector(".meta").textContent = metaParts.filter(Boolean).join(" · ");
    btn.addEventListener("click", () => selectVideo(video));
    if (video.id === currentId) btn.classList.add("active");
    li.appendChild(btn);
    listEl.appendChild(li);
  }

  function renderBrowse(data) {
    listEl.innerHTML = "";
    visibleVideos = Array.isArray(data.videos) ? data.videos : [];
    const folders = Array.isArray(data.folders) ? data.folders : [];

    if (currentPath) {
      const li = document.createElement("li");
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "entry entry-up";
      btn.innerHTML =
        `<span class="icon" aria-hidden="true">..</span>` +
        `<span class="body"><span class="name">返回上级</span><span class="meta"></span></span>`;
      btn.addEventListener("click", () => enterPath(data.parent || ""));
      li.appendChild(btn);
      listEl.appendChild(li);
    }

    for (const f of folders) appendFolderRow(f);
    for (const v of visibleVideos) appendVideoRow(v);

    if (folders.length === 0 && visibleVideos.length === 0) {
      setStatus("此文件夹为空。");
    } else {
      setStatus(`文件夹 ${folders.length} · 视频 ${visibleVideos.length}`);
    }
    renderBreadcrumb();
  }

  function renderSearch(videos) {
    listEl.innerHTML = "";
    visibleVideos = Array.isArray(videos) ? videos : [];
    for (const v of visibleVideos) appendVideoRow(v, { showPath: true });
    if (visibleVideos.length === 0) {
      setStatus(`未找到匹配「${searchQuery}」的视频。`);
    } else {
      setStatus(`搜索「${searchQuery}」：${visibleVideos.length} 个结果（最多 200）`);
    }
    renderBreadcrumb();
  }

  async function loadBrowse(path, { preferId = "", autoplay = false } = {}) {
    mode = "browse";
    currentPath = path || "";
    searchQuery = "";
    searchInput.value = "";
    clearSearchBtn.hidden = true;
    setStatus("加载中…");
    try {
      const qs = currentPath
        ? `?path=${encodeURIComponent(currentPath)}`
        : "";
      const res = await fetch(`/api/browse${qs}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      currentPath = data.path || "";
      renderBrowse(data);
      const wanted =
        preferId || new URL(window.location.href).searchParams.get("v");
      if (wanted) {
        const hit = visibleVideos.find((v) => v.id === wanted);
        if (hit) {
          selectVideo(hit, { autoplay, updateURL: true });
        } else {
          syncURL({ preferId: wanted });
        }
      } else {
        syncURL();
      }
    } catch (err) {
      console.error(err);
      listEl.innerHTML = "";
      setStatus("加载目录失败。", true);
    }
  }

  async function loadSearch(q, { preferId = "", autoplay = false } = {}) {
    const query = (q || "").trim();
    if (!query) {
      await loadBrowse(currentPath);
      return;
    }
    mode = "search";
    searchQuery = query;
    searchInput.value = query;
    clearSearchBtn.hidden = false;
    setStatus("搜索中…");
    try {
      const res = await fetch(`/api/search?q=${encodeURIComponent(query)}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      renderSearch(Array.isArray(data.videos) ? data.videos : []);
      const wanted = preferId || currentId;
      if (wanted) {
        const hit = visibleVideos.find((v) => v.id === wanted);
        if (hit) selectVideo(hit, { autoplay, updateURL: true });
        else syncURL({ preferId: wanted });
      } else {
        syncURL();
      }
    } catch (err) {
      console.error(err);
      listEl.innerHTML = "";
      setStatus("搜索失败。", true);
    }
  }

  function enterPath(path) {
    loadBrowse(path || "");
  }

  async function refresh() {
    if (mode === "search" && searchQuery) {
      await loadSearch(searchQuery, { preferId: currentId });
    } else {
      await loadBrowse(currentPath, { preferId: currentId });
    }
  }

  async function uploadOne(file) {
    const path = uploadPath(file);
    const body = new FormData();
    body.append("path", path);
    body.append("file", file, file.name);
    const res = await fetch("/api/videos", { method: "POST", body });
    const text = await res.text();
    let payload = null;
    try {
      payload = text ? JSON.parse(text) : null;
    } catch {
      /* non-json */
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
    refresh();
  });

  fileInput.addEventListener("change", updateSelectionHint);
  folderInput.addEventListener("change", updateSelectionHint);

  searchForm.addEventListener("submit", (event) => {
    event.preventDefault();
    loadSearch(searchInput.value);
  });

  clearSearchBtn.addEventListener("click", () => {
    searchInput.value = "";
    loadBrowse(currentPath, { preferId: currentId });
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

    if (lastId && lastId.includes("/")) {
      currentPath = lastId.slice(0, lastId.lastIndexOf("/"));
    }
    await loadBrowse(currentPath, { preferId: lastId, autoplay: Boolean(lastId) });
    uploadBtn.disabled = false;
    fileInput.disabled = false;
    folderInput.disabled = false;
  });

  updateSelectionHint();

  const params = new URL(window.location.href).searchParams;
  const initialQ = params.get("q");
  const initialPath = params.get("path") || "";
  const initialV = params.get("v") || "";
  if (initialQ) {
    loadSearch(initialQ, { preferId: initialV });
  } else if (initialV && initialV.includes("/")) {
    const dir = initialV.slice(0, initialV.lastIndexOf("/"));
    loadBrowse(dir, { preferId: initialV });
  } else {
    loadBrowse(initialPath, { preferId: initialV });
  }
})();
