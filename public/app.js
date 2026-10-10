const rowsEl = document.getElementById("rows");
const formEl = document.getElementById("download-form");
const addBtn = document.getElementById("add-row");
const jobListEl = document.getElementById("job-list");

function addRow(url = "", name = "") {
  const row = document.createElement("div");
  row.className = "row";

  const urlInput = document.createElement("input");
  urlInput.type = "text";
  urlInput.placeholder = "m3u8 URL (required)";
  urlInput.value = url;
  urlInput.className = "input-url";

  const nameInput = document.createElement("input");
  nameInput.type = "text";
  nameInput.placeholder = "File name";
  nameInput.value = name;
  nameInput.className = "input-name";

  const removeBtn = document.createElement("button");
  removeBtn.type = "button";
  removeBtn.className = "btn-remove";
  removeBtn.textContent = "×";
  removeBtn.title = "Remove row";
  removeBtn.addEventListener("click", () => {
    if (rowsEl.children.length > 1) row.remove();
    else { urlInput.value = ""; nameInput.value = ""; }
  });

  row.append(urlInput, nameInput, removeBtn);
  rowsEl.appendChild(row);
  return row;
}

addBtn.addEventListener("click", () => addRow());

formEl.addEventListener("submit", async (e) => {
  e.preventDefault();
  const jobs = [];
  rowsEl.querySelectorAll(".row").forEach((row) => {
    const url = row.querySelector(".input-url").value.trim();
    const filename = row.querySelector(".input-name").value.trim();
    if (url) jobs.push({ url, filename });
  });

  if (jobs.length === 0) {
    alert("Enter at least one m3u8 URL.");
    return;
  }

  const submitBtn = formEl.querySelector("button[type=submit]");
  submitBtn.disabled = true;
  try {
    const res = await fetch("/api/download", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ jobs }),
    });
    if (!res.ok) {
      const text = await res.text();
      alert("Error: " + text);
      return;
    }
    // Clear inputs after successful submit
    rowsEl.innerHTML = "";
    addRow();
    refreshJobs();
  } finally {
    submitBtn.disabled = false;
  }
});

function renderJobs(jobs) {
  if (!jobs || jobs.length === 0) {
    jobListEl.innerHTML = '<li class="empty">No jobs yet</li>';
    return;
  }
  // newest first
  jobs.sort((a, b) => new Date(b.started_at) - new Date(a.started_at));
  jobListEl.innerHTML = "";
  jobs.forEach((j) => {
    const li = document.createElement("li");
    li.className = "job";

    const info = document.createElement("div");
    info.className = "job-info";
    const name = document.createElement("div");
    name.className = "job-name";
    name.textContent = j.filename;
    const url = document.createElement("div");
    url.className = "job-url";
    url.textContent = j.url;
    info.append(name, url);
    if (j.error) {
      const err = document.createElement("div");
      err.className = "job-error";
      err.textContent = j.error;
      info.appendChild(err);
    }

    const badge = document.createElement("span");
    badge.className = "badge " + j.status;
    badge.textContent = j.status;

    const right = document.createElement("div");
    right.className = "job-right";
    right.appendChild(badge);

    if (j.status === "queued" || j.status === "running") {
      const cancelBtn = document.createElement("button");
      cancelBtn.type = "button";
      cancelBtn.className = "btn-cancel";
      cancelBtn.textContent = "Cancel";
      cancelBtn.addEventListener("click", () => cancelJob(j.id));
      right.appendChild(cancelBtn);
    }

    li.append(info, right);
    jobListEl.appendChild(li);
  });
}

async function cancelJob(id) {
  try {
    await fetch("/api/cancel?id=" + encodeURIComponent(id), { method: "POST" });
    refreshJobs();
  } catch (_) {
    /* ignore */
  }
}

async function refreshJobs() {
  try {
    const res = await fetch("/api/jobs");
    if (!res.ok) return;
    const data = await res.json();
    if (data.output_dir) {
      document.getElementById("output-dir").textContent = data.output_dir;
    }
    renderJobs(data.jobs);
  } catch (_) {
    /* server not ready yet */
  }
}

// ---- Memory usage chart ----

const memCanvas = document.getElementById("mem-chart");
const memCtx = memCanvas.getContext("2d");
const samples = [];
const MAX_SAMPLES = 120; // ~2 minutes at 1s interval

function fmtBytes(b) {
  if (b >= 1 << 30) return (b / (1 << 30)).toFixed(2) + " GB";
  if (b >= 1 << 20) return (b / (1 << 20)).toFixed(1) + " MB";
  if (b >= 1 << 10) return (b / (1 << 10)).toFixed(1) + " KB";
  return b + " B";
}

function drawChart() {
  const w = memCanvas.clientWidth;
  const h = memCanvas.clientHeight;
  const dpr = window.devicePixelRatio || 1;
  if (memCanvas.width !== Math.round(w * dpr)) {
    memCanvas.width = Math.round(w * dpr);
    memCanvas.height = Math.round(h * dpr);
  }
  memCtx.setTransform(dpr, 0, 0, dpr, 0, 0);
  memCtx.clearRect(0, 0, w, h);

  const pad = { top: 8, right: 8, bottom: 18, left: 52 };
  const cw = w - pad.left - pad.right;
  const ch = h - pad.top - pad.bottom;

  if (samples.length === 0) return;

  const maxVal = Math.max(...samples.map((s) => Math.max(s.heap, s.sys)), 1);

  // Grid + y-axis labels
  memCtx.font = "10px monospace";
  memCtx.fillStyle = "#6b7280";
  memCtx.strokeStyle = "#262b36";
  memCtx.lineWidth = 1;
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + ch - (ch * i) / 4;
    memCtx.beginPath();
    memCtx.moveTo(pad.left, y);
    memCtx.lineTo(w - pad.right, y);
    memCtx.stroke();
    const v = (maxVal * i) / 4;
    memCtx.fillText(fmtBytes(v), 4, y + 3);
  }

  const xAt = (i) => pad.left + (cw * i) / (MAX_SAMPLES - 1);
  const yAt = (v) => pad.top + ch - (ch * v) / maxVal;

  // sys line (green)
  if (samples.length > 1) {
    memCtx.strokeStyle = "#4ade80";
    memCtx.lineWidth = 1.5;
    memCtx.beginPath();
    samples.forEach((s, i) => {
      const x = xAt(i + (MAX_SAMPLES - samples.length));
      const y = yAt(s.sys);
      i === 0 ? memCtx.moveTo(x, y) : memCtx.lineTo(x, y);
    });
    memCtx.stroke();
  }

  // heap line (blue) with fill
  memCtx.strokeStyle = "#4f7cff";
  memCtx.lineWidth = 1.5;
  memCtx.beginPath();
  samples.forEach((s, i) => {
    const x = xAt(i + (MAX_SAMPLES - samples.length));
    const y = yAt(s.heap);
    i === 0 ? memCtx.moveTo(x, y) : memCtx.lineTo(x, y);
  });
  memCtx.stroke();

  if (samples.length > 1) {
    memCtx.lineTo(xAt(MAX_SAMPLES - 1), pad.top + ch);
    memCtx.lineTo(xAt(MAX_SAMPLES - samples.length), pad.top + ch);
    memCtx.closePath();
    memCtx.fillStyle = "rgba(79,124,255,0.15)";
    memCtx.fill();
  }

  // x-axis time label
  memCtx.fillStyle = "#6b7280";
  memCtx.fillText("−2 min", pad.left, h - 4);
  memCtx.fillText("now", w - pad.right - 20, h - 4);
}

async function refreshMem() {
  try {
    const res = await fetch("/api/memstats");
    if (!res.ok) return;
    const m = await res.json();

    samples.push({ t: m.timestamp, heap: m.heap_alloc, sys: m.sys });
    if (samples.length > MAX_SAMPLES) samples.shift();

    document.getElementById("stat-heap").textContent = fmtBytes(m.heap_alloc);
    document.getElementById("stat-heapsys").textContent = fmtBytes(m.heap_sys);
    document.getElementById("stat-sys").textContent = fmtBytes(m.sys);
    document.getElementById("stat-goro").textContent = m.goroutines;
    document.getElementById("stat-gc").textContent = m.num_gc;

    drawChart();
  } catch (_) {
    /* server not ready yet */
  }
}

addRow();
refreshJobs();
refreshMem();
setInterval(refreshJobs, 1500);
setInterval(refreshMem, 1000);
window.addEventListener("resize", drawChart);
