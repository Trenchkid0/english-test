/* Hallmark · extension: IELTS Mock Test Engine & Simulation Controller */

const mockState = {
  test: null,
  currentSection: "listening",
  currentIndex: 0,
  answers: {
    listening: {},
    reading: {},
    writing: { task1: "", task2: "" }
  },
  flagged: new Set(),
  timeRemaining: 9600,
  timerInterval: null,
  autosaveInterval: null,
  audioPlayCount: {},
  currentAudioPlayback: null
};

function formatDuration(seconds) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

// -------------------------------------------------------------
// Voice Synthesis Engine with Multi-Speaker Support
// -------------------------------------------------------------
function parseDialogueTurns(text = "") {
  const lines = text.split("\n").map(l => l.trim()).filter(Boolean);
  const turns = [];
  const speakerRegex = /^([A-Za-z0-9\s]+):\s*(.+)$/i;

  for (const line of lines) {
    const match = line.match(speakerRegex);
    if (match) {
      turns.push({ speaker: match[1].trim(), text: match[2].trim() });
    } else {
      if (turns.length > 0 && !turns[turns.length - 1].speaker.startsWith("[")) {
        turns[turns.length - 1].text += " " + line;
      } else {
        turns.push({ speaker: "Narrator", text: line });
      }
    }
  }
  return turns.length > 0 ? turns : [{ speaker: "Speaker 1", text }];
}

function getSynthesizedVoices() {
  if (!("speechSynthesis" in window)) return [];
  const voices = window.speechSynthesis.getVoices();
  return voices.filter(v => v.lang.startsWith("en"));
}

function speakDialogue(text, onStart, onEnd, onError) {
  if (!("speechSynthesis" in window)) {
    if (onError) onError("Browser tidak mendukung sintesis suara.");
    return null;
  }
  window.speechSynthesis.cancel();

  const turns = parseDialogueTurns(text);
  const voices = getSynthesizedVoices();
  const gbVoice = voices.find(v => v.lang.includes("GB") || v.lang.includes("en_GB")) || voices[0];
  const auVoice = voices.find(v => v.lang.includes("AU") || v.lang.includes("US")) || voices[1] || gbVoice;

  let turnIndex = 0;
  let isCancelled = false;

  function speakNextTurn() {
    if (isCancelled || turnIndex >= turns.length) {
      if (!isCancelled && onEnd) onEnd();
      return;
    }
    const turn = turns[turnIndex];
    const utterance = new SpeechSynthesisUtterance(turn.text);

    // Differentiate voice and pitch per speaker
    const isFirstSpeaker = turn.speaker.toLowerCase().includes("1") || turn.speaker.toLowerCase().includes("woman") || turn.speaker.toLowerCase().includes("receptionist");
    if (isFirstSpeaker) {
      utterance.voice = gbVoice;
      utterance.lang = "en-GB";
      utterance.pitch = 1.05;
      utterance.rate = 0.88;
    } else {
      utterance.voice = auVoice;
      utterance.lang = "en-US";
      utterance.pitch = 0.95;
      utterance.rate = 0.86;
    }

    if (turnIndex === 0 && onStart) onStart();

    utterance.onend = () => {
      turnIndex++;
      setTimeout(speakNextTurn, 250);
    };
    utterance.onerror = (e) => {
      if (onError) onError(e.error);
    };

    window.speechSynthesis.speak(utterance);
  }

  speakNextTurn();

  return {
    stop: () => {
      isCancelled = true;
      window.speechSynthesis.cancel();
    }
  };
}

// -------------------------------------------------------------
// Mock Lobby & Navigation
// -------------------------------------------------------------
async function openMockLobby() {
  clearInterval(state.timerID);
  clearInterval(mockState.timerInterval);
  clearInterval(mockState.autosaveInterval);
  if (mockState.currentAudioPlayback) {
    mockState.currentAudioPlayback.stop();
    mockState.currentAudioPlayback = null;
  }

  showScreen("mock");
  $("#mock-lobby").hidden = false;
  $("#mock-exam-workspace").hidden = true;
  $("#mock-result-workspace").hidden = true;

  loadMockHistory();
}

async function loadMockHistory() {
  const listEl = $("#mock-history-list");
  if (!listEl) return;
  listEl.innerHTML = '<p class="empty-copy">Memuat riwayat simulasi…</p>';

  try {
    const data = await api("/api/mock-tests");
    const tests = data.mockTests || [];
    if (tests.length === 0) {
      listEl.innerHTML = '<p class="empty-copy">Belum ada tes simulasi yang dikerjakan. Mulai tes pertama Anda!</p>';
      return;
    }

    listEl.innerHTML = tests.map(t => {
      const dateStr = new Intl.DateTimeFormat("id-ID", { day: "numeric", month: "short", year: "numeric" }).format(new Date(t.createdAt));
      const isDone = t.status === "completed";
      const bandBadge = isDone && t.overallBand ? `<strong class="band-badge">Band ${t.overallBand.toFixed(1)}</strong>` : `<span class="badge-in-progress">In Progress</span>`;
      return `
        <article class="mock-history-row">
          <div class="mock-row-info">
            <span class="stage-label">${escapeHTML(t.moduleType.toUpperCase())} · ${dateStr}</span>
            <h4>${escapeHTML(t.title)}</h4>
            <div class="row-chips">
              ${bandBadge}
              <small>${isDone ? "Selesai diuji" : "Belum selesai"}</small>
            </div>
          </div>
          <button class="btn btn--soft" type="button" data-mock-id="${t.id}" data-mock-status="${t.status}">
            ${isDone ? "Buka Laporan" : "Lanjutkan"}
          </button>
        </article>
      `;
    }).join("");

    $$('[data-mock-id]', listEl).forEach(btn => {
      btn.addEventListener("click", () => resumeOrOpenMock(btn.dataset.mockId, btn.dataset.mockStatus));
    });
  } catch (err) {
    listEl.innerHTML = `<p class="empty-copy">${escapeHTML(err.message)}</p>`;
  }
}

async function resumeOrOpenMock(id, status) {
  try {
    const testData = await api(`/api/mock-tests/${encodeURIComponent(id)}`);
    if (status === "completed") {
      renderMockResults(testData);
    } else {
      initializeMockExam(testData);
    }
  } catch (err) {
    showToast(err.message);
  }
}

// -------------------------------------------------------------
// Mock Start Form & Lifecycle
// -------------------------------------------------------------
$("#mock-start-form")?.addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.currentTarget;
  const title = $("#mock-title").value.trim() || "IELTS Academic Practice Test";
  const moduleType = $("#mock-module").value;
  const fullLength = $("#mock-length").value === "true";

  const btn = $("#start-mock-btn");
  btn.disabled = true;
  btn.dataset.state = "loading";

  try {
    const testData = await api("/api/mock-tests", {
      method: "POST",
      body: JSON.stringify({ title, moduleType, fullLength })
    });
    initializeMockExam(testData);
  } catch (err) {
    btn.dataset.state = "error";
    showToast(err.message);
  } finally {
    btn.disabled = false;
    if (btn.dataset.state !== "error") delete btn.dataset.state;
  }
});

function initializeMockExam(testData) {
  mockState.test = testData;
  mockState.currentSection = testData.currentSection || "listening";
  mockState.currentIndex = 0;
  mockState.answers = testData.answers || {
    listening: {},
    reading: {},
    writing: { task1: "", task2: "" }
  };
  mockState.flagged = new Set();
  mockState.timeRemaining = testData.timeRemainingSeconds || (testData.config.reduce((acc, c) => acc + c.timeLimitMinutes, 0) * 60);

  $("#mock-lobby").hidden = true;
  $("#mock-result-workspace").hidden = true;
  $("#mock-exam-workspace").hidden = false;

  // Setup Section Tabs
  updateSectionTabs();

  // Setup Autosave Interval (every 15s)
  clearInterval(mockState.autosaveInterval);
  mockState.autosaveInterval = setInterval(triggerMockAutosave, 15000);

  // Setup Timer Interval
  clearInterval(mockState.timerInterval);
  mockState.timerInterval = setInterval(handleMockTimerTick, 1000);
  updateTimerUI();

  // Render first section & question
  switchSection(mockState.currentSection);
}

function handleMockTimerTick() {
  if (mockState.timeRemaining > 0) {
    mockState.timeRemaining--;
    updateTimerUI();
  } else {
    clearInterval(mockState.timerInterval);
    showToast("Waktu ujian telah berakhir. Mengumpulkan jawaban otomatis...");
    finishMockExam();
  }
}

function updateTimerUI() {
  const display = $("#mock-time-display");
  if (display) {
    display.textContent = formatDuration(mockState.timeRemaining);
    if (mockState.timeRemaining < 300) {
      display.parentElement.classList.add("is-warning");
    } else {
      display.parentElement.classList.remove("is-warning");
    }
  }
}

async function triggerMockAutosave() {
  if (!mockState.test || !mockState.test.id) return;
  const tag = $("#mock-autosave-tag");
  if (tag) tag.textContent = "○ Menyimpan…";

  // Capture current writing responses if in writing
  if (mockState.currentSection === "writing") {
    const t1 = $("#mock-t1-response")?.value || "";
    const t2 = $("#mock-t2-response")?.value || "";
    mockState.answers.writing.task1 = t1;
    mockState.answers.writing.task2 = t2;
  }

  try {
    await api(`/api/mock-tests/${encodeURIComponent(mockState.test.id)}/autosave`, {
      method: "POST",
      body: JSON.stringify({
        currentSection: mockState.currentSection,
        timeRemainingSeconds: mockState.timeRemaining,
        answers: mockState.answers
      })
    });
    if (tag) tag.textContent = "● Tersimpan";
  } catch (err) {
    if (tag) tag.textContent = "✕ Gagal sync";
  }
}

function updateSectionTabs() {
  $$(".mock-sec-tab").forEach(tab => {
    const sec = tab.dataset.sec;
    const active = sec === mockState.currentSection;
    tab.classList.toggle("is-active", active);
  });
}

function switchSection(sectionName) {
  // Save current writing inputs before switching
  if (mockState.currentSection === "writing") {
    mockState.answers.writing.task1 = $("#mock-t1-response")?.value || "";
    mockState.answers.writing.task2 = $("#mock-t2-response")?.value || "";
  }

  mockState.currentSection = sectionName;
  mockState.currentIndex = 0;
  updateSectionTabs();

  $("#mock-listening-pane").hidden = sectionName !== "listening";
  $("#mock-reading-pane").hidden = sectionName !== "reading";
  $("#mock-writing-pane").hidden = sectionName !== "writing";

  renderQuestionGrid();
  renderCurrentQuestion();
}

$$(".mock-sec-tab").forEach(tab => {
  tab.addEventListener("click", () => {
    switchSection(tab.dataset.sec);
  });
});

// -------------------------------------------------------------
// Question Navigator Grid & Flagging
// -------------------------------------------------------------
function renderQuestionGrid() {
  const container = $("#mock-grid-buttons");
  if (!container) return;
  container.innerHTML = "";

  const sec = mockState.currentSection;
  let count = 0;
  if (sec === "listening") count = (mockState.test.questions.listening || []).length;
  else if (sec === "reading") count = (mockState.test.questions.reading || []).length;
  else if (sec === "writing") count = 2; // Task 1 and Task 2

  for (let i = 0; i < count; i++) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "mock-grid-pill";
    btn.textContent = String(i + 1);

    const isCurrent = i === mockState.currentIndex;
    const isFlagged = mockState.flagged.has(`${sec}_${i}`);

    let isAnswered = false;
    if (sec === "listening") {
      isAnswered = mockState.answers.listening[String(i)] !== undefined;
    } else if (sec === "reading") {
      isAnswered = mockState.answers.reading[String(i)] !== undefined;
    } else if (sec === "writing") {
      if (i === 0) isAnswered = (mockState.answers.writing.task1 || "").trim().length > 30;
      if (i === 1) isAnswered = (mockState.answers.writing.task2 || "").trim().length > 30;
    }

    if (isCurrent) btn.classList.add("is-current");
    if (isAnswered) btn.classList.add("is-answered");
    if (isFlagged) btn.classList.add("is-flagged");

    btn.addEventListener("click", () => {
      mockState.currentIndex = i;
      renderQuestionGrid();
      renderCurrentQuestion();
    });

    container.appendChild(btn);
  }

  // Update Flag Button state
  const flagKey = `${sec}_${mockState.currentIndex}`;
  const isFlagged = mockState.flagged.has(flagKey);
  $("#mock-flag-btn")?.classList.toggle("is-active", isFlagged);
  if ($("#mock-flag-text")) {
    $("#mock-flag-text").textContent = isFlagged ? "Tandai Ragu (Aktif)" : "Tandai Ragu";
  }

  // Update Footer Label
  if ($("#mock-current-label")) {
    $("#mock-current-label").textContent = `Soal ${mockState.currentIndex + 1} dari ${count}`;
  }
}

$("#mock-flag-btn")?.addEventListener("click", () => {
  const key = `${mockState.currentSection}_${mockState.currentIndex}`;
  if (mockState.flagged.has(key)) {
    mockState.flagged.delete(key);
  } else {
    mockState.flagged.add(key);
  }
  renderQuestionGrid();
});

// -------------------------------------------------------------
// Render Question Pane (Listening, Reading, Writing)
// -------------------------------------------------------------
function renderCurrentQuestion() {
  const sec = mockState.currentSection;
  const idx = mockState.currentIndex;

  if (sec === "listening") {
    renderListeningQuestion(idx);
  } else if (sec === "reading") {
    renderReadingQuestion(idx);
  } else if (sec === "writing") {
    renderWritingQuestion(idx);
  }
}

function renderListeningQuestion(idx) {
  const qList = mockState.test.questions.listening || [];
  const q = qList[idx];
  if (!q) return;

  const contentEl = $("#mock-listening-question-content");
  const audioTitleEl = $("#mock-audio-title");
  const playsLeftEl = $("#mock-plays-left");

  if (audioTitleEl) audioTitleEl.textContent = `Listening Audio Track #${idx + 1}`;

  // Check play counts
  if (mockState.audioPlayCount[idx] === undefined) {
    mockState.audioPlayCount[idx] = 2; // Allowed 2 plays in practice simulation
  }
  const playsLeft = mockState.audioPlayCount[idx];
  if (playsLeftEl) playsLeftEl.textContent = `Sisa Putar: ${playsLeft}x`;

  const selectedAnswer = mockState.answers.listening[String(idx)];

  contentEl.innerHTML = `
    <div class="mock-q-card">
      <span class="q-kicker">PERTANYAAN ${idx + 1} / ${qList.length}</span>
      <h3 class="q-prompt">${escapeHTML(learnerPrompt(q.prompt))}</h3>
      <div class="mock-options-list">
        ${q.choices.map((choice, cIdx) => `
          <label class="mock-option-label ${selectedAnswer === cIdx ? "is-selected" : ""}">
            <input type="radio" name="mock_listening_ans" value="${cIdx}" ${selectedAnswer === cIdx ? "checked" : ""}>
            <span class="option-num">${String.fromCharCode(65 + cIdx)}</span>
            <span class="option-text">${escapeHTML(choice)}</span>
          </label>
        `).join("")}
      </div>
    </div>
  `;

  $$('input[name="mock_listening_ans"]', contentEl).forEach(input => {
    input.addEventListener("change", (e) => {
      mockState.answers.listening[String(idx)] = Number(e.target.value);
      renderQuestionGrid();
      renderListeningQuestion(idx);
    });
  });

  // Setup Audio play button for current question
  const playBtn = $("#mock-audio-play-btn");
  const stopBtn = $("#mock-audio-stop-btn");
  const statusEl = $("#mock-audio-status");
  const vizEl = $("#mock-audio-visualizer");

  playBtn.onclick = () => {
    if (mockState.audioPlayCount[idx] <= 0) {
      showToast("Batas pemutaran audio untuk soal ini telah habis (0x).");
      return;
    }
    if (mockState.currentAudioPlayback) {
      mockState.currentAudioPlayback.stop();
      mockState.currentAudioPlayback = null;
    }

    mockState.audioPlayCount[idx]--;
    if (playsLeftEl) playsLeftEl.textContent = `Sisa Putar: ${mockState.audioPlayCount[idx]}x`;

    statusEl.textContent = "Audio sedang diputar...";
    vizEl.classList.add("is-playing");
    playBtn.disabled = true;

    mockState.currentAudioPlayback = speakDialogue(
      q.context || q.prompt,
      () => {
        statusEl.textContent = "Audio sedang diputar...";
        vizEl.classList.add("is-playing");
      },
      () => {
        statusEl.textContent = "Audio selesai. Silakan pilih jawaban Anda.";
        vizEl.classList.remove("is-playing");
        playBtn.disabled = false;
        mockState.currentAudioPlayback = null;
      },
      (err) => {
        statusEl.textContent = "Error: " + err;
        vizEl.classList.remove("is-playing");
        playBtn.disabled = false;
        mockState.currentAudioPlayback = null;
      }
    );
  };

  stopBtn.onclick = () => {
    if (mockState.currentAudioPlayback) {
      mockState.currentAudioPlayback.stop();
      mockState.currentAudioPlayback = null;
      statusEl.textContent = "Audio dihentikan.";
      vizEl.classList.remove("is-playing");
      playBtn.disabled = false;
    }
  };
}

function renderReadingQuestion(idx) {
  const qList = mockState.test.questions.reading || [];
  const q = qList[idx];
  if (!q) return;

  const passageTagEl = $("#mock-reading-passage-tag");
  const passageTitleEl = $("#mock-reading-passage-title");
  const passageBodyEl = $("#mock-reading-passage-body");
  const contentEl = $("#mock-reading-question-content");

  if (passageTagEl) passageTagEl.textContent = `READING PASSAGE · QUESTION ${idx + 1}`;
  if (passageTitleEl) passageTitleEl.textContent = `Passage Section`;
  if (passageBodyEl) {
    const paragraphs = (q.context || "No passage context provided.").split("\n\n").filter(Boolean);
    passageBodyEl.innerHTML = paragraphs.map(p => `<p>${escapeHTML(p)}</p>`).join("");
  }

  const selectedAnswer = mockState.answers.reading[String(idx)];

  contentEl.innerHTML = `
    <div class="mock-q-card">
      <span class="q-kicker">PERTANYAAN ${idx + 1} / ${qList.length}</span>
      <h3 class="q-prompt">${escapeHTML(learnerPrompt(q.prompt))}</h3>
      <div class="mock-options-list">
        ${q.choices.map((choice, cIdx) => `
          <label class="mock-option-label ${selectedAnswer === cIdx ? "is-selected" : ""}">
            <input type="radio" name="mock_reading_ans" value="${cIdx}" ${selectedAnswer === cIdx ? "checked" : ""}>
            <span class="option-num">${String.fromCharCode(65 + cIdx)}</span>
            <span class="option-text">${escapeHTML(choice)}</span>
          </label>
        `).join("")}
      </div>
    </div>
  `;

  $$('input[name="mock_reading_ans"]', contentEl).forEach(input => {
    input.addEventListener("change", (e) => {
      mockState.answers.reading[String(idx)] = Number(e.target.value);
      renderQuestionGrid();
      renderReadingQuestion(idx);
    });
  });
}

function renderWritingQuestion(idx) {
  const isT1 = idx === 0;
  $("#mock-writing-t1-btn").classList.toggle("is-active", isT1);
  $("#mock-writing-t2-btn").classList.toggle("is-active", !isT1);
  $("#mock-writing-t1-subpane").hidden = !isT1;
  $("#mock-writing-t2-subpane").hidden = isT1;

  const wList = mockState.test.questions.writing || [];
  const t1Data = wList[0] || { prompt: "Summarise the information by selecting and reporting the main features." };
  const t2Data = wList[1] || { prompt: "Write an essay discussing both views and giving your own opinion." };

  if (isT1) {
    $("#mock-t1-prompt").textContent = t1Data.prompt;
    if (t1Data.visualSvg) {
      $("#mock-t1-chart-svg").innerHTML = t1Data.visualSvg;
      $("#mock-t1-chart-svg").hidden = false;
    } else {
      $("#mock-t1-chart-svg").hidden = true;
    }
    if (t1Data.dataTableHtml) {
      $("#mock-t1-table").innerHTML = t1Data.dataTableHtml;
      $("#mock-t1-table").hidden = false;
    } else {
      $("#mock-t1-table").hidden = true;
    }
    const t1Val = mockState.answers.writing.task1 || "";
    const editor = $("#mock-t1-response");
    if (editor && editor.value !== t1Val) editor.value = t1Val;
    updateMockWritingWordCount(1);
  } else {
    $("#mock-t2-prompt").textContent = t2Data.prompt;
    const t2Val = mockState.answers.writing.task2 || "";
    const editor = $("#mock-t2-response");
    if (editor && editor.value !== t2Val) editor.value = t2Val;
    updateMockWritingWordCount(2);
  }
}

$("#mock-writing-t1-btn")?.addEventListener("click", () => {
  mockState.currentIndex = 0;
  renderQuestionGrid();
  renderCurrentQuestion();
});
$("#mock-writing-t2-btn")?.addEventListener("click", () => {
  mockState.currentIndex = 1;
  renderQuestionGrid();
  renderCurrentQuestion();
});

$("#mock-t1-response")?.addEventListener("input", (e) => {
  mockState.answers.writing.task1 = e.target.value;
  updateMockWritingWordCount(1);
  renderQuestionGrid();
});

$("#mock-t2-response")?.addEventListener("input", (e) => {
  mockState.answers.writing.task2 = e.target.value;
  updateMockWritingWordCount(2);
  renderQuestionGrid();
});

function updateMockWritingWordCount(taskNum) {
  if (taskNum === 1) {
    const text = $("#mock-t1-response")?.value || "";
    const words = text.trim() ? text.trim().split(/\s+/).length : 0;
    $("#mock-t1-word-count").textContent = `${words} / 150 kata ${words >= 150 ? "✓" : ""}`;
  } else {
    const text = $("#mock-t2-response")?.value || "";
    const words = text.trim() ? text.trim().split(/\s+/).length : 0;
    $("#mock-t2-word-count").textContent = `${words} / 250 kata ${words >= 250 ? "✓" : ""}`;
  }
}

// -------------------------------------------------------------
// Prev & Next Buttons
// -------------------------------------------------------------
$("#mock-prev-btn")?.addEventListener("click", () => {
  if (mockState.currentIndex > 0) {
    mockState.currentIndex--;
    renderQuestionGrid();
    renderCurrentQuestion();
  } else {
    // Navigate to previous section
    if (mockState.currentSection === "writing") switchSection("reading");
    else if (mockState.currentSection === "reading") switchSection("listening");
  }
});

$("#mock-next-btn")?.addEventListener("click", () => {
  const sec = mockState.currentSection;
  let count = 0;
  if (sec === "listening") count = (mockState.test.questions.listening || []).length;
  else if (sec === "reading") count = (mockState.test.questions.reading || []).length;
  else if (sec === "writing") count = 2;

  if (mockState.currentIndex < count - 1) {
    mockState.currentIndex++;
    renderQuestionGrid();
    renderCurrentQuestion();
  } else {
    // Navigate to next section
    if (mockState.currentSection === "listening") switchSection("reading");
    else if (mockState.currentSection === "reading") switchSection("writing");
    else if (mockState.currentSection === "writing") finishMockExam();
  }
});

// -------------------------------------------------------------
// Finish & Grade Mock Exam
// -------------------------------------------------------------
$("#mock-finish-exam-btn")?.addEventListener("click", () => {
  if (confirm("Apakah Anda yakin ingin menyelesaikan dan mengirim seluruh lembar jawaban simulasi IELTS sekarang?")) {
    finishMockExam();
  }
});

async function finishMockExam() {
  clearInterval(mockState.timerInterval);
  clearInterval(mockState.autosaveInterval);
  if (mockState.currentAudioPlayback) {
    mockState.currentAudioPlayback.stop();
    mockState.currentAudioPlayback = null;
  }

  // Ensure writing answers are synced
  mockState.answers.writing.task1 = $("#mock-t1-response")?.value || "";
  mockState.answers.writing.task2 = $("#mock-t2-response")?.value || "";

  const btn = $("#mock-finish-exam-btn");
  if (btn) {
    btn.disabled = true;
    btn.textContent = "Menilai ujian…";
  }

  try {
    const result = await api(`/api/mock-tests/${encodeURIComponent(mockState.test.id)}/finish`, {
      method: "POST",
      body: JSON.stringify({ answers: mockState.answers })
    });
    renderMockResults(result);
  } catch (err) {
    showToast("Gagal menilai ujian: " + err.message);
    if (btn) {
      btn.disabled = false;
      btn.textContent = "Selesaikan Ujian";
    }
  }
}

function renderMockResults(result) {
  $("#mock-lobby").hidden = true;
  $("#mock-exam-workspace").hidden = true;
  $("#mock-result-workspace").hidden = false;

  const scores = result.sectionScores || {};
  const overallBand = result.overallBand || 6.5;

  $("#mock-result-overall-band").textContent = overallBand.toFixed(1);

  // Section 1: Listening
  const lScore = scores.listening || { rawScore: 0, bandScore: 5.0, accuracyPct: 0 };
  $("#mock-res-listening-band").textContent = `Band ${lScore.bandScore.toFixed(1)}`;
  $("#mock-res-listening-raw").textContent = lScore.rawScore;
  $("#mock-res-listening-pct").textContent = `${Math.round(lScore.accuracyPct)}%`;

  // Section 2: Reading
  const rScore = scores.reading || { rawScore: 0, bandScore: 5.0, accuracyPct: 0 };
  $("#mock-res-reading-band").textContent = `Band ${rScore.bandScore.toFixed(1)}`;
  $("#mock-res-reading-raw").textContent = rScore.rawScore;
  $("#mock-res-reading-pct").textContent = `${Math.round(rScore.accuracyPct)}%`;

  // Section 3: Writing
  const wScore = scores.writing || { rawScore: 0, bandScore: 6.0 };
  $("#mock-res-writing-band").textContent = `Band ${wScore.bandScore.toFixed(1)}`;
  const wDetails = wScore.details || {};
  $("#mock-res-t1-band").textContent = `Band ${(wDetails.task1Band || 6.0).toFixed(1)}`;
  $("#mock-res-t2-band").textContent = `Band ${(wDetails.task2Band || 6.0).toFixed(1)}`;

  // Render Review Tab Default (Listening)
  renderMockReviewTab("listening", result);
}

function renderMockReviewTab(section, result) {
  const body = $("#mock-review-body");
  if (!body) return;

  $$(".mock-review-tabs button").forEach(btn => {
    btn.classList.toggle("is-active", btn.id === `mock-rev-tab-${section}`);
  });

  const questions = result.questions || {};
  const answers = result.answers || {};

  if (section === "listening" || section === "reading") {
    const list = questions[section] || [];
    const userAnswers = answers[section] || {};

    body.innerHTML = list.map((q, idx) => {
      const userAns = userAnswers[String(idx)];
      const isCorrect = userAns === q.correctAnswer;
      const statusBadge = isCorrect ? '<span class="rev-badge rev-badge--correct">✓ Benar</span>' : '<span class="rev-badge rev-badge--wrong">✕ Belum Tepat</span>';

      return `
        <article class="mock-review-item ${isCorrect ? "is-correct" : "is-wrong"}">
          <div class="rev-item-head">
            <span class="stage-label">SOAL ${idx + 1}</span>
            ${statusBadge}
          </div>
          <h4>${escapeHTML(learnerPrompt(q.prompt))}</h4>
          ${q.context ? `<div class="rev-transcript-box"><strong>${section === "listening" ? "Transkrip Audio" : "Konteks Teks"}:</strong><p>${highlightEvidence(q.context, q.explanation)}</p></div>` : ""}
          <div class="rev-choices-list">
            ${q.choices.map((c, cIdx) => {
              const isUser = userAns === cIdx;
              const isAns = q.correctAnswer === cIdx;
              let choiceClass = "";
              if (isAns) choiceClass = "is-correct-choice";
              if (isUser && !isAns) choiceClass = "is-user-wrong-choice";

              return `
                <div class="rev-choice-row ${choiceClass}">
                  <span class="choice-alpha">${String.fromCharCode(65 + cIdx)}</span>
                  <span class="choice-text">${escapeHTML(c)}</span>
                  ${isUser ? '<span class="choice-tag">Jawaban Anda</span>' : ""}
                  ${isAns ? '<span class="choice-tag choice-tag--correct">Kunci Jawaban</span>' : ""}
                </div>
              `;
            }).join("")}
          </div>
          ${q.explanation ? `<div class="rev-explanation"><p><strong>Pembahasan:</strong> ${escapeHTML(q.explanation)}</p></div>` : ""}
        </article>
      `;
    }).join("");
  } else if (section === "writing") {
    const wScore = (result.sectionScores || {}).writing || {};
    const details = wScore.details || {};
    const t1Fb = details.task1Feedback || {};
    const t2Fb = details.task2Feedback || {};

    body.innerHTML = `
      <div class="writing-review-split">
        <article class="writing-task-review-card">
          <h4>Review Writing Task 1 (Band ${(details.task1Band || 6.0).toFixed(1)})</h4>
          <p class="writing-summary-quote">${escapeHTML(t1Fb.summary || "Penilaian Task 1 selesai.")}</p>
          <div class="rubric-mini-grid">
            <span>Task Achievement: <strong>${t1Fb.taskResponse || 6.0}</strong></span>
            <span>Coherence & Cohesion: <strong>${t1Fb.coherence || 6.0}</strong></span>
            <span>Lexical Resource: <strong>${t1Fb.lexicalResource || 6.0}</strong></span>
            <span>Grammar Accuracy: <strong>${t1Fb.grammar || 6.0}</strong></span>
          </div>
          <div class="task-essay-view">
            <strong>Jawaban Anda:</strong>
            <pre>${escapeHTML(answers.writing?.task1 || "(Tidak ada jawaban yang dimasukkan)")}</pre>
          </div>
        </article>

        <article class="writing-task-review-card">
          <h4>Review Writing Task 2 (Band ${(details.task2Band || 6.0).toFixed(1)})</h4>
          <p class="writing-summary-quote">${escapeHTML(t2Fb.summary || "Penilaian Task 2 selesai.")}</p>
          <div class="rubric-mini-grid">
            <span>Task Response: <strong>${t2Fb.taskResponse || 6.0}</strong></span>
            <span>Coherence & Cohesion: <strong>${t2Fb.coherence || 6.0}</strong></span>
            <span>Lexical Resource: <strong>${t2Fb.lexicalResource || 6.0}</strong></span>
            <span>Grammar Accuracy: <strong>${t2Fb.grammar || 6.0}</strong></span>
          </div>
          <div class="task-essay-view">
            <strong>Jawaban Anda:</strong>
            <pre>${escapeHTML(answers.writing?.task2 || "(Tidak ada jawaban yang dimasukkan)")}</pre>
          </div>
        </article>
      </div>
    `;
  }
}

$("#mock-rev-tab-listening")?.addEventListener("click", () => renderMockReviewTab("listening", mockState.test));
$("#mock-rev-tab-reading")?.addEventListener("click", () => renderMockReviewTab("reading", mockState.test));
$("#mock-rev-tab-writing")?.addEventListener("click", () => renderMockReviewTab("writing", mockState.test));

$("#mock-back-to-lobby-btn")?.addEventListener("click", openMockLobby);
