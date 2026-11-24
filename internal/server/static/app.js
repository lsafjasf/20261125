"use strict";

const $ = (id) => document.getElementById(id);
const state = {
  sessionId: null,
  analysis: null,
  waveform: null,
  selectedIndex: 0,
  showContext: false,
  sensitivityTimer: null,
  sensitivitySeq: 0,
  audioUrl: "",
};

const els = {
  fileInput: $("fileInput"),
  uploadBtn: $("uploadBtn"),
  demoBtn: $("demoBtn"),
  ingestStatus: $("ingestStatus"),
  dropzone: $("dropzone"),
  workspace: $("workspace"),
  toast: $("toast"),
  audio: $("audio"),
  canvas: $("waveCanvas"),
  waveEmpty: $("waveEmpty"),
  timeAxis: $("timeAxis"),
  sensRange: $("sensRange"),
  revisionNote: $("revisionNote"),
  showContext: $("showContext"),
};

function init() {
  els.uploadBtn.addEventListener("click", () => els.fileInput.click());
  els.demoBtn.addEventListener("click", loadDemo);
  els.fileInput.addEventListener("change", () => {
    if (els.fileInput.files[0]) uploadFile(els.fileInput.files[0]);
  });
  els.dropzone.addEventListener("click", () => els.fileInput.click());
  els.dropzone.addEventListener("dragover", (event) => {
    event.preventDefault();
    els.dropzone.classList.add("dragover");
  });
  els.dropzone.addEventListener("dragleave", () => els.dropzone.classList.remove("dragover"));
  els.dropzone.addEventListener("drop", (event) => {
    event.preventDefault();
    els.dropzone.classList.remove("dragover");
    const file = event.dataTransfer.files[0];
    if (file) uploadFile(file);
  });
  $("copyHash").addEventListener("click", copyHash);
  $("showContext").addEventListener("change", () => {
    state.showContext = els.showContext.checked;
    renderFindings();
    drawWaveform();
  });
  els.sensRange.addEventListener("input", onSensitivityInput);
  els.audio.addEventListener("timeupdate", scheduleWaveDraw);
  els.audio.addEventListener("seeked", scheduleWaveDraw);
  els.canvas.addEventListener("click", onWaveClick);
  window.addEventListener("resize", scheduleWaveDraw);
  $("findingsBody").addEventListener("click", onFindingClick);
  $("inspectBody").addEventListener("click", onInspectorClick);
  setTimeout(() => {
    if (!state.analysis) loadDemo();
  }, 320);
}

async function api(url, options) {
  const response = await fetch(url, options);
  let body = null;
  try {
    body = await response.json();
  } catch (_) {
    body = null;
  }
  if (!response.ok) {
    throw new Error((body && body.error) || `请求失败（${response.status}）`);
  }
  return body;
}

function setBusy(busy, message) {
  els.uploadBtn.disabled = busy;
  els.demoBtn.disabled = busy;
  if (message) setStatus(message);
}

function setStatus(message) {
  els.ingestStatus.textContent = message;
}

async function loadDemo() {
  setBusy(true, "正在生成演示样本并计算上下文、相位与重复证据…");
  try {
    const data = await api("/api/demo", { method: "POST" });
    ingestResponse(data);
    setStatus("演示样本已载入 · 原始音频只读，波形与证据已对齐");
    showToast("演示样本已载入");
  } catch (error) {
    setStatus(error.message);
    showToast(error.message, true);
  } finally {
    setBusy(false);
  }
}

async function uploadFile(file) {
  if (!/\.wav$/i.test(file.name) && file.type && !/wav/i.test(file.type)) {
    showToast("请选择 WAV 文件", true);
    return;
  }
  setBusy(true, `正在解析 ${file.name}，原始字节将保持不变…`);
  try {
    const data = await api(`/api/sessions?name=${encodeURIComponent(file.name)}`, {
      method: "POST",
      headers: { "Content-Type": file.type || "application/octet-stream" },
      body: file,
    });
    ingestResponse(data);
    setStatus(`${file.name} 分析完成 · 原始 SHA-256 已锁定`);
    showToast("分析完成，原始音频已锁定");
  } catch (error) {
    setStatus(error.message);
    showToast(error.message, true);
  } finally {
    setBusy(false);
  }
}

function ingestResponse(data) {
  state.sessionId = data.id;
  state.analysis = data.analysis;
  state.audioUrl = data.audioUrl;
  state.waveform = null;
  state.selectedIndex = firstFlaggedIndex();
  els.audio.src = data.audioUrl;
  $("downloadOriginal").href = data.audioUrl;
  $("downloadOriginal").download = state.analysis.metadata.filename;
  els.workspace.hidden = false;
  els.waveEmpty.hidden = false;
  els.sensRange.value = String(Math.round((state.analysis.sensitivity ?? 0.55) * 100));
  els.revisionNote.classList.remove("active");
  els.revisionNote.textContent = "等待灵敏度调整";
  renderAll();
  loadWaveform(data.waveformUrl);
  els.workspace.scrollIntoView({ behavior: "smooth", block: "start" });
}

async function loadWaveform(url) {
  try {
    const data = await api(url);
    if (!state.analysis) return;
    state.waveform = data;
    els.waveEmpty.hidden = true;
    drawWaveform();
  } catch (error) {
    showToast(`波形读取失败：${error.message}`, true);
  }
}

function firstFlaggedIndex() {
  if (!state.analysis) return 0;
  const found = state.analysis.intervals.findIndex((iv) => iv.flagged);
  if (found >= 0) return found;
  const anomaly = state.analysis.intervals.findIndex((iv) => (iv.context?.score || 0) >= 0.35);
  return anomaly >= 0 ? anomaly : 0;
}

function renderAll() {
  renderMeta();
  renderMetrics();
  renderSensitivityCopy();
  renderFindings();
  renderInspector();
  renderCompare();
  renderRepeats();
  renderAxis();
  drawWaveform();
}

function renderMeta() {
  const meta = state.analysis.metadata;
  $("fileName").textContent = meta.filename;
  $("fileHash").textContent = meta.sha256;
  $("fileHash").title = meta.sha256;
  $("metaDuration").textContent = formatDuration(meta.duration);
  $("metaRate").textContent = `${meta.sampleRate.toLocaleString()} Hz`;
  $("metaFormat").textContent = `${meta.channels} ch · ${meta.bitsPerSample} bit`;
  const cfg = state.analysis.config;
  $("metaGrid").textContent = `${cfg.frameSize} / ${cfg.hop}`;
  $("audioGuard").textContent = `SHA ${meta.sha256.slice(0, 10)}… · 未降噪`;
  els.audio.title = `${meta.filename} · SHA-256 ${meta.sha256}`;
}

function renderMetrics() {
  const stats = state.analysis.stats;
  const legacy = state.analysis.legacyStats;
  const repeats = state.analysis.repeats || [];
  $("metricFlagged").textContent = String(stats.flagged);
  $("metricFlaggedSub").textContent = `${(stats.flaggedRatio * 100).toFixed(1)}% · ${stats.redRanges} 段`;
  $("metricLegacy").textContent = String(legacy.flagged);
  $("metricLegacySub").textContent = `${(legacy.flaggedRatio * 100).toFixed(1)}% · 同一网格对照`;
  $("metricRepeats").textContent = String(repeats.length);
  $("metricRepeatsSub").textContent = repeats.length ? "已逐样本复核" : "未发现精确复制";
  const alignment = state.analysis.alignment;
  $("metricAligned").textContent = alignment.aligned ? "✓" : "!";
  $("metricAlignedSub").textContent = `${alignment.intervalCount} 个区间 · 边界一致`;
}

function renderSensitivityCopy() {
  const sensitivity = state.analysis.sensitivity;
  const threshold = state.analysis.threshold.confidenceCutoff;
  $("sensTitle").textContent = sensitivity.toFixed(2);
  $("sensExplain").textContent =
    `置信度阈值 ${threshold.toFixed(3)}；上下文单独最多 0.28，低于最低阈值 0.40。`;
}

function formatDuration(seconds) {
  const total = Math.max(0, Number(seconds) || 0);
  const mins = Math.floor(total / 60);
  const secs = total - mins * 60;
  return `${mins}:${secs.toFixed(2).padStart(5, "0")}`;
}

function formatRange(start, end) {
  return `${formatDuration(start)} – ${formatDuration(end)}`;
}

function esc(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

let toastTimer = null;
function showToast(message, error = false) {
  els.toast.textContent = message;
  els.toast.classList.toggle("error", error);
  els.toast.classList.add("show");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => els.toast.classList.remove("show"), 3200);
}

async function copyHash() {
  const hash = state.analysis?.metadata?.sha256;
  if (!hash) return;
  try {
    await navigator.clipboard.writeText(hash);
    showToast("SHA-256 已复制");
  } catch (_) {
    showToast("浏览器未允许剪贴板访问", true);
  }
}

function onSensitivityInput() {
  if (!state.analysis) return;
  const value = Number(els.sensRange.value) / 100;
  $("sensTitle").textContent = value.toFixed(2);
  clearTimeout(state.sensitivityTimer);
  state.sensitivityTimer = setTimeout(() => applySensitivity(value), 130);
}

async function applySensitivity(value) {
  if (!state.sessionId) return;
  const seq = ++state.sensitivitySeq;
  try {
    const resp = await api(`/api/sessions/${state.sessionId}/sensitivity`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sensitivity: value }),
    });
    if (seq !== state.sensitivitySeq) return;
    state.analysis.sensitivity = resp.sensitivity;
    state.analysis.threshold = resp.threshold;
    state.analysis.stats = resp.stats;
    state.analysis.alignment = resp.alignment;
    for (const change of (resp.revision.changes || [])) {
      const interval = state.analysis.intervals[change.index];
      if (interval) interval.flagged = change.flagged;
    }
    renderMetrics();
    renderSensitivityCopy();
    renderFindings();
    renderInspector();
    renderCompare();
    drawWaveform();
    renderRevision(resp);
  } catch (error) {
    showToast(error.message, true);
  }
}

function renderRevision(resp) {
  const revision = resp.revision;
  const note = els.revisionNote;
  note.classList.add("active");
  if (revision.recomputedIntervals === 0) {
    note.textContent = `阈值 ${revision.threshold.confidenceCutoff.toFixed(3)} · 0 个区间重算，结论复用`;
    return;
  }
  const ranges = revision.recomputedRanges || [];
  const shown = ranges.slice(0, 3).map((range) => formatRange(range.start, range.end)).join("；");
  const more = ranges.length > 3 ? ` 等 ${ranges.length} 段` : "";
  note.textContent =
    `仅重算 ${revision.recomputedIntervals} 个区间（${ranges.length} 段）：${shown}${more} · ` +
    `红色 ${revision.before.flagged} → ${revision.after.flagged}`;
}

function contextOnly(interval) {
  return (interval.context?.score || 0) >= 0.35;
}

function findingGroups(includeContext) {
  const groups = [];
  let current = null;
  state.analysis.intervals.forEach((interval, index) => {
    const visible = interval.flagged || (includeContext && contextOnly(interval));
    if (!visible) {
      current = null;
      return;
    }
    if (!current || index !== current.last + 1) {
      current = {
        first: index,
        last: index,
        maxIndex: index,
        maxConfidence: interval.confidence,
        flagged: interval.flagged,
        context: contextOnly(interval),
        phase: (interval.phase?.score || 0) >= 0.30,
        repeat: (interval.repeat?.score || 0) >= 0.5,
      };
      groups.push(current);
      return;
    }
    current.last = index;
    current.flagged ||= interval.flagged;
    current.context ||= contextOnly(interval);
    current.phase ||= (interval.phase?.score || 0) >= 0.30;
    current.repeat ||= (interval.repeat?.score || 0) >= 0.5;
    if (interval.confidence > current.maxConfidence) {
      current.maxConfidence = interval.confidence;
      current.maxIndex = index;
    }
  });
  return groups;
}

function renderFindings() {
  if (!state.analysis) return;
  const groups = findingGroups(state.showContext);
  const rows = [];
  for (const group of groups.slice(0, 220)) {
    const start = state.analysis.intervals[group.first].start;
    const end = state.analysis.intervals[group.last].end;
    const chips = [];
    if (group.repeat) chips.push('<span class="chip purple">重复样本</span>');
    if (group.phase) chips.push('<span class="chip cyan">相位跳变</span>');
    if (group.context) chips.push('<span class="chip amber">上下文</span>');
    const verdict = group.flagged
      ? '<span class="chip red">红色</span>'
      : '<span class="chip amber">仅观察</span>';
    rows.push(
      `<tr data-index="${group.maxIndex}">
        <td class="time">${formatRange(start, end)}</td>
        <td class="score">${group.maxConfidence.toFixed(3)}</td>
        <td>${chips.join("") || '<span class="chip amber">阈值附近</span>'}</td>
        <td>${verdict}</td>
      </tr>`
    );
  }
  $("findingsBody").innerHTML = rows.join("");
  $("findingsEmpty").hidden = rows.length > 0;
  $("findingsEmpty").textContent = state.showContext
    ? "当前灵敏度下没有红色或上下文异常区间。"
    : "当前灵敏度下没有红色区间；可勾选显示上下文异常。";
}

function onFindingClick(event) {
  const row = event.target.closest("tr[data-index]");
  if (!row) return;
  selectInterval(Number(row.dataset.index), true);
}

function selectInterval(index, seek) {
  if (!state.analysis || !state.analysis.intervals[index]) return;
  state.selectedIndex = index;
  renderInspector();
  document.querySelectorAll("#findingsBody tr").forEach((row) => {
    row.classList.toggle("selected", Number(row.dataset.index) === index);
  });
  drawWaveform();
  if (seek) {
    const interval = state.analysis.intervals[index];
    els.audio.currentTime = interval.start;
    els.audio.play().catch(() => {});
  }
}

function renderInspector() {
  if (!state.analysis) return;
  const interval = state.analysis.intervals[state.selectedIndex] || state.analysis.intervals[0];
  if (!interval) return;
  const verdict = $("inspectVerdict");
  verdict.textContent = interval.flagged ? "红色区间" : "未达阈值";
  verdict.className = `verdict ${interval.flagged ? "flag" : "safe"}`;
  $("inspectTitle").textContent = `${formatDuration(interval.start)} – ${formatDuration(interval.end)}`;
  const reasons = [];
  if ((interval.repeat?.score || 0) >= 0.5) reasons.push('<span class="chip purple">重复样本</span>');
  if ((interval.phase?.score || 0) >= 0.30) reasons.push('<span class="chip cyan">相位跳变</span>');
  if ((interval.context?.score || 0) >= 0.35) reasons.push('<span class="chip amber">上下文突变</span>');
  const ctx = interval.context || {};
  const phase = interval.phase || {};
  const repeat = interval.repeat || {};
  const minDB = Math.min(ctx.leftDb || 0, ctx.rightDb || 0, ctx.localDb || 0) - 3;
  const maxDB = Math.max(ctx.leftDb || 0, ctx.rightDb || 0, ctx.localDb || 0) + 3;
  const span = Math.max(1, maxDB - minDB);
  const localPos = clampPercent((((ctx.localDb || 0) - minDB) / span) * 100);
  const predictedPos = clampPercent((((ctx.predictedDb || 0) - minDB) / span) * 100);
  const repeatLine = repeat.score
    ? `${formatRange(repeat.matchStart, repeat.matchEnd)}（${esc(repeat.segmentId || "R")}）`
    : "未命中";
  $("inspectBody").innerHTML = `
    <div class="inspect-hero">
      <strong>${interval.confidence.toFixed(3)}</strong>
      <span>RMS ${Number(interval.rmsDb).toFixed(1)} dBFS</span>
    </div>
    <div class="reason-row">${reasons.join("") || '<span class="muted">无独立证据；分类由阈值决定。</span>'}</div>
    <section class="evidence-block">
      <h4><span>上下文窗口</span><b>${Number(ctx.score || 0).toFixed(3)}</b></h4>
      <div class="kv-grid">
        <div><span>局部</span><code>${Number(ctx.localDb || 0).toFixed(1)} dB</code></div>
        <div><span>趋势预测</span><code>${Number(ctx.predictedDb || 0).toFixed(1)} dB</code></div>
        <div><span>残差</span><code>${Number(ctx.residualDb || 0).toFixed(1)} dB</code></div>
        <div><span>鲁棒离散</span><code>${Number(ctx.spreadDb || 0).toFixed(1)} dB</code></div>
        <div><span>底噪</span><code>${Number(ctx.noiseFloorDb || 0).toFixed(1)} dB</code></div>
        <div><span>左 / 右</span><code>${Number(ctx.leftDb || 0).toFixed(1)} / ${Number(ctx.rightDb || 0).toFixed(1)}</code></div>
      </div>
      <div class="ctx-bar" title="青色为上下文趋势，红色为局部电平">
        <i style="left:${predictedPos}%"></i><b style="left:${localPos}%"></b>
      </div>
      <p class="explain-note">左窗 ${formatDuration(ctx.leftStart || 0)}–${formatDuration(ctx.leftEnd || 0)}，右窗 ${formatDuration(ctx.rightStart || 0)}–${formatDuration(ctx.rightEnd || 0)}。慢漂移会被趋势预测吸收。</p>
    </section>
    <section class="evidence-block">
      <h4><span>相位连续性</span><b>${Number(phase.score || 0).toFixed(3)}</b></h4>
      <div class="kv-grid">
        <div><span>原始跳变</span><code>${Number(phase.jump || 0).toFixed(3)}</code></div>
        <div><span>上下文基线</span><code>${Number(phase.baseline || 0).toFixed(3)}</code></div>
        <div><span>调性权重</span><code>${Number(phase.tonal || 0).toFixed(3)}</code></div>
        <div><span>通道间</span><code>${phase.interChannel ? "参与" : "未触发"}</code></div>
      </div>
      <p class="explain-note">相位残差与相邻历史增量比较，宽带噪声会抬高基线，接缝才会产生相对跳变。</p>
    </section>
    <section class="evidence-block">
      <h4><span>重复样本</span><b>${Number(repeat.score || 0).toFixed(3)}</b></h4>
      <div class="kv-grid">
        <div><span>匹配片段</span><code>${repeatLine}</code></div>
        <div><span>指纹方式</span><code>PCM 精确复核</code></div>
      </div>
      <p class="explain-note">静音窗口不会进入指纹桶；只有连续窗口命中并逐样本一致才形成复制区间。</p>
    </section>
    <button class="btn btn-ghost" type="button" data-action="seek">定位到该区间</button>
  `;
}

function clampPercent(value) {
  return Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0));
}

function onInspectorClick(event) {
  if (event.target.closest('[data-action="seek"]')) {
    const interval = state.analysis.intervals[state.selectedIndex];
    if (interval) {
      els.audio.currentTime = interval.start;
      els.audio.play().catch(() => {});
    }
  }
}

function renderCompare() {
  const current = state.analysis.stats;
  const legacy = state.analysis.legacyStats;
  const rows = [
    ["时间网格区间", legacy.intervals, current.intervals, current.intervals - legacy.intervals, true],
    ["标红区间", legacy.flagged, current.flagged, current.flagged - legacy.flagged],
    ["红色占比", `${(legacy.flaggedRatio * 100).toFixed(1)}%`, `${(current.flaggedRatio * 100).toFixed(1)}%`, `${((current.flaggedRatio - legacy.flaggedRatio) * 100).toFixed(1)} pp`],
    ["红色时长", `${legacy.flaggedSeconds.toFixed(2)} s`, `${current.flaggedSeconds.toFixed(2)} s`, `${(current.flaggedSeconds - legacy.flaggedSeconds).toFixed(2)} s`],
    ["红色片段数", legacy.redRanges, current.redRanges, current.redRanges - legacy.redRanges],
    ["上下文异常区间", legacy.evidence.context || legacy.evidence.noiseFloor || 0, current.evidence.context || 0, (current.evidence.context || 0) - (legacy.evidence.context || legacy.evidence.noiseFloor || 0)],
    ["相位证据区间", 0, current.evidence.phase || 0, current.evidence.phase || 0],
    ["重复证据区间", 0, current.evidence.repeat || 0, current.evidence.repeat || 0],
  ];
  $("compareBody").innerHTML = rows.map((row) => {
    const delta = row[4] === true ? "—" : (row[3] > 0 ? `+${row[3]}` : String(row[3]));
    return `<tr><td>${row[0]}</td><td>${row[1]}</td><td>${row[2]}</td><td>${delta}</td></tr>`;
  }).join("");

  const alignment = state.analysis.alignment;
  const badge = $("alignBadge");
  badge.textContent = alignment.aligned ? "对齐 ✓" : "网格不一致";
  badge.style.color = alignment.aligned ? "var(--green)" : "var(--red)";
  $("alignList").innerHTML = [
    ["区间数", alignment.intervalCount],
    ["网格起点", `${alignment.gridStart.toFixed(3)} s`],
    ["网格终点", `${alignment.gridEnd.toFixed(3)} s`],
    ["区间步长", `${alignment.intervalSeconds.toFixed(5)} s`],
    ["旧 / 新统计源", "同一 Intervals 数组"],
    ["原始 SHA-256", `${state.analysis.metadata.sha256.slice(0, 16)}…`],
  ].map(([key, value]) => `<li><span>${key}</span><code>${esc(value)}</code></li>`).join("");
  $("compareNote").textContent =
    `旧页 ${legacy.flagged} 个红色由底噪规则触发；当前证据包含 ${current.evidence.context || 0} 个上下文异常、` +
    `${current.evidence.phase || 0} 个相位证据、${current.evidence.repeat || 0} 个重复证据。` +
    "上下文单独贡献封顶 0.28，低于最低红色阈值 0.40，因此底噪漂移不会直接标红。";
}

function renderRepeats() {
  const repeats = state.analysis.repeats || [];
  const list = $("repeatList");
  if (!repeats.length) {
    list.innerHTML = '<p class="muted">未发现通过逐样本复核的精确重复区间。</p>';
    return;
  }
  list.innerHTML = repeats.map((repeat) => `
    <div class="repeat-item">
      <span class="repeat-id">${esc(repeat.id)}</span>
      <code>${formatRange(repeat.start, repeat.end)}</code>
      <span>复制自 <code>${formatRange(repeat.matchStart, repeat.matchEnd)}</code></span>
    </div>
  `).join("");
}

let waveRaf = 0;
function scheduleWaveDraw() {
  if (waveRaf) return;
  waveRaf = requestAnimationFrame(() => {
    waveRaf = 0;
    drawWaveform();
  });
}

function drawWaveform() {
  if (!state.analysis) return;
  const canvas = els.canvas;
  const rect = canvas.getBoundingClientRect();
  if (rect.width < 20) return;
  const dpr = Math.min(2, window.devicePixelRatio || 1);
  const width = Math.floor(rect.width);
  const height = Math.floor(rect.height || 220);
  if (canvas.width !== Math.floor(width * dpr) || canvas.height !== Math.floor(height * dpr)) {
    canvas.width = Math.floor(width * dpr);
    canvas.height = Math.floor(height * dpr);
  }
  const ctx = canvas.getContext("2d");
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);

  const padding = 14;
  const innerW = Math.max(1, width - padding * 2);
  const mid = height * 0.51;
  const duration = Math.max(0.001, state.analysis.metadata.duration);
  const xFor = (time) => padding + (Math.max(0, Math.min(duration, time)) / duration) * innerW;

  ctx.fillStyle = "#090d15";
  ctx.fillRect(0, 0, width, height);

  ctx.strokeStyle = "rgba(142, 168, 210, 0.08)";
  ctx.lineWidth = 1;
  for (let i = 0; i <= 8; i++) {
    const x = padding + (innerW * i) / 8;
    ctx.beginPath();
    ctx.moveTo(x, 8);
    ctx.lineTo(x, height - 8);
    ctx.stroke();
  }
  ctx.strokeStyle = "rgba(142, 168, 210, 0.14)";
  ctx.beginPath();
  ctx.moveTo(padding, mid);
  ctx.lineTo(width - padding, mid);
  ctx.stroke();

  const intervals = state.analysis.intervals;
  for (const interval of intervals) {
    const x0 = xFor(interval.start);
    const x1 = Math.max(x0 + 1, xFor(interval.end));
    if (interval.flagged) {
      ctx.fillStyle = "rgba(255, 77, 99, 0.13)";
      ctx.fillRect(x0, 0, x1 - x0, height);
      ctx.fillStyle = "rgba(255, 77, 99, 0.88)";
      ctx.fillRect(x0, 0, x1 - x0, 3);
    } else if ((interval.context?.score || 0) >= 0.35) {
      ctx.fillStyle = "rgba(255, 176, 46, 0.62)";
      ctx.fillRect(x0, 5, Math.max(1, x1 - x0), 2);
    }
    if ((interval.phase?.score || 0) >= 0.30) {
      ctx.fillStyle = "rgba(70, 215, 245, 0.92)";
      ctx.fillRect(x0, height - 10, Math.max(2, x1 - x0), 6);
    }
    if ((interval.repeat?.score || 0) >= 0.5) {
      ctx.fillStyle = "rgba(179, 140, 255, 0.65)";
      ctx.fillRect(x0, height - 4, Math.max(1, x1 - x0), 3);
    }
  }

  const peaks = state.waveform?.peaks || [];
  if (peaks.length) {
    const amp = height * 0.41;
    const maxPeak = Math.max(0.0001, ...peaks);
    ctx.strokeStyle = "rgba(173, 204, 236, 0.58)";
    ctx.lineWidth = 1;
    ctx.beginPath();
    for (let i = 0; i < peaks.length; i++) {
      const x = padding + (innerW * i) / Math.max(1, peaks.length - 1);
      const p = Math.max(0, Math.min(1, Math.pow((peaks[i] || 0) / maxPeak, 0.72)));
      ctx.moveTo(x, mid - p * amp);
      ctx.lineTo(x, mid + p * amp);
    }
    ctx.stroke();
    ctx.strokeStyle = "rgba(70, 215, 245, 0.20)";
    ctx.beginPath();
    for (let i = 0; i < peaks.length; i++) {
      const x = padding + (innerW * i) / Math.max(1, peaks.length - 1);
      const p = Math.max(0, Math.min(1, Math.pow((peaks[i] || 0) / maxPeak, 0.72)));
      if (i === 0) ctx.moveTo(x, mid - p * amp);
      else ctx.lineTo(x, mid - p * amp);
    }
    ctx.stroke();
  }

  for (const repeat of state.analysis.repeats || []) {
    drawRepeatBracket(ctx, xFor(repeat.start), xFor(repeat.end), height, height * 0.09, "rgba(179, 140, 255, 0.75)");
    drawRepeatBracket(ctx, xFor(repeat.matchStart), xFor(repeat.matchEnd), height, height * 0.72, "rgba(179, 140, 255, 0.38)");
  }

  const selected = intervals[state.selectedIndex];
  if (selected) {
    const x = xFor(selected.start);
    ctx.strokeStyle = "rgba(238, 244, 255, 0.7)";
    ctx.setLineDash([3, 3]);
    ctx.beginPath();
    ctx.moveTo(x, 4);
    ctx.lineTo(x, height - 4);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  if (els.audio.currentTime > 0) {
    const x = xFor(els.audio.currentTime);
    ctx.strokeStyle = "rgba(89, 227, 167, 0.88)";
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.moveTo(x, 0);
    ctx.lineTo(x, height);
    ctx.stroke();
  }
}

function drawRepeatBracket(ctx, x0, x1, height, y, color) {
  if (!Number.isFinite(x0) || !Number.isFinite(x1)) return;
  ctx.strokeStyle = color;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(x0, y + 7);
  ctx.lineTo(x0, y);
  ctx.lineTo(x1, y);
  ctx.lineTo(x1, y + 7);
  ctx.stroke();
}

function renderAxis() {
  if (!state.analysis) return;
  const duration = state.analysis.metadata.duration;
  const count = window.innerWidth < 700 ? 4 : 8;
  const parts = [];
  for (let i = 0; i <= count; i++) {
    const t = (duration * i) / count;
    parts.push(`<span>${formatTick(t, duration)}</span>`);
  }
  els.timeAxis.innerHTML = parts.join("");
}

function formatTick(seconds, duration) {
  if (duration >= 120) {
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins}:${String(secs).padStart(2, "0")}`;
  }
  return `${seconds.toFixed(1)}s`;
}

function onWaveClick(event) {
  if (!state.analysis) return;
  const rect = els.canvas.getBoundingClientRect();
  const padding = 14;
  const inner = Math.max(1, rect.width - padding * 2);
  const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left - padding) / inner));
  const time = ratio * state.analysis.metadata.duration;
  const intervals = state.analysis.intervals;
  const step = state.analysis.alignment.intervalSeconds || 0.02;
  let index = Math.max(0, Math.min(intervals.length - 1, Math.floor(time / step)));
  if (intervals[index] && time > intervals[index].end && index + 1 < intervals.length) index++;
  selectInterval(index, false);
}

window.addEventListener("DOMContentLoaded", init);
