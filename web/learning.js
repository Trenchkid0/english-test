/* Hallmark · extension: Workbench · genre: playful · tone: soft · theme: Hum · parent shell: Narrative Workflow */

const learningState = { prompt: "", activeTool: "dashboard", listening: false };
const writingDraftKey = "ruang-kata-writing-draft";

const typeNames = {
  grammar: "Grammar",
  vocabulary: "Vocabulary",
  reading: "Reading",
  fill_blank: "Fill the blank",
  listening: "Listening",
  error_identification: "Error Identification",
};

function formatNumber(value, suffix = "") {
  return value == null ? "—" : `${new Intl.NumberFormat("id-ID", { maximumFractionDigits: 1 }).format(value)}${suffix}`;
}
function learnerPrompt(prompt) { return String(prompt || "").replace(/\s*\[Level[^\]]*\]\s*$/u, "").trim(); }

function setTool(name) {
  learningState.activeTool = name;
  $$('[role="tab"][data-tool]').forEach((tab) => {
    const active = tab.dataset.tool === name;
    tab.setAttribute("aria-selected", String(active));
    tab.tabIndex = active ? 0 : -1;
  });
  $$('[data-panel]').forEach((panel) => { panel.hidden = panel.dataset.panel !== name; });
  if (name === "dashboard") loadDashboard();
  if (name === "review") loadReviews();
  if (name === "vocabulary") loadVocabulary();
  if (name === "writing") loadWritingPrompt();
  if (name === "writing") loadWritingHistory();
}

async function openLearning(tool = "dashboard") {
  clearInterval(state.timerID);
  state.timerID = null;
  showScreen("learning");
  setTool(tool);
}

$$('[role="tab"][data-tool]').forEach((tab, tabIndex, tabs) => {
  tab.addEventListener("click", () => setTool(tab.dataset.tool));
  tab.addEventListener("keydown", (event) => {
    if (!['ArrowLeft', 'ArrowRight'].includes(event.key)) return;
    event.preventDefault();
    const direction = event.key === 'ArrowRight' ? 1 : -1;
    const next = tabs[(tabIndex + direction + tabs.length) % tabs.length];
    next.focus({ preventScroll: true });
    setTool(next.dataset.tool);
  });
});

async function loadDashboard() {
  const metrics = $("#dashboard-metrics");
  metrics.innerHTML = '<p class="empty-copy">Memuat ringkasan…</p>';
  try {
    const data = await api("/api/dashboard");
    const values = [
      ["Sesi selesai", formatNumber(data.completedSessions)],
      ["Rata-rata nilai", formatNumber(data.averageScore, data.averageScore == null ? "" : "/100")],
      ["Review hari ini", formatNumber(data.dueReviews)],
      ["Kata tersimpan", formatNumber(data.vocabularyCount)],
      ["Writing dinilai", formatNumber(data.writingCount)],
    ];
    metrics.innerHTML = values.map(([label, value]) => `<article class="metric"><span>${escapeHTML(label)}</span><strong>${escapeHTML(value)}</strong></article>`).join("");
    $("#weakness-list").innerHTML = data.weaknesses.length ? data.weaknesses.map((item) => `
      <div class="weakness-row"><span><strong>${escapeHTML(typeNames[item.type] || item.type)}</strong><small>${item.wrong} dari ${item.total} belum tepat</small></span><strong>${item.accuracy}%</strong></div>`).join("") : '<p class="empty-copy">Selesaikan satu sesi untuk melihat pola kelemahan.</p>';
    $("#weekly-list").innerHTML = data.weekly.length ? data.weekly.map((item) => {
      const date = new Intl.DateTimeFormat("id-ID", { weekday: "short", day: "numeric" }).format(new Date(`${item.date}T00:00:00`));
      return `<div class="week-row"><span>${escapeHTML(date)} · ${item.sessions} sesi</span><strong>${item.average}/100</strong></div>`;
    }).join("") : '<p class="empty-copy">Belum ada sesi selesai dalam tujuh hari terakhir.</p>';
    const pulse = $("#skill-pulse-list");
    pulse.innerHTML = data.weaknesses.length ? data.weaknesses.map((item) => `<div class="skill-pulse-row"><span><strong>${escapeHTML(typeNames[item.type] || item.type)}</strong><small>${item.total} jawaban tercatat</small></span><div class="skill-meter" aria-label="Akurasi ${item.accuracy}%"><i class="skill-meter-fill ${accuracyClass(item.accuracy)}"></i></div><strong>${item.accuracy}%</strong></div>`).join("") : '<p class="empty-copy">Selesaikan latihan untuk melihat peta skill.</p>';
  } catch (error) {
    metrics.innerHTML = `<p class="empty-copy">${escapeHTML(error.message)}</p>`;
  }
}

$("#refresh-dashboard").addEventListener("click", loadDashboard);

async function loadReviews() {
  const list = $("#review-due-list");
  const button = $("#start-due-review");
  list.innerHTML = '<p class="empty-copy">Memuat jadwal review…</p>';
  try {
    const items = await api("/api/reviews");
    button.disabled = items.length === 0;
    button.textContent = items.length ? `Mulai ${items.length} soal` : "Belum ada jadwal";
    list.innerHTML = items.length ? items.map((item) => `
      <article class="due-row"><span><strong>${escapeHTML(typeNames[item.question.type] || item.question.type)}</strong><small>Penguasaan ${item.mastery}/5</small></span><p>${escapeHTML(learnerPrompt(item.question.prompt))}</p></article>`).join("") : '<p class="empty-copy">Belum ada review jatuh tempo. Soal akan muncul kembali sesuai jarak belajar Anda.</p>';
  } catch (error) {
    button.disabled = true;
    list.innerHTML = `<p class="empty-copy">${escapeHTML(error.message)}</p>`;
  }
}

async function beginGeneratedSession(path, options = {}) {
  const data = await api(path, { method: "POST", ...options });
  state.session = data.session;
  state.review = [];
  state.index = 0;
  if (data.notice) showToast(data.notice, 2000);
  startQuiz();
}

$("#start-due-review").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.dataset.state = "loading";
  try { await beginGeneratedSession("/api/reviews/session"); }
  catch (error) { button.dataset.state = "error"; showToast(error.message); }
  finally { button.disabled = false; if (button.dataset.state !== "error") delete button.dataset.state; }
});

async function loadVocabulary() {
  const list = $("#vocab-list");
  list.innerHTML = '<p class="empty-copy">Memuat notebook…</p>';
  try {
    const items = await api("/api/vocabulary");
    list.innerHTML = items.length ? items.map((item) => {
      const due = new Date(item.nextReviewAt) <= new Date();
      return `<article class="vocab-card" data-vocab-id="${item.id}">
        <header><div><h3>${escapeHTML(item.word)}</h3>${item.collocation ? `<span>${escapeHTML(item.collocation)}</span>` : ""}</div><small>${due ? "Perlu diulang" : `Penguasaan ${item.mastery}/5`}</small></header>
        <p>${escapeHTML(item.meaning)}</p><blockquote lang="en">${escapeHTML(item.example)}</blockquote>
        ${due ? '<div class="vocab-actions"><button class="btn btn--soft" type="button" data-remembered="false">Ulang besok</button><button class="btn btn--pear" type="button" data-remembered="true">Sudah ingat</button></div>' : ""}
      </article>`;
    }).join("") : '<p class="empty-copy">Belum ada kosakata. Simpan kata bersama arti dan contoh penggunaannya.</p>';
    $$('[data-vocab-id] [data-remembered]', list).forEach((button) => button.addEventListener("click", reviewVocabulary));
  } catch (error) {
    list.innerHTML = `<p class="empty-copy">${escapeHTML(error.message)}</p>`;
  }
}

$("#vocab-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const button = $('button[type="submit"]', form);
  const values = Object.fromEntries(new FormData(form));
  button.disabled = true;
  button.dataset.state = "loading";
  $("#vocab-status").textContent = "Menyimpan kata…";
  try {
    await api("/api/vocabulary", { method: "POST", body: JSON.stringify(values) });
    form.reset();
    $("#vocab-status").textContent = "Kata tersimpan dan siap direview.";
    await loadVocabulary();
  } catch (error) {
    button.dataset.state = "error";
    $("#vocab-status").textContent = error.message;
  } finally {
    button.disabled = false;
    if (button.dataset.state !== "error") delete button.dataset.state;
  }
});

async function reviewVocabulary(event) {
  const button = event.currentTarget;
  const card = button.closest("[data-vocab-id]");
  button.disabled = true;
  try {
    await api(`/api/vocabulary/${card.dataset.vocabId}/review`, { method: "PUT", body: JSON.stringify({ remembered: button.dataset.remembered === "true" }) });
    await loadVocabulary();
  } catch (error) { showToast(error.message); button.disabled = false; }
}

function countWords(text) {
  const trimmed = String(text || "").trim();
  return trimmed ? trimmed.split(/\s+/).length : 0;
}

function countParagraphs(text) {
  const trimmed = String(text || "").trim();
  if (!trimmed) return 0;
  return trimmed.split(/\n+/).filter((p) => p.trim().length > 0).length;
}

function updateWritingMetrics() {
  const text = $("#writing-response") ? $("#writing-response").value : "";
  const words = countWords(text);
  const paragraphs = countParagraphs(text);
  const taskType = $("#writing-type") ? $("#writing-type").value : "task2";
  const targetWords = taskType === "task1" ? 150 : 250;
  const progressPct = Math.min(100, Math.round((words / targetWords) * 100));

  if ($("#writing-count")) $("#writing-count").textContent = `${words} kata`;
  if ($("#writing-count-sub")) {
    $("#writing-count-sub").textContent = taskType === "task1"
      ? "Disarankan 150–200 kata untuk Task 1"
      : "Disarankan 250–350 kata untuk Task 2";
  }
  if ($("#writing-target-label")) $("#writing-target-label").textContent = `Target: Min ${targetWords} kata`;
  if ($("#writing-stat-words")) $("#writing-stat-words").innerHTML = `<strong>${words}</strong> kata`;
  if ($("#writing-stat-paragraphs")) $("#writing-stat-paragraphs").innerHTML = `<strong>${paragraphs}</strong> paragraf`;

  const goalEl = $("#writing-stat-goal");
  if (goalEl) {
    if (words >= targetWords) {
      goalEl.classList.add("is-met");
      goalEl.textContent = `✓ Target ${targetWords} kata tercapai`;
    } else {
      goalEl.classList.remove("is-met");
      goalEl.textContent = `Kurang ${targetWords - words} kata`;
    }
  }

  const barEl = $("#writing-progress-bar");
  if (barEl) {
    barEl.style.width = `${progressPct}%`;
    if (words >= targetWords) {
      barEl.classList.add("is-completed");
    } else {
      barEl.classList.remove("is-completed");
    }
  }
}

async function loadWritingPrompt() {
  const taskType = $("#writing-type").value;
  updateWritingMetrics();
  $("#writing-prompt").textContent = "Memuat prompt…";
  try {
    const data = await api(`/api/writing/prompt?type=${encodeURIComponent(taskType)}`);
    learningState.prompt = data.prompt;
    $("#writing-prompt").textContent = data.prompt;
  } catch (error) {
    learningState.prompt = "";
    $("#writing-prompt").textContent = error.message;
  }
}

function readWritingDraft() {
  try { return JSON.parse(localStorage.getItem(writingDraftKey) || "null"); } catch { return null; }
}

function saveWritingDraft(showStatus = true) {
  const draft = { taskType: $("#writing-type").value, prompt: learningState.prompt, response: $("#writing-response").value, updatedAt: new Date().toISOString() };
  localStorage.setItem(writingDraftKey, JSON.stringify(draft));
  if (showStatus) $("#writing-status").textContent = "Draft tersimpan di perangkat ini.";
}

function restoreWritingDraft() {
  const draft = readWritingDraft();
  if (!draft || !draft.response) {
    updateWritingMetrics();
    return;
  }
  $("#writing-response").value = draft.response;
  if (draft.taskType && $("#writing-type")) {
    $("#writing-type").value = draft.taskType;
  }
  updateWritingMetrics();
}

async function loadWritingHistory() {
  const list = $("#writing-history-list");
  if (!list) return;
  list.innerHTML = '<p class="empty-copy">Memuat riwayat writing…</p>';
  try {
    const items = await api("/api/writing");
    list.innerHTML = items.length ? items.map((item) => `<article class="writing-history-row"><div><span class="stage-label">${escapeHTML(item.taskType)} · ${escapeHTML(new Intl.DateTimeFormat("id-ID", { day: "numeric", month: "short" }).format(new Date(item.createdAt)))}</span><strong>Band ${item.feedback.overallBand}</strong><small>${item.wordCount} kata · ${item.source === "demo" ? "mode demo" : "dinilai AI"}</small></div><button class="btn btn--soft" type="button" data-writing-id="${item.id}">Buka</button></article>`).join("") : '<p class="empty-copy">Belum ada tulisan tersimpan.</p>';
    $$('[data-writing-id]', list).forEach((button, index) => button.addEventListener("click", () => {
      const item = items[index];
      $("#writing-type").value = item.taskType;
      learningState.prompt = item.prompt;
      $("#writing-prompt").textContent = item.prompt;
      $("#writing-response").value = item.response;
      updateWritingMetrics();
      $("#writing-response").focus();
    }));
  } catch (error) { list.innerHTML = `<p class="empty-copy">${escapeHTML(error.message)}</p>`; }
}

$("#writing-type").addEventListener("change", loadWritingPrompt);
$("#writing-type").addEventListener("change", () => saveWritingDraft(false));
$("#writing-response").addEventListener("input", () => {
  updateWritingMetrics();
  saveWritingDraft(false);
});
$("#writing-response").addEventListener("keydown", (e) => {
  if (e.key === "Tab") {
    e.preventDefault();
    const start = e.target.selectionStart;
    const end = e.target.selectionEnd;
    e.target.value = e.target.value.substring(0, start) + "  " + e.target.value.substring(end);
    e.target.selectionStart = e.target.selectionEnd = start + 2;
    updateWritingMetrics();
    saveWritingDraft(false);
  }
});
$("#save-writing-draft").addEventListener("click", () => saveWritingDraft(true));
restoreWritingDraft();

$("#writing-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = $('button[type="submit"]', event.currentTarget);
  button.disabled = true;
  button.dataset.state = "loading";
  $("#writing-status").textContent = "Menilai struktur, kosakata, dan grammar…";
  try {
    const data = await api("/api/writing", { method: "POST", body: JSON.stringify({ taskType: $("#writing-type").value, prompt: learningState.prompt, response: $("#writing-response").value }) });
    renderWritingFeedback(data);
    localStorage.removeItem(writingDraftKey);
    $("#writing-status").textContent = data.source === "demo" ? "Mode demo aktif; estimasi terbatas pada struktur dasar." : "Feedback tersimpan.";
  } catch (error) {
    button.dataset.state = "error";
    $("#writing-status").textContent = error.message;
  } finally {
    button.disabled = false;
    if (button.dataset.state !== "error") delete button.dataset.state;
  }
});
function accuracyClass(value) { return `meter-${Math.max(1, Math.min(10, Math.round(Number(value || 0) / 10)))}`; }

if ($("#listen-play")) {
const listeningTracks = [
  ["A quieter city centre", "detail dan angka", "The city centre introduced a traffic trial for six weeks. Cars were limited on the main street, while buses and bicycles were given more space. The first survey showed that more people walked to work.", "What changed after the traffic trial?", ["Bus routes became longer", "More people walked to work", "Shops closed earlier"], 1],
  ["A better morning routine", "sequence", "Maya prepares her bag the night before. She leaves home at seven thirty, catches the second bus, and usually reaches the office ten minutes early.", "When does Maya leave home?", ["Seven o'clock", "Seven thirty", "Eight o'clock"], 1],
  ["The community repair café", "purpose", "The repair café opens on the first Saturday of every month. Volunteers fix small appliances and teach visitors how to maintain them instead of replacing them.", "What do volunteers teach?", ["How to sell appliances", "How to maintain appliances", "How to build a café"], 1],
  ["A change to the library", "location", "The library moved its language books to the second floor. The ground floor now has study desks and a small exhibition about local history.", "Where are the language books now?", ["On the ground floor", "In the exhibition", "On the second floor"], 2],
  ["Rainwater in schools", "main idea", "Two schools installed tanks to collect rainwater. The water is used for gardens, while drinking water continues to come from the public supply.", "What is the collected water used for?", ["Gardens", "Drinking", "Cleaning buses"], 0],
  ["The evening course", "schedule", "The evening course runs on Tuesdays and Thursdays. Learners can join online, but the final presentation must be delivered in person.", "When is the final presentation?", ["Online only", "In person", "On Saturday"], 1],
  ["A local food market", "comparison", "The market used to open every day, but rising costs led organisers to reduce it to three days a week. Saturday remains the busiest day.", "Which day is busiest?", ["Monday", "Wednesday", "Saturday"], 2],
  ["Bikes at the station", "specific detail", "The railway station added covered bicycle parking beside platform four. Users need a free digital pass to enter the area.", "What is needed to enter?", ["A paper ticket", "A digital pass", "A parking fee"], 1],
  ["Learning through retrieval", "contrast", "Rereading can feel easy because the answer is visible. Retrieval feels harder because the learner must produce the answer, but that effort usually strengthens memory.", "Why can retrieval feel harder?", ["The answer is hidden", "The text is longer", "The learner must produce the answer"], 2],
  ["A new bus timetable", "change", "The new timetable adds an early service at 5:45. The late service remains unchanged, so night-shift workers can still use it.", "What was added?", ["An early service", "A late service", "A new train"], 0],
  ["The rooftop garden", "reason", "Residents created a rooftop garden to cool the building in summer. They grow herbs in lightweight containers and share the harvest.", "Why was the garden created?", ["To cool the building", "To sell containers", "To build a new roof"], 0],
  ["A museum audio guide", "instruction", "Visitors can borrow an audio guide at the entrance. They should return it before leaving, even if they have not finished every recording.", "Where can visitors borrow the guide?", ["At the entrance", "In the café", "Outside the museum"], 0],
  ["The study group", "frequency", "The study group meets fortnightly, not weekly. Members share one writing task before each meeting and discuss two common errors.", "How often does the group meet?", ["Every week", "Every two weeks", "Every month"], 1],
  ["A quieter hospital", "result", "After quiet hours were introduced, patients reported sleeping longer. Staff also noticed fewer interruptions during medication rounds.", "What did patients report?", ["Longer sleep", "More visitors", "Shorter rounds"], 0],
  ["The school garden project", "people and roles", "Students grow vegetables in the school garden. Local farmers visit twice a term to demonstrate composting and answer questions.", "Who demonstrates composting?", ["Students", "Local farmers", "Parents"], 1],
];
let listeningIndex = 0;
let listeningUtterance = null;
function renderListeningTrack() {
  const track = listeningTracks[listeningIndex];
  $("#listening-label").textContent = `TRACK ${String(listeningIndex + 1).padStart(2, "0")} / 15 · B1`;
  $("#listening-title").textContent = track[0]; $("#listening-meta").textContent = `Durasi pendek · fokus: ${track[1]}`;
  $("#listening-question-title").textContent = track[3]; $("#listening-choices").innerHTML = track[4].map((choice, i) => `<label><input type="radio" name="listening-answer" value="${i}"><span>${escapeHTML(choice)}</span></label>`).join("");
  $("#listening-track-select").value = String(listeningIndex);
  $("#listening-transcript").textContent = track[2]; $("#listening-transcript").hidden = true; $("#listen-transcript-toggle").textContent = "Buka transcript setelah menjawab"; $("#listen-transcript-toggle").setAttribute("aria-expanded", "false"); $("#listening-status").textContent = ""; $("#listening-progress").textContent = "Siap diputar.";
  $$('input[name="listening-answer"]').forEach((input) => input.addEventListener("change", (event) => { $("#listening-status").textContent = Number(event.target.value) === track[5] ? "Benar. Dengarkan kembali untuk menangkap detailnya." : "Belum tepat. Dengarkan sekali lagi dan cari bukti spesifik."; }));
}
function stopListening() { if ("speechSynthesis" in window) speechSynthesis.cancel(); learningState.listening = false; $("#audio-visualizer").classList.remove("is-playing"); $("#listen-play").textContent = "▶ Putar audio"; }
$("#listen-play").addEventListener("click", () => {
  if (!("speechSynthesis" in window)) { $("#listening-status").textContent = "Browser ini belum mendukung audio listening."; return; }
  if (speechSynthesis.speaking && !speechSynthesis.paused) { speechSynthesis.pause(); learningState.listening = false; $("#audio-visualizer").classList.remove("is-playing"); $("#listen-play").textContent = "▶ Lanjutkan"; $("#listening-progress").textContent = "Audio dijeda."; return; }
  if (speechSynthesis.paused && listeningUtterance) { speechSynthesis.resume(); learningState.listening = true; $("#audio-visualizer").classList.add("is-playing"); $("#listen-play").textContent = "Ⅱ Pause"; $("#listening-progress").textContent = "Audio sedang diputar…"; return; }
  stopListening(); const track = listeningTracks[listeningIndex]; listeningUtterance = new SpeechSynthesisUtterance(track[2]); listeningUtterance.lang = "en-GB"; listeningUtterance.rate = .86; listeningUtterance.onstart = () => { learningState.listening = true; $("#audio-visualizer").classList.add("is-playing"); $("#listen-play").textContent = "Ⅱ Pause"; $("#listening-progress").textContent = "Audio sedang diputar…"; }; listeningUtterance.onend = () => { learningState.listening = false; $("#audio-visualizer").classList.remove("is-playing"); $("#listen-play").textContent = "↻ Putar lagi"; $("#listening-progress").textContent = "Selesai. Pilih jawaban Anda."; }; speechSynthesis.speak(listeningUtterance);
});
$("#listen-stop").addEventListener("click", () => { stopListening(); $("#listening-progress").textContent = "Audio dihentikan."; });
$("#listening-track-select").innerHTML = listeningTracks.map((track, index) => `<option value="${index}">Track ${String(index + 1).padStart(2, "0")} · ${escapeHTML(track[0])}</option>`).join("");
$("#listening-track-select").addEventListener("change", (event) => { stopListening(); listeningIndex = Number(event.target.value); renderListeningTrack(); });
$("#listen-transcript-toggle").addEventListener("click", (event) => { const transcript = $("#listening-transcript"); const open = transcript.hidden; transcript.hidden = !open; event.currentTarget.setAttribute("aria-expanded", String(open)); event.currentTarget.textContent = open ? "Tutup transcript" : "Buka transcript setelah menjawab"; });
renderListeningTrack();
}
$$('.reading-open').forEach((button) => button.addEventListener("click", (event) => { const body = event.currentTarget.parentElement.querySelector(".reading-body"); body.hidden = !body.hidden; event.currentTarget.textContent = body.hidden ? "Baca artikel" : "Tutup artikel"; }));

function renderWritingFeedback(data) {
  const feedback = data.feedback;
  const container = $("#writing-feedback");
  const scores = [["Task response", feedback.taskResponse], ["Coherence", feedback.coherence], ["Lexical resource", feedback.lexicalResource], ["Grammar", feedback.grammar]];
  container.hidden = false;
  container.innerHTML = `<div class="feedback-score"><span>Perkiraan band</span><strong>${feedback.overallBand}</strong><small>${data.wordCount} kata</small></div>
    <div class="feedback-body"><p>${escapeHTML(feedback.summary)}</p><div class="rubric-list">${scores.map(([label, value]) => `<span>${escapeHTML(label)}<strong>${value}</strong></span>`).join("")}</div>
    <h3>Langkah berikutnya</h3><ul>${feedback.nextSteps.map((item) => `<li>${escapeHTML(item)}</li>`).join("")}</ul>
    <h3>Koreksi yang perlu diperiksa</h3><ul>${feedback.corrections.map((item) => `<li>${escapeHTML(item)}</li>`).join("")}</ul></div>`;
}

$("#start-diagnostic").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.dataset.state = "loading";
  try { await beginGeneratedSession("/api/diagnostic"); }
  catch (error) { button.dataset.state = "error"; showToast(error.message); }
  finally { button.disabled = false; if (button.dataset.state !== "error") delete button.dataset.state; }
});

const startReadingButton = $("#start-reading");
if (startReadingButton) startReadingButton.addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.dataset.state = "loading";
  try {
    await beginGeneratedSession("/api/sessions", { body: JSON.stringify({ level: "B1", ieltsTarget: 6.5, count: 10, durationMinutes: 20, types: ["reading"] }) });
  } catch (error) { button.dataset.state = "error"; showToast(error.message); }
  finally { button.disabled = false; if (button.dataset.state !== "error") delete button.dataset.state; }
});
