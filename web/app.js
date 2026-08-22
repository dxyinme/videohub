(() => {
  const listEl = document.getElementById("video-list");
  const statusEl = document.getElementById("status");
  const uploadStatusEl = document.getElementById("upload-status");
  const player = document.getElementById("player");
  const nowPlaying = document.getElementById("now-playing");
  const refreshBtn = document.getElementById("refresh");
  const uploadForm = document.getElementById("upload-form");
  const fileInput = document.getElementById("file-input");
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

  refreshBtn.addEventListener("click", () => {
    loadVideos();
  });

  uploadForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const file = fileInput.files && fileInput.files[0];
    if (!file) {
      setUploadStatus("请先选择 MP4 文件。", true);
      return;
    }
    if (!/\.mp4$/i.test(file.name)) {
      setUploadStatus("仅支持 .mp4 文件。", true);
      return;
    }

    const body = new FormData();
    body.append("file", file, file.name);

    uploadBtn.disabled = true;
    fileInput.disabled = true;
    setUploadStatus(`正在上传 ${file.name}（${formatSize(file.size)}）…`);

    try {
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
        throw new Error(msg.trim());
      }
      const id = payload && payload.video && payload.video.id;
      setUploadStatus(`上传成功：${(payload && payload.video && payload.video.path) || file.name}`);
      uploadForm.reset();
      await loadVideos({ preferId: id || "" });
    } catch (err) {
      console.error(err);
      setUploadStatus(err.message || "上传失败。", true);
    } finally {
      uploadBtn.disabled = false;
      fileInput.disabled = false;
    }
  });

  loadVideos();
})();
