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

const featureLoads = new Map();

function loadFeatureScript(name) {
  if (featureLoads.has(name)) return featureLoads.get(name);
  const promise = new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = `/${name}.js?v=20260924`;
    script.async = true;
    script.addEventListener("load", resolve, { once: true });
    script.addEventListener("error", () => reject(new Error("Fitur belum dapat dimuat. Periksa koneksi lalu coba lagi.")), { once: true });
    document.head.append(script);
  }).catch((error) => {
    featureLoads.delete(name);
    throw error;
  });
  featureLoads.set(name, promise);
  return promise;
}

async function openFeature(name) {
  await loadFeatureScript(name);
  if (name === "learning") return openLearning();
  if (name === "mock_test") return openMockLobby();
}

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

const historyState = {
  items: [],
  filter: "all",
  search: "",
  sort: "newest",
  inited: false,
};

function formatFriendlyDate(dateInput) {
  if (!dateInput) return "-";
  const d = new Date(dateInput);
  const now = new Date();
  const isToday = d.toDateString() === now.toDateString();
  const yesterday = new Date();
  yesterday.setDate(yesterday.getDate() - 1);
  const isYesterday = d.toDateString() === yesterday.toDateString();

  const timeStr = new Intl.DateTimeFormat("id-ID", { hour: "2-digit", minute: "2-digit" }).format(d);
  if (isToday) return `Hari ini, ${timeStr}`;
  if (isYesterday) return `Kemarin, ${timeStr}`;
  const dateStr = new Intl.DateTimeFormat("id-ID", {
    day: "numeric",
    month: "short",
    year: d.getFullYear() !== now.getFullYear() ? "numeric" : undefined,
  }).format(d);
  return `${dateStr}, ${timeStr}`;
}

function getRatingTier(score) {
  if (score >= 85) return { label: "Sangat Baik", classModifier: "high" };
  if (score >= 70) return { label: "Baik", classModifier: "mid" };
  return { label: "Perlu Latihan", classModifier: "low" };
}

function renderHistorySkeletons() {
  const list = $("#history-list");
  if (!list) return;
  list.innerHTML = `
    <div class="history-skeleton-card" aria-hidden="true"></div>
    <div class="history-skeleton-card" aria-hidden="true"></div>
    <div class="history-skeleton-card" aria-hidden="true"></div>
  `;
}

function updateHistoryMetrics() {
  const items = historyState.items;
  const total = items.length;
  const completed = items.filter((i) => i.status === "completed").length;
  const pending = items.filter((i) => i.status !== "completed").length;
  const completedWithScore = items.filter((i) => i.status === "completed" && typeof i.score === "number");
  const avgScore = completedWithScore.length
    ? Math.round(completedWithScore.reduce((sum, item) => sum + (item.score || 0), 0) / completedWithScore.length)
    : null;
  const completionRate = total ? Math.round((completed / total) * 100) : 0;

  const totalEl = $("#hist-stat-total");
  const compEl = $("#hist-stat-completed");
  const rateEl = $("#hist-stat-comp-rate");
  const avgEl = $("#hist-stat-avg");
  const pendEl = $("#hist-stat-pending");

  if (totalEl) totalEl.textContent = total;
  if (compEl) compEl.textContent = completed;
  if (rateEl) rateEl.textContent = `${completionRate}% tuntas`;
  if (avgEl) avgEl.textContent = avgScore !== null ? `${avgScore}` : "-";
  if (pendEl) pendEl.textContent = pending;

  const countAll = $("#filter-count-all");
  const countComp = $("#filter-count-completed");
  const countPend = $("#filter-count-pending");
  if (countAll) countAll.textContent = total;
  if (countComp) countComp.textContent = completed;
  if (countPend) countPend.textContent = pending;
}

function renderHistoryView() {
  const list = $("#history-list");
  if (!list) return;

  if (!historyState.items.length) {
    list.innerHTML = `
      <div class="history-empty-state">
        <div class="history-empty-icon" aria-hidden="true">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1-2.5-2.5Z"></path>
            <path d="M6 6h10"></path>
            <path d="M6 10h7"></path>
          </svg>
        </div>
        <h3 class="history-empty-title">Belum ada riwayat latihan</h3>
        <p class="history-empty-desc">Selesaikan sesi latihan pertama Anda untuk mulai memetakan perkembangan IELTS personal di sini.</p>
        <button class="btn btn--pear" type="button" id="hist-empty-start-btn" style="margin-top:0.5rem;">Mulai Latihan Baru</button>
      </div>
    `;
    const btn = $("#hist-empty-start-btn");
    if (btn) {
      btn.addEventListener("click", () => {
        $("#history-dialog").close();
        showSetup();
      });
    }
    return;
  }

  // Filter
  const filtered = historyState.items.filter((item) => {
    if (historyState.filter === "completed" && item.status !== "completed") return false;
    if (historyState.filter === "in_progress" && item.status === "completed") return false;

    if (historyState.search) {
      const q = historyState.search.toLowerCase().trim();
      const level = (item.level || "").toLowerCase();
      const source = (sourceLabel(item.source) || "").toLowerCase();
      const target = `target ${item.ieltsTarget || ""}`.toLowerCase();
      const dateStr = formatFriendlyDate(item.createdAt).toLowerCase();
      if (!level.includes(q) && !source.includes(q) && !target.includes(q) && !dateStr.includes(q)) {
        return false;
      }
    }
    return true;
  });

  // Sort
  filtered.sort((a, b) => {
    if (historyState.sort === "oldest") return new Date(a.createdAt) - new Date(b.createdAt);
    if (historyState.sort === "highest_score") return (b.score ?? -1) - (a.score ?? -1);
    if (historyState.sort === "lowest_score") return (a.score ?? 999) - (b.score ?? 999);
    return new Date(b.createdAt) - new Date(a.createdAt); // newest
  });

  if (!filtered.length) {
    list.innerHTML = `
      <div class="history-empty-state">
        <div class="history-empty-icon" aria-hidden="true">
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="11" cy="11" r="8"></circle>
            <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
          </svg>
        </div>
        <h3 class="history-empty-title">Tidak ada riwayat yang cocok</h3>
        <p class="history-empty-desc">Coba sesuaikan kata kunci pencarian atau ubah tab status di atas.</p>
        <button class="btn btn--soft" type="button" id="reset-hist-filter" style="margin-top:0.4rem;">Reset Filter</button>
      </div>
    `;
    const resetBtn = $("#reset-hist-filter");
    if (resetBtn) {
      resetBtn.addEventListener("click", () => {
        historyState.filter = "all";
        historyState.search = "";
        const searchInput = $("#history-search");
        if (searchInput) searchInput.value = "";
        $$(".history-tab").forEach((tab) => {
          const isAll = tab.dataset.histFilter === "all";
          tab.classList.toggle("is-active", isAll);
          tab.setAttribute("aria-selected", isAll ? "true" : "false");
        });
        renderHistoryView();
      });
    }
    return;
  }

  list.innerHTML = filtered
    .map((item) => {
      const isCompleted = item.status === "completed";
      const dateFormatted = formatFriendlyDate(item.createdAt);
      const source = sourceLabel(item.source);
      const totalQ = item.questionCount || 10;
      const answeredQ = item.answeredCount || 0;
      const progressPercent = Math.min(100, Math.round((answeredQ / totalQ) * 100));

      let scoreHTML = "";
      if (isCompleted) {
        const score = item.score ?? 0;
        const tier = getRatingTier(score);
        scoreHTML = `
          <div class="history-score-badge history-score-badge--${tier.classModifier}">
            <div class="history-score-badge__val">${score}<span style="font-size:0.8rem; font-weight:500; opacity:0.7;">/100</span></div>
            <span class="history-score-badge__label">${tier.label}</span>
          </div>
        `;
      } else {
        scoreHTML = `
          <div class="history-score-badge">
            <div class="history-score-badge__val" style="color:var(--color-accent); font-size:1.15rem;">${answeredQ}<span style="font-size:0.8rem; opacity:0.75;">/${totalQ}</span></div>
            <span class="history-score-badge__label">Soal Terjawab</span>
          </div>
        `;
      }

      const progressSection = !isCompleted
        ? `
        <div class="history-card__progress-wrap">
          <div class="history-progress-track" aria-hidden="true">
            <div class="history-progress-fill" style="width: ${progressPercent}%;"></div>
          </div>
          <span class="history-progress-caption">${progressPercent}% tuntas (${answeredQ}/${totalQ})</span>
        </div>
      `
        : "";

      const statusBadge = isCompleted
        ? `
        <span class="history-status-tag history-status-tag--completed">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="20 6 9 17 4 12"></polyline></svg>
          Selesai
        </span>
      `
        : `
        <span class="history-status-tag history-status-tag--pending">
          <span class="history-pulse-dot" aria-hidden="true"></span>
          Sedang Berjalan
        </span>
      `;

      const actionButton = isCompleted
        ? `
        <button class="history-card__btn history-card__btn--review" type="button" aria-label="Buka review sesi ${escapeHTML(item.level)}">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"></path>
            <circle cx="12" cy="12" r="3"></circle>
          </svg>
          Buka Review
        </button>
      `
        : `
        <button class="history-card__btn history-card__btn--resume" type="button" aria-label="Lanjutkan sesi latihan ${escapeHTML(item.level)}">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" stroke="none" aria-hidden="true">
            <polygon points="5 3 19 12 5 21 5 3"></polygon>
          </svg>
          Lanjutkan
        </button>
      `;

      return `
        <article class="history-row history-card" tabindex="0" role="button" data-session-id="${escapeHTML(item.id)}" aria-label="Sesi ${escapeHTML(item.level)} target IELTS ${item.ieltsTarget} ${isCompleted ? 'selesai' : 'sedang berjalan'}">
          <div class="history-card__main">
            <div class="history-card__meta-top">
              ${statusBadge}
              <span class="history-card__date">
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <circle cx="12" cy="12" r="10"></circle>
                  <polyline points="12 6 12 12 16 14"></polyline>
                </svg>
                ${dateFormatted}
              </span>
            </div>

            <div class="history-card__title-row">
              <span class="history-pill history-pill--level">${escapeHTML(item.level)}</span>
              <span class="history-pill history-pill--target">🎯 IELTS ${item.ieltsTarget}</span>
              <span class="history-pill history-pill--source">${escapeHTML(source)}</span>
            </div>

            ${progressSection}
          </div>

          <div class="history-card__action-col">
            ${scoreHTML}
            ${actionButton}
          </div>
        </article>
      `;
    })
    .join("");

  $$(".history-card", list).forEach((card) => {
    const sessionId = card.dataset.sessionId;
    const triggerAction = () => loadSession(sessionId, card);
    card.addEventListener("click", () => triggerAction());
    card.addEventListener("keydown", (e) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        triggerAction();
      }
    });
  });
}

function initHistoryControls() {
  if (historyState.inited) return;
  historyState.inited = true;

  // Filter tabs
  $$(".history-tab").forEach((tab) => {
    tab.addEventListener("click", () => {
      $$(".history-tab").forEach((t) => {
        t.classList.remove("is-active");
        t.setAttribute("aria-selected", "false");
      });
      tab.classList.add("is-active");
      tab.setAttribute("aria-selected", "true");
      historyState.filter = tab.dataset.histFilter || "all";
      renderHistoryView();
    });
  });

  // Search input
  const searchInput = $("#history-search");
  if (searchInput) {
    searchInput.addEventListener("input", (e) => {
      historyState.search = e.target.value;
      renderHistoryView();
    });
  }

  // Sort select
  const sortSelect = $("#history-sort");
  if (sortSelect) {
    sortSelect.addEventListener("change", (e) => {
      historyState.sort = e.target.value;
      renderHistoryView();
    });
  }
}

async function openHistory() {
  const dialog = $("#history-dialog");
  if (!dialog) return;
  initHistoryControls();
  dialog.showModal();
  renderHistorySkeletons();
  try {
    const items = await api("/api/sessions");
    historyState.items = Array.isArray(items) ? items : [];
    updateHistoryMetrics();
    renderHistoryView();
  } catch (error) {
    const list = $("#history-list");
    if (list) {
      list.innerHTML = `
        <div class="history-empty-state">
          <h3 class="history-empty-title">Gagal memuat riwayat</h3>
          <p class="history-empty-desc">${escapeHTML(error.message)}</p>
          <button class="btn btn--soft" type="button" id="hist-retry-load-btn" style="margin-top:0.5rem;">Coba Lagi</button>
        </div>
      `;
      const retryBtn = $("#hist-retry-load-btn");
      if (retryBtn) retryBtn.addEventListener("click", openHistory);
    }
  }
}

async function loadSession(id, button) {
  if (button) {
    button.disabled = true;
    button.dataset.state = "loading";
  }
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
    if (button) button.dataset.state = "error";
    showToast(error.message);
  } finally {
    if (button) {
      button.disabled = false;
      if (button.dataset.state !== "error") delete button.dataset.state;
    }
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

document.addEventListener("click", async (event) => {
  const action = event.target.closest("[data-action]")?.dataset.action;
  if (action === "home") showSetup();
  if (action === "mock" || action === "learning") {
    try {
      await openFeature(action === "mock" ? "mock_test" : "learning");
    } catch (error) {
      showToast(error.message);
    }
  }
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
