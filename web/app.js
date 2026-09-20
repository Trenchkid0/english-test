const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];

const state = {
  session: null,
  review: [],
  index: 0,
  timerID: null,
  saveInFlight: false,
  quizAudio: null,
  listenedQuestions: {},
};

const screens = {
  setup: $("#setup-screen"),
  learning: $("#learning-screen"),
  quiz: $("#quiz-screen"),
  result: $("#result-screen"),
  mock: $("#mock-screen"),
};

function escapeHTML(value = "") {
  return String(value).replace(/[&<>'"]/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;",
  })[character]);
}

function highlightEvidence(context, evidence) {
  if (!evidence) return escapeHTML(context);
  const start = context.toLocaleLowerCase().indexOf(evidence.toLocaleLowerCase());
  if (start < 0) return escapeHTML(context);
  const end = start + evidence.length;
  return `${escapeHTML(context.slice(0, start))}<mark>${escapeHTML(context.slice(start, end))}</mark>${escapeHTML(context.slice(end))}`;
}

function sourceLabel(source) {
  return ({ review: "latihan ulang", spaced_review: "review terjadwal", diagnostic: "diagnostik", question_bank: "bank soal", deepseek: "DeepSeek", system: "sistem", demo: "demo" })[source] || source;
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = new Error(data.error || "Server belum dapat memproses permintaan.");
    error.status = response.status;
    if (response.status === 401 && !path.startsWith("/api/auth/")) {
      document.dispatchEvent(new CustomEvent("auth:expired"));
    }
    throw error;
  }
  return data;
}

function showScreen(name) {
  if (name !== "quiz") stopQuizAudio();
  Object.entries(screens).forEach(([key, element]) => { element.hidden = key !== name; });
  $("#app").focus({ preventScroll: true });
  window.scrollTo({ top: 0, behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });
}

function stopQuizAudio() {
  const playback = state.quizAudio;
  if (playback) {
    playback.utterance.onstart = null;
    playback.utterance.onend = null;
    playback.utterance.onerror = null;
  }
  state.quizAudio = null;
  if ("speechSynthesis" in window) window.speechSynthesis.cancel();
}

function showSetup() {
  clearInterval(state.timerID);
  state.timerID = null;
  showScreen("setup");
}

function updateSetupSummary() {
  const count = Number($("#count").value);
  const countInput = $("#count");
  const questionMode = $('input[name="questionMode"]:checked', $("#practice-form"))?.value || "bank";
  const progress = ((count - Number(countInput.min)) / (Number(countInput.max) - Number(countInput.min))) * 100;
  [...countInput.classList].filter((name) => name.startsWith("range-progress-")).forEach((name) => countInput.classList.remove(name));
  countInput.classList.add(`range-progress-${Math.round(progress)}`);
  $("#count-output").value = `${count} soal`;
  $("#summary-count").textContent = `${count} pertanyaan`;
  const source = questionMode === "deepseek" ? "soal baru" : "bank soal";
  $("#summary-detail").textContent = `${$("#level").value} · target ${$("#ielts-target").value} · ${$("#duration").value} menit · ${source}`;
}

updateSetupSummary();

$("#practice-form").addEventListener("input", updateSetupSummary);
$("#practice-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const types = $$('input[name="types"]:checked', form).map((input) => input.value);
  const questionMode = $('input[name="questionMode"]:checked', form).value;
  const typeField = $(".type-field");
  if (!types.length) {
    typeField.setAttribute("aria-invalid", "true");
    $("#types-help").textContent = "Belum ada tipe dipilih. Pilih minimal satu untuk membuat latihan.";
    return;
  }
  typeField.removeAttribute("aria-invalid");
  $("#types-help").textContent = "Pilih minimal satu tipe.";

  const button = $("#generate-button");
  const status = $("#form-status");
  button.disabled = true;
  button.dataset.state = "loading";
  $(".btn-label", button).textContent = questionMode === "deepseek" ? "Membuat soal baru…" : "Membuka bank soal…";
  status.textContent = questionMode === "deepseek" ? "DeepSeek sedang menyusun soal dan akan menyimpannya ke bank." : "Memilih soal tersimpan sesuai level dan fokus Anda.";

  try {
    const data = await api("/api/sessions", {
      method: "POST",
      body: JSON.stringify({
        level: $("#level").value,
        ieltsTarget: Number($("#ielts-target").value),
        count: Number($("#count").value),
        durationMinutes: Number($("#duration").value),
        types,
        questionMode,
      }),
    });
    state.session = data.session;
    state.index = 0;
    state.review = [];
    if (data.notice) showToast(data.notice, 2000);
    startQuiz();
  } catch (error) {
    button.dataset.state = "error";
    status.textContent = error.message;
  } finally {
    button.disabled = false;
    if (button.dataset.state !== "error") delete button.dataset.state;
    $(".btn-label", button).textContent = "Mulai latihan";
  }
});

function startQuiz() {
  stopQuizAudio();
  state.listenedQuestions = {};
  showScreen("quiz");
  renderQuestion();
  clearInterval(state.timerID);
  updateTimer();
  state.timerID = setInterval(updateTimer, 1000);
}

function cleanErrorSentence(prompt) {
  let s = learnerPrompt(prompt);
  s = s.replace(/^(Identify the (incorrect part (in the sentence)?|error (in the sentence)?|grammatical error)|Choose the (incorrect part|error)|Find the error):\s*/i, "").trim();
  if ((s.startsWith("'") && s.endsWith("'")) || (s.startsWith('"') && s.endsWith('"'))) {
    s = s.slice(1, -1).trim();
  }
  return s;
}

function renderQuestionPromptAndChoices(question, selected, letters) {
  const rawPrompt = learnerPrompt(question.prompt);
  const bracketRegex = /\[([^\]]+)\]/g;
  const hasBrackets = /\[([^\]]+)\]/.test(rawPrompt);

  if (hasBrackets) {
    let chipIndex = 0;
    const cleanInstruction = cleanErrorSentence(rawPrompt);

    const interactiveSentence = cleanInstruction.replace(bracketRegex, (match, word) => {
      let choiceIdx = question.choices.findIndex((c) => c.trim().toLowerCase() === word.trim().toLowerCase());
      if (choiceIdx < 0) {
        choiceIdx = chipIndex;
      }
      chipIndex++;
      const letter = letters[choiceIdx] || String.fromCharCode(65 + choiceIdx);
      const isSelected = selected === choiceIdx;
      const displayWord = (word.trim().match(/^[A-D]$/i) && question.choices[choiceIdx] && question.choices[choiceIdx].toLowerCase() !== word.toLowerCase())
        ? question.choices[choiceIdx]
        : word;

      return `<button type="button" class="error-chip ${isSelected ? "is-selected" : ""}" data-choice-index="${choiceIdx}" aria-pressed="${isSelected}"><span class="chip-badge">${letter}</span><span class="chip-word">${escapeHTML(displayWord)}</span></button>`;
    });

    const selectedWord = (selected !== undefined && question.choices[selected]) ? question.choices[selected] : null;

    return `
      <div class="error-spotter-panel">
        <div class="error-spotter-header">
          <div class="spotter-badge">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2.2"><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>
            <span>ERROR IDENTIFICATION</span>
          </div>
          <span class="spotter-help">Klik langsung pada kata di dalam kalimat yang menurut Anda <strong>salah</strong>:</span>
        </div>
        <div class="error-spotter-sentence-box">
          <p class="error-spotter-sentence">${interactiveSentence}</p>
        </div>
        <div class="error-spotter-feedback-bar" id="error-spotter-feedback">
          ${selectedWord ? `
            <div class="spotter-picked-status">
              <span class="picked-icon">✓</span>
              <span>Kata yang Anda tandai salah:</span>
              <strong class="picked-word-pill">${escapeHTML(selectedWord)}</strong>
            </div>
          ` : `
            <div class="spotter-waiting-status">
              <span class="waiting-icon">👉</span>
              <span>Klik langsung kata di atas yang memuat kesalahan grammar.</span>
            </div>
          `}
        </div>
      </div>
      <!-- Hidden inputs for seamless form management -->
      <div hidden aria-hidden="true">
        ${question.choices.map((choice, index) => `
          <input type="radio" name="answer" value="${index}" ${selected === index ? "checked" : ""}>
        `).join("")}
      </div>
    `;
  }

  return `
    <h3 class="question-title">${escapeHTML(rawPrompt)}</h3>
    <div class="answer-list" role="radiogroup" aria-label="Pilihan jawaban">
      ${question.choices.map((choice, index) => `
        <label class="answer-option ${selected === index ? "is-selected" : ""}">
          <input type="radio" name="answer" value="${index}" ${selected === index ? "checked" : ""}>
          <span class="answer-letter">${letters[index]}</span>
          <span>${escapeHTML(choice)}</span>
        </label>`).join("")}
    </div>
  `;
}

function renderReviewPrompt(rawPrompt, choices, selectedIdx, correctIdx, letters) {
  const cleanPrompt = learnerPrompt(rawPrompt);
  const bracketRegex = /\[([^\]]+)\]/g;
  if (!bracketRegex.test(cleanPrompt)) {
    return `<h3 class="review-question">${escapeHTML(cleanPrompt)}</h3>`;
  }

  let chipIndex = 0;
  const cleanInstruction = cleanErrorSentence(cleanPrompt);

  const interactiveSentence = cleanInstruction.replace(bracketRegex, (match, word) => {
    let choiceIdx = choices.findIndex((c) => c.trim().toLowerCase() === word.trim().toLowerCase());
    if (choiceIdx < 0) choiceIdx = chipIndex;
    chipIndex++;

    const letter = letters[choiceIdx] || String.fromCharCode(65 + choiceIdx);
    const isChosen = selectedIdx === choiceIdx;
    const isTarget = correctIdx === choiceIdx;
    const displayWord = (word.trim().match(/^[A-D]$/i) && choices[choiceIdx] && choices[choiceIdx].toLowerCase() !== word.toLowerCase())
      ? choices[choiceIdx]
      : word;

    let chipClass = "error-chip-review";
    if (isTarget) {
      chipClass += " is-actual-error";
    }
    if (isChosen) {
      chipClass += isTarget ? " is-correctly-spotted" : " is-wrongly-chosen";
    }

    return `<span class="${chipClass}"><span class="chip-badge">${letter}</span><span class="chip-word">${escapeHTML(displayWord)}</span>${isTarget ? `<span class="error-spot-tag">⚠️ Kata Salah</span>` : ""}</span>`;
  });

  return `
    <div class="error-spotter-review-panel">
      <div class="spotter-badge">
        <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2.2"><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>
        <span>ERROR IDENTIFICATION</span>
      </div>
      <div class="error-spotter-sentence-box">
        <p class="error-spotter-sentence">${interactiveSentence}</p>
      </div>
    </div>
  `;
}

function renderQuestion() {
  stopQuizAudio();
  const session = state.session;
  const question = session.questions[state.index];
  const selected = session.answers[String(state.index)] ?? session.answers[state.index];
  const letters = ["A", "B", "C", "D"];
  const isListening = question.type === "listening";
  const isUnlocked = !isListening || Boolean(state.listenedQuestions?.[state.index]) || selected !== undefined;

  let listeningBanner = "";
  if (isListening) {
    const playIconSvg = `<svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><polygon points="6 3 20 12 6 21 6 3"></polygon></svg>`;
    const replayIconSvg = `<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 4v6h6"></path><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"></path></svg>`;

    listeningBanner = `
      <div class="listening-sim-deck" id="quiz-audio-container" data-unlocked="${isUnlocked}">
        <div class="audio-deck-player">
          <div class="audio-deck-meta">
            <div class="audio-pill-badge">
              <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 18v-6a9 9 0 0 1 18 0v6"></path><path d="M21 19a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3zM3 19a2 2 0 0 0 2 2h1a2 2 0 0 0 2-2v-3a2 2 0 0 0-2-2H3z"></path></svg>
              <span>IELTS Listening Track</span>
            </div>
            <div class="audio-deck-specs">
              <span class="spec-accent">British English</span>
              <span class="spec-dot">•</span>
              <span class="spec-speed">0.86x Native Test Pace</span>
            </div>
          </div>

          <div class="audio-deck-controls">
            <button class="audio-deck-btn" type="button" id="quiz-listen-play" aria-label="Kontrol rekaman audio">
              <span class="audio-btn-icon" id="audio-btn-icon">${isUnlocked ? replayIconSvg : playIconSvg}</span>
              <span class="audio-btn-text" id="audio-btn-text">${isUnlocked ? "Putar Ulang Audio" : "Putar Rekaman Audio"}</span>
            </button>

            <div class="audio-waveform-visual" id="audio-visualizer" data-active="false" aria-hidden="true">
              <span class="wave-bar bar-1"></span>
              <span class="wave-bar bar-2"></span>
              <span class="wave-bar bar-3"></span>
              <span class="wave-bar bar-4"></span>
              <span class="wave-bar bar-5"></span>
              <span class="wave-bar bar-6"></span>
              <span class="wave-bar bar-7"></span>
            </div>

            <div class="audio-deck-status-box">
              <span class="audio-status-pill ${isUnlocked ? "is-complete" : "is-ready"}" id="audio-status-pill">
                ${isUnlocked ? "Selesai" : "Siap"}
              </span>
              <span id="quiz-audio-status" class="audio-deck-status-text" role="status">
                ${isUnlocked ? "Rekaman selesai didengarkan. Lembar soal telah terbuka di bawah." : "Tekan tombol putar untuk memulai audio percakapan."}
              </span>
            </div>
          </div>
        </div>

        ${!isUnlocked ? `
          <div class="listening-focus-card" id="quiz-audio-locked">
            <div class="focus-card-glow"></div>
            <div class="focus-card-header">
              <div class="focus-card-icon-badge">
                <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
                  <path d="M7 11V7a5 5 0 0 1 10 0v4"></path>
                </svg>
              </div>
              <div class="focus-card-headings">
                <h4 class="focus-card-title">Fokus Menyimak Rekaman Audio</h4>
                <p class="focus-card-subtitle">Format resmi simulasi IELTS Listening</p>
              </div>
            </div>

            <div class="focus-card-body">
              <p class="focus-card-desc">
                Pertanyaan dan pilihan jawaban akan <strong>muncul secara otomatis</strong> tepat setelah Anda mendengarkan rekaman audio sampai selesai.
              </p>
              <div class="focus-card-strategy-pill">
                <span class="strategy-icon">💡</span>
                <span><strong>Tips IELTS:</strong> Pusatkan perhatian pada detail percakapan, angka/waktu, nama tokoh, dan kata kunci utama.</span>
              </div>
            </div>
          </div>
        ` : ""}
      </div>`;
  }

  const promptAndChoicesHTML = renderQuestionPromptAndChoices(question, selected, letters);
  const displayType = (typeof typeNames !== "undefined" && typeNames[question.type]) ? typeNames[question.type] : question.type.replace(/_/g, " ");
  $("#question-panel").innerHTML = `
    <p class="question-meta">${escapeHTML(displayType)} · ${escapeHTML(question.ieltsSkill)}</p>
    ${isListening ? listeningBanner : (question.context ? `<p class="question-context">${escapeHTML(question.context)}</p>` : "")}
    <div id="quiz-body-section" class="quiz-body-section ${!isUnlocked ? "is-hidden-listening" : "quiz-body-section--revealed"}" ${!isUnlocked ? "hidden" : ""}>
      ${promptAndChoicesHTML}
    </div>`;

  $$('input[name="answer"]', $("#question-panel")).forEach((input) => input.addEventListener("change", saveCurrentAnswer));
  $$('.error-chip', $("#question-panel")).forEach((chip) => {
    chip.addEventListener("click", () => {
      const idx = chip.dataset.choiceIndex;
      const radio = $(`input[name="answer"][value="${idx}"]`, $("#question-panel"));
      if (radio) {
        radio.checked = true;
        radio.dispatchEvent(new Event("change"));
      }
    });
  });
  if (isListening) {
    const audioButton = $("#quiz-listen-play");
    const audioBtnIcon = $("#audio-btn-icon");
    const audioBtnText = $("#audio-btn-text");
    const audioStatus = $("#quiz-audio-status");
    const audioStatusPill = $("#audio-status-pill");
    const audioVisualizer = $("#audio-visualizer");
    const bodySection = $("#quiz-body-section");
    const lockedCard = $("#quiz-audio-locked");

    const playSvg = `<svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><polygon points="6 3 20 12 6 21 6 3"></polygon></svg>`;
    const pauseSvg = `<svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><rect x="6" y="4" width="4" height="16" rx="1"></rect><rect x="14" y="4" width="4" height="16" rx="1"></rect></svg>`;
    const replaySvg = `<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M1 4v6h6"></path><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"></path></svg>`;

    const setAudioVisualState = (stateName) => {
      if (!audioVisualizer) return;
      audioVisualizer.dataset.active = stateName === "playing" ? "true" : "false";
      if (audioStatusPill) {
        audioStatusPill.className = `audio-status-pill is-${stateName}`;
        if (stateName === "playing") audioStatusPill.textContent = "Memutar";
        else if (stateName === "paused") audioStatusPill.textContent = "Dijeda";
        else if (stateName === "complete") audioStatusPill.textContent = "Selesai";
        else audioStatusPill.textContent = "Siap";
      }
    };

    const unlockQuestion = () => {
      if (!state.listenedQuestions) state.listenedQuestions = {};
      state.listenedQuestions[state.index] = true;
      setAudioVisualState("complete");
      if (bodySection) {
        bodySection.hidden = false;
        bodySection.classList.remove("is-hidden-listening");
        bodySection.classList.add("quiz-body-section--revealed");
      }
      if (lockedCard) {
        lockedCard.classList.add("is-dissolving");
        setTimeout(() => lockedCard.remove(), 260);
      }
      if (audioStatus) audioStatus.textContent = "Rekaman audio selesai. Silakan pilih jawaban terbaik di bawah.";
      if (audioBtnIcon) audioBtnIcon.innerHTML = replaySvg;
      if (audioBtnText) audioBtnText.textContent = "Putar Ulang Audio";
    };

    audioButton.addEventListener("click", () => {
      if (!("speechSynthesis" in window)) {
        if (audioStatus) audioStatus.textContent = "Browser tidak mendukung audio. Pertanyaan dibuka secara otomatis.";
        unlockQuestion();
        return;
      }
      const current = state.quizAudio;
      if (current?.questionIndex === state.index && current.status === "playing") {
        window.speechSynthesis.pause();
        current.status = "paused";
        if (audioBtnIcon) audioBtnIcon.innerHTML = playSvg;
        if (audioBtnText) audioBtnText.textContent = "Lanjutkan Audio";
        if (audioStatus) audioStatus.textContent = "Audio dijeda. Tekan tombol untuk melanjutkan mendengarkan.";
        setAudioVisualState("paused");
        return;
      }
      if (current?.questionIndex === state.index && current.status === "paused") {
        window.speechSynthesis.resume();
        current.status = "playing";
        if (audioBtnIcon) audioBtnIcon.innerHTML = pauseSvg;
        if (audioBtnText) audioBtnText.textContent = "Jeda Audio";
        if (audioStatus) audioStatus.textContent = "Audio sedang diputar… Dengarkan sampai selesai.";
        setAudioVisualState("playing");
        return;
      }
      if (current?.questionIndex === state.index && current.status === "queued") return;

      stopQuizAudio();
      const utterance = new SpeechSynthesisUtterance(question.context || question.prompt);
      const playback = { utterance, questionIndex: state.index, status: "queued" };
      state.quizAudio = playback;
      utterance.lang = "en-GB";
      utterance.rate = 0.86;
      utterance.onstart = () => {
        if (state.quizAudio !== playback) return;
        playback.status = "playing";
        if (audioBtnIcon) audioBtnIcon.innerHTML = pauseSvg;
        if (audioBtnText) audioBtnText.textContent = "Jeda Audio";
        if (audioStatus) audioStatus.textContent = "Audio sedang diputar… Dengarkan sampai selesai.";
        setAudioVisualState("playing");
      };
      utterance.onend = () => {
        if (state.quizAudio !== playback) return;
        state.quizAudio = null;
        unlockQuestion();
      };
      utterance.onerror = () => {
        if (state.quizAudio !== playback) return;
        state.quizAudio = null;
        if (audioBtnIcon) audioBtnIcon.innerHTML = playSvg;
        if (audioBtnText) audioBtnText.textContent = "Putar Rekaman Audio";
        if (audioStatus) audioStatus.textContent = "Audio gagal diputar. Pertanyaan tetap dibuka agar Anda dapat melanjutkan latihan.";
        unlockQuestion();
      };
      if (audioStatus) audioStatus.textContent = "Menyiapkan rekaman audio…";
      window.speechSynthesis.speak(utterance);
    });
  }
  renderQuestionNav();
  updateProgress();
  $("#prev-question").disabled = state.index === 0;
  const next = $("#next-question");
  next.textContent = state.index === session.questions.length - 1 ? "Nilai latihan" : "Soal berikutnya";
}

function learnerPrompt(prompt) {
  return String(prompt || "").replace(/\s*\[Level[^\]]*\]\s*$/u, "").trim();
}

function renderQuestionNav() {
  const answers = state.session.answers;
  $("#question-nav").innerHTML = state.session.questions.map((_, index) => {
    const answered = answers[String(index)] !== undefined || answers[index] !== undefined;
    return `<button type="button" data-index="${index}" aria-label="Soal ${index + 1}${answered ? ", sudah dijawab" : ""}" aria-current="${index === state.index}" class="${answered ? "is-answered" : ""}">${index + 1}</button>`;
  }).join("");
  $$("button", $("#question-nav")).forEach((button) => button.addEventListener("click", () => {
    if (state.saveInFlight) return;
    state.index = Number(button.dataset.index);
    renderQuestion();
  }));
}

async function saveCurrentAnswer(event) {
  const index = state.index;
  const selectedIndex = Number(event.target.value);
  const prior = state.session.answers[String(index)] ?? state.session.answers[index];
  state.session.answers[index] = selectedIndex;
  state.saveInFlight = true;
  $("#save-status").textContent = "Menyimpan jawaban…";

  $$('.error-chip', $("#question-panel")).forEach((chip) => {
    const isSelected = Number(chip.dataset.choiceIndex) === selectedIndex;
    chip.classList.toggle("is-selected", isSelected);
    chip.setAttribute("aria-pressed", String(isSelected));
  });
  const feedbackBar = $("#error-spotter-feedback", $("#question-panel"));
  if (feedbackBar && state.session?.questions[index]?.choices[selectedIndex]) {
    const pickedWord = state.session.questions[index].choices[selectedIndex];
    feedbackBar.innerHTML = `
      <div class="spotter-picked-status">
        <span class="picked-icon">✓</span>
        <span>Kata yang Anda tandai salah:</span>
        <strong class="picked-word-pill">${escapeHTML(pickedWord)}</strong>
      </div>
    `;
  }
  $$('.answer-option', $("#question-panel")).forEach((opt, idx) => {
    opt.classList.toggle("is-selected", idx === selectedIndex);
  });

  $$('input[name="answer"]').forEach((input) => { input.disabled = true; });
  renderQuestionNav();
  updateProgress();
  try {
    await api(`/api/sessions/${encodeURIComponent(state.session.id)}/answers/${index}`, {
      method: "PUT",
      body: JSON.stringify({ selectedIndex }),
    });
    $("#save-status").textContent = `Jawaban soal ${index + 1} tersimpan.`;
  } catch (error) {
    if (prior === undefined) delete state.session.answers[index]; else state.session.answers[index] = prior;
    $("#save-status").textContent = "Jawaban belum tersimpan.";
    showToast(`${error.message} Pilih jawaban sekali lagi.`);
    renderQuestionNav();
    updateProgress();
  } finally {
    state.saveInFlight = false;
    $$('input[name="answer"]').forEach((input) => { input.disabled = false; });
  }
}

$("#prev-question").addEventListener("click", () => {
  if (state.index > 0 && !state.saveInFlight) { state.index -= 1; renderQuestion(); }
});

$("#next-question").addEventListener("click", async () => {
  if (state.saveInFlight) return;
  if (state.index < state.session.questions.length - 1) {
    state.index += 1;
    renderQuestion();
    return;
  }
  const answered = Object.keys(state.session.answers).length;
  if (answered < state.session.questions.length) {
    const firstEmpty = state.session.questions.findIndex((_, index) => state.session.answers[String(index)] === undefined && state.session.answers[index] === undefined);
    state.index = firstEmpty;
    renderQuestion();
    $("#save-status").textContent = `${state.session.questions.length - answered} soal belum dijawab.`;
    return;
  }
  await finishSession();
});

function updateProgress() {
  const answered = Object.keys(state.session.answers).length;
  const progress = answered / state.session.questions.length;
  $("#progress-bar").value = progress;
  $("#quiz-heading").textContent = `${answered} dari ${state.session.questions.length} soal terjawab.`;
}

function updateTimer() {
  if (!state.session) return;
  const expiresAt = new Date(state.session.createdAt).getTime() + state.session.durationMinutes * 60_000;
  const left = Math.max(0, expiresAt - Date.now());
  const minutes = Math.floor(left / 60_000);
  const seconds = Math.floor((left % 60_000) / 1000);
  $("#timer strong").textContent = `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  $("#timer").classList.toggle("is-low", left <= 60_000);
  if (left === 0) {
    clearInterval(state.timerID);
    finishSession(true);
  }
}

async function finishSession(timedOut = false) {
  stopQuizAudio();
  const button = $("#next-question");
  button.disabled = true;
  button.dataset.state = "loading";
  button.textContent = "Menghitung nilai…";
  try {
    const data = await api(`/api/sessions/${encodeURIComponent(state.session.id)}/complete`, { method: "POST" });
    state.session = data.session;
    state.review = data.review;
    renderResults(timedOut);
  } catch (error) {
    showToast(error.message);
  } finally {
    button.disabled = false;
    delete button.dataset.state;
  }
}

function animateScore(target) {
  const output = $("#score-value");
  if (matchMedia("(prefers-reduced-motion: reduce)").matches) { output.textContent = target; return; }
  const start = performance.now();
  const tick = (now) => {
    const progress = Math.min(1, (now - start) / 700);
    const eased = 1 - Math.pow(1 - progress, 3);
    output.textContent = Math.round(target * eased);
    if (progress < 1) requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);
}

async function openHistory() {
  const dialog = $("#history-dialog");
  dialog.showModal();
  $("#history-list").innerHTML = '<p class="empty-copy">Memuat riwayat…</p>';
  try {
    const items = await api("/api/sessions");
    if (!items.length) {
      $("#history-list").innerHTML = '<p class="empty-copy">Belum ada sesi. Buat latihan pertama untuk mulai mengisi arsip belajar.</p>';
      return;
    }
    $("#history-list").innerHTML = items.map((item) => {
      const date = new Intl.DateTimeFormat("id-ID", { dateStyle: "medium", timeStyle: "short" }).format(new Date(item.createdAt));
      const progress = item.status === "completed" ? `${item.score}/100` : `${item.answeredCount}/${item.questionCount}`;
		const action = item.status === "completed" ? "Buka review" : "Lanjutkan";
      const source = sourceLabel(item.source);
      return `<button class="history-row" type="button" data-session-id="${escapeHTML(item.id)}">
		<span><strong>${escapeHTML(item.level)} · target IELTS ${item.ieltsTarget}</strong><span>${date} · ${action} · ${escapeHTML(source)}</span></span>
        <strong class="history-score">${progress}</strong>
      </button>`;
    }).join("");
    $$(".history-row", $("#history-list")).forEach((row) => row.addEventListener("click", () => loadSession(row.dataset.sessionId, row)));
  } catch (error) {
    $("#history-list").innerHTML = `<p class="empty-copy">${escapeHTML(error.message)}</p>`;
  }
}

async function loadSession(id, button) {
  button.disabled = true;
  button.dataset.state = "loading";
  try {
    const data = await api(`/api/sessions/${encodeURIComponent(id)}`);
    state.session = data.session;
    state.review = data.review || [];
    $("#history-dialog").close();
    if (state.session.status === "completed") {
      renderResults();
    } else {
      const firstEmpty = state.session.questions.findIndex((_, index) => state.session.answers[String(index)] === undefined && state.session.answers[index] === undefined);
      state.index = firstEmpty < 0 ? state.session.questions.length - 1 : firstEmpty;
      startQuiz();
    }
  } catch (error) {
    button.dataset.state = "error";
    showToast(error.message);
  } finally {
    button.disabled = false;
    if (button.dataset.state !== "error") delete button.dataset.state;
  }
}

function renderResults(timedOut = false) {
  clearInterval(state.timerID);
  showScreen("result");
  const score = state.session.score ?? 0;
  const diagnostic = state.session.source === "diagnostic";
	const estimatedLevel = score < 40 ? "A2" : score < 75 ? "B1" : "B2";
  $("#result-message").textContent = diagnostic
		? `Rentang latihan awal: ${estimatedLevel}. Bank diagnostik singkat ini hanya memetakan A2–B2; gunakan pola kesalahan di bawah untuk menentukan fokus.`
    : timedOut
    ? `Waktu habis. Semua jawaban yang sudah tersimpan tetap masuk penilaian. Review ${state.review.length} soal di bawah.`
    : `${Object.keys(state.session.answers).length} jawaban tersimpan. Seluruh soal tetap ada di riwayat untuk dipelajari kembali.`;
  animateScore(score);
	const incorrectCount = state.review.filter((item) => !item.isCorrect).length;
	const retryButton = $("#retry-questions");
	retryButton.hidden = incorrectCount === 0;
	retryButton.textContent = `Latih lagi ${incorrectCount} soal`;
  $("#review-list").innerHTML = state.review.map((item, index) => {
    const selectedText = item.selectedIndex === undefined ? "Tidak dijawab" : item.question.choices[item.selectedIndex];
    const correctText = item.question.choices[item.correctIndex];
    const readingReflection = !item.isCorrect && item.question.type === "reading" ? `<form class="reading-reflection" data-reflection-index="${index}">
      <label for="reflection-${index}">Mengapa jawaban ini meleset?</label><div><select id="reflection-${index}" name="reason"><option value="detail">Detail terlewat</option><option value="inference">Inferensi keliru</option><option value="vocabulary">Kosakata</option><option value="rushed">Terburu-buru</option></select><button class="btn btn--soft" type="submit">Simpan refleksi</button></div><p role="status"></p>
    </form>` : "";
    const reviewPromptHTML = renderReviewPrompt(item.question.prompt, item.question.choices, item.selectedIndex, item.correctIndex, ["A", "B", "C", "D"]);
    return `<article class="review-item">
      <p class="review-number">SOAL ${index + 1} · ${escapeHTML(item.question.ieltsSkill)}</p>
      ${item.question.context ? `<p class="review-context">${highlightEvidence(item.question.context, item.evidence)}</p>` : ""}
      ${reviewPromptHTML}
      <div class="review-answer">
        <p class="answer-line ${item.isCorrect ? "is-correct" : "is-wrong"}"><strong>Jawaban Anda:</strong> ${escapeHTML(selectedText)} · ${item.isCorrect ? "Benar" : "Belum tepat"}</p>
        ${item.isCorrect ? "" : `<p class="answer-line is-correct"><strong>Jawaban benar:</strong> ${escapeHTML(correctText)}</p>`}
      </div>
      <div class="explanation">
        <p><strong>Mengapa?</strong> ${escapeHTML(item.explanation)}</p>
        <p class="learning-tip"><strong>Untuk IELTS:</strong> ${escapeHTML(item.learningTip)}</p>
        ${item.errorTag ? `<p><strong>Fokus:</strong> ${escapeHTML(item.errorTag)}</p>` : ""}
      </div>
      ${readingReflection}
    </article>`;
  }).join("");
  $$(".reading-reflection", $("#review-list")).forEach((form) => form.addEventListener("submit", saveReadingReflection));
}

async function saveReadingReflection(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const button = $('button[type="submit"]', form);
  const status = $('[role="status"]', form);
  button.disabled = true;
  button.dataset.state = "loading";
  try {
    await api(`/api/reflections/${encodeURIComponent(state.session.id)}/${form.dataset.reflectionIndex}`, { method: "PUT", body: JSON.stringify({ reason: $("select", form).value }) });
    button.dataset.state = "success";
    button.textContent = "Refleksi tersimpan";
    status.textContent = "Catatan ini akan membantu membaca pola kesalahan Anda.";
  } catch (error) {
    button.dataset.state = "error";
    button.disabled = false;
    status.textContent = error.message;
  }
}

$("#retry-questions").addEventListener("click", async () => {
	const button = $("#retry-questions");
	button.disabled = true;
	button.dataset.state = "loading";
	const label = button.textContent;
	button.textContent = "Menyiapkan latihan ulang…";
	try {
		const data = await api(`/api/sessions/${encodeURIComponent(state.session.id)}/retry`, { method: "POST" });
		state.session = data.session;
		state.review = [];
		state.index = 0;
		if (data.notice) showToast(data.notice, 2000);
		startQuiz();
	} catch (error) {
		button.dataset.state = "error";
		button.textContent = label;
		showToast(error.message);
	} finally {
		button.disabled = false;
		if (button.dataset.state !== "error") delete button.dataset.state;
	}
});

function animateScore(target) {
  const output = $("#score-value");
  if (matchMedia("(prefers-reduced-motion: reduce)").matches) { output.textContent = target; return; }
  const start = performance.now();
  const tick = (now) => {
    const progress = Math.min(1, (now - start) / 700);
    const eased = 1 - Math.pow(1 - progress, 3);
    output.textContent = Math.round(target * eased);
    if (progress < 1) requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);
}

async function openHistory() {
  const dialog = $("#history-dialog");
  dialog.showModal();
  $("#history-list").innerHTML = '<p class="empty-copy">Memuat riwayat…</p>';
  try {
    const items = await api("/api/sessions");
    if (!items.length) {
      $("#history-list").innerHTML = '<p class="empty-copy">Belum ada sesi. Buat latihan pertama untuk mulai mengisi arsip belajar.</p>';
      return;
    }
    $("#history-list").innerHTML = items.map((item) => {
      const date = new Intl.DateTimeFormat("id-ID", { dateStyle: "medium", timeStyle: "short" }).format(new Date(item.createdAt));
      const progress = item.status === "completed" ? `${item.score}/100` : `${item.answeredCount}/${item.questionCount}`;
		const action = item.status === "completed" ? "Buka review" : "Lanjutkan";
      const source = sourceLabel(item.source);
      return `<button class="history-row" type="button" data-session-id="${escapeHTML(item.id)}">
		<span><strong>${escapeHTML(item.level)} · target IELTS ${item.ieltsTarget}</strong><span>${date} · ${action} · ${escapeHTML(source)}</span></span>
        <strong class="history-score">${progress}</strong>
      </button>`;
    }).join("");
    $$(".history-row", $("#history-list")).forEach((row) => row.addEventListener("click", () => loadSession(row.dataset.sessionId, row)));
  } catch (error) {
    $("#history-list").innerHTML = `<p class="empty-copy">${escapeHTML(error.message)}</p>`;
  }
}

async function loadSession(id, button) {
  button.disabled = true;
  button.dataset.state = "loading";
  try {
    const data = await api(`/api/sessions/${encodeURIComponent(id)}`);
    state.session = data.session;
    state.review = data.review || [];
    $("#history-dialog").close();
    if (state.session.status === "completed") {
      renderResults();
    } else {
      const firstEmpty = state.session.questions.findIndex((_, index) => state.session.answers[String(index)] === undefined && state.session.answers[index] === undefined);
      state.index = firstEmpty < 0 ? state.session.questions.length - 1 : firstEmpty;
      startQuiz();
    }
  } catch (error) {
    button.dataset.state = "error";
    showToast(error.message);
  } finally {
    button.disabled = false;
    if (button.dataset.state !== "error") delete button.dataset.state;
  }
}

let toastTimerID = null;

function hideToast() {
  const toast = $("#toast");
  if (toast) toast.hidden = true;
  if (toastTimerID !== null) {
    clearTimeout(toastTimerID);
    toastTimerID = null;
  }
}

function showToast(message, duration = 2000) {
  const toast = $("#toast");
  if (!toast) return;
  if (toastTimerID !== null) {
    clearTimeout(toastTimerID);
    toastTimerID = null;
  }
  const span = $("span", toast);
  if (span) span.textContent = message;
  toast.hidden = false;
  toastTimerID = duration > 0 ? setTimeout(hideToast, duration) : null;
}

$("#toast button").addEventListener("click", hideToast);
$("#close-history").addEventListener("click", () => $("#history-dialog").close());
$("#history-dialog").addEventListener("click", (event) => {
  if (event.target === $("#history-dialog")) $("#history-dialog").close();
});

document.addEventListener("click", (event) => {
  const action = event.target.closest("[data-action]")?.dataset.action;
  if (action === "home") showSetup();
  if (action === "mock") openMockLobby();
  if (action === "learning") openLearning();
  if (action === "history") openHistory();
  if (action === "theme") toggleTheme();
});

window.addEventListener("pagehide", stopQuizAudio);

function toggleTheme() {
  const next = document.documentElement.dataset.theme === "dark" ? "light" : "dark";
  document.documentElement.dataset.theme = next;
  localStorage.setItem("ruang-kata-theme", next);
}

const savedTheme = localStorage.getItem("ruang-kata-theme");
document.documentElement.dataset.theme = savedTheme || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
updateSetupSummary();
