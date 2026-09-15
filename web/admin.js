/* Hallmark · admin question catalogue behaviour */

const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];
const typeLabels = {
  grammar: "Grammar", vocabulary: "Vocabulary", reading: "Reading",
  fill_blank: "Fill blank", fill_in_blank: "Fill blank", fill_in_the_blank: "Fill blank",
  listening: "Listening", error_identification: "Error identification",
};
const catalogueState = { page: 1, pages: 0, total: 0, items: [], request: null };

function escapeHTML(value = "") {
  return String(value).replace(/[&<>'"]/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;",
  })[character]);
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
    throw error;
  }
  return data;
}

function setTheme(theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem("ruang-kata-theme", theme);
}

function restoreFilters() {
  const params = new URLSearchParams(location.search);
  const form = $("#filter-form");
  ["query", "level", "target", "type", "source", "status", "sort", "pageSize"].forEach((name) => {
    const value = params.get(name);
    if (value !== null && form.elements[name]) form.elements[name].value = value;
  });
  catalogueState.page = Math.max(1, Number(params.get("page")) || 1);
}

function requestParams() {
  const data = new FormData($("#filter-form"));
  const params = new URLSearchParams();
  for (const [key, rawValue] of data) {
    const value = String(rawValue).trim();
    if (value && !(key === "status" && value === "all") && !(key === "sort" && value === "desc")) params.set(key, value);
  }
  params.set("page", String(catalogueState.page));
  return params;
}

function syncURL(params) {
  history.replaceState(null, "", `${location.pathname}?${params.toString()}`);
}

function loadingCards() {
  $("#question-grid").innerHTML = '<div class="question-skeleton"></div><div class="question-skeleton"></div><div class="question-skeleton"></div><div class="question-skeleton"></div>';
  $("#question-grid").setAttribute("aria-busy", "true");
}

function renderSources(sources) {
  const select = $("#source-filter");
  const selected = select.value || new URLSearchParams(location.search).get("source") || "";
  select.innerHTML = '<option value="">Semua sumber</option>' + sources.map((source) => `<option value="${escapeHTML(source)}">${escapeHTML(source)}</option>`).join("");
  if ([...select.options].some((option) => option.value === selected)) select.value = selected;
}

function questionCard(item, index) {
  const question = item.question || {};
  const prompt = String(question.prompt || "Soal tanpa prompt").replace(/ \[Level .*?Practice \d+\]$/, "");
  const formattedDate = item.createdAt ? new Date(item.createdAt).toLocaleDateString("id-ID", { day: "numeric", month: "short", year: "numeric" }) : "";
  return `<article class="question-card">
    <div class="card-meta"><span>${escapeHTML(item.level)} · IELTS ${escapeHTML(item.ieltsTarget)}</span><span class="status-dot ${item.active ? "is-active" : "is-inactive"}">${item.active ? "Aktif" : "Nonaktif"}</span></div>
    <h3>${escapeHTML(prompt)}</h3>
    <div class="card-foot"><span>${escapeHTML(typeLabels[item.type] || item.type)} · ${escapeHTML(item.source)}${formattedDate ? ` · <time datetime="${escapeHTML(item.createdAt)}">${escapeHTML(formattedDate)}</time>` : ""}</span><button class="detail-button" type="button" data-detail-index="${index}">Buka detail</button></div>
  </article>`;
}

function renderCatalogue(data) {
  catalogueState.page = data.page;
  catalogueState.pages = data.pages;
  catalogueState.total = data.total;
  catalogueState.items = data.items || [];
  renderSources(data.sources || []);
  const grid = $("#question-grid");
  grid.setAttribute("aria-busy", "false");
  if (!catalogueState.items.length) {
    grid.innerHTML = '<div class="empty-state"><span aria-hidden="true">0</span><div><h3>Tidak ada soal yang cocok.</h3><p>Ubah kata pencarian atau reset filter untuk melihat katalog lengkap.</p></div><button class="button button--quiet" type="button" data-reset-empty>Reset filter</button></div>';
  } else {
    grid.innerHTML = catalogueState.items.map(questionCard).join("");
  }
  const first = data.total ? ((data.page - 1) * data.pageSize) + 1 : 0;
  const last = Math.min(data.page * data.pageSize, data.total);
  $("#result-summary").textContent = `${data.total.toLocaleString("id-ID")} soal · menampilkan ${first.toLocaleString("id-ID")}–${last.toLocaleString("id-ID")}`;
  $("#page-status").textContent = data.pages ? `Halaman ${data.page.toLocaleString("id-ID")} dari ${data.pages.toLocaleString("id-ID")}` : "Tidak ada halaman";
  $("#previous-page").disabled = data.page <= 1;
  $("#next-page").disabled = data.pages === 0 || data.page >= data.pages;
}

function renderLoadError(error) {
  const grid = $("#question-grid");
  grid.setAttribute("aria-busy", "false");
  if (error.status === 401 || error.status === 403) {
    location.replace(`/error.html?status=${error.status}&message=${encodeURIComponent(error.message)}`);
    return;
  }
  grid.innerHTML = `<div class="empty-state empty-state--error"><span aria-hidden="true">!</span><div><h3>Daftar soal belum dapat dimuat.</h3><p>${escapeHTML(error.message)} Muat ulang untuk mencoba lagi.</p></div><button class="button button--quiet" type="button" data-retry>Muat ulang</button></div>`;
  $("#result-summary").textContent = "Pemuatan gagal.";
}

async function loadQuestions() {
  catalogueState.request?.abort();
  const controller = new AbortController();
  catalogueState.request = controller;
  const params = requestParams();
  syncURL(params);
  loadingCards();
  $("#refresh-button").disabled = true;
  $("#refresh-button").dataset.state = "loading";
  try {
    const data = await api(`/api/admin/questions?${params.toString()}`, { signal: controller.signal });
    renderCatalogue(data);
  } catch (error) {
    if (error.name !== "AbortError") renderLoadError(error);
  } finally {
    if (catalogueState.request === controller) {
      $("#refresh-button").disabled = false;
      delete $("#refresh-button").dataset.state;
    }
  }
}

function detailSection(label, value) {
  if (value === undefined || value === null || value === "") return "";
  return `<section><h3>${escapeHTML(label)}</h3><p>${escapeHTML(value)}</p></section>`;
}

function openDetail(index) {
  const item = catalogueState.items[index];
  if (!item) return;
  const q = item.question || {};
  $("#detail-meta").textContent = `DATABASE #${item.databaseId} · ${item.level} · IELTS ${item.ieltsTarget}`;
  $("#detail-title").textContent = q.prompt || "Soal tanpa prompt";
  const choices = (q.choices || []).map((choice, choiceIndex) => `<li class="${choiceIndex === q.correctIndex ? "is-correct" : ""}"><span>${String.fromCharCode(65 + choiceIndex)}</span><p>${escapeHTML(choice)}</p>${choiceIndex === q.correctIndex ? "<strong>Jawaban benar</strong>" : ""}</li>`).join("");
  $("#detail-body").innerHTML = `
    <dl class="detail-facts"><div><dt>Tipe</dt><dd>${escapeHTML(typeLabels[item.type] || item.type)}</dd></div><div><dt>Sumber</dt><dd>${escapeHTML(item.source)}</dd></div><div><dt>Status</dt><dd>${item.active ? "Aktif" : "Nonaktif"}</dd></div><div><dt>Dibuat</dt><dd>${escapeHTML(new Date(item.createdAt).toLocaleString("id-ID"))}</dd></div></dl>
    ${detailSection("Konteks", q.context)}
    ${detailSection("Bukti", q.evidence)}
    <section><h3>Pilihan jawaban</h3><ol class="detail-choices">${choices || "<li>Tidak ada pilihan jawaban.</li>"}</ol></section>
    ${detailSection("Penjelasan", q.explanation)}
    ${detailSection("Tips belajar", q.learningTip)}
    ${detailSection("IELTS skill", q.ieltsSkill)}
    ${detailSection("Error tag", q.errorTag)}
    ${detailSection("Question ID", q.id)}`;
  $("#detail-dialog").showModal();
  $("#close-detail").focus({ preventScroll: true });
}

$("#filter-form").addEventListener("submit", (event) => {
  event.preventDefault();
  catalogueState.page = 1;
  loadQuestions();
});
$("#reset-filter").addEventListener("click", () => {
  $("#filter-form").reset();
  catalogueState.page = 1;
  loadQuestions();
});
$("#refresh-button").addEventListener("click", loadQuestions);
function movePage(change) {
  catalogueState.page += change;
  loadQuestions();
  scrollTo({ top: 0, behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });
}
$("#previous-page").addEventListener("click", () => movePage(-1));
$("#next-page").addEventListener("click", () => movePage(1));
$("#question-grid").addEventListener("click", (event) => {
  const detailButton = event.target.closest("[data-detail-index]");
  if (detailButton) openDetail(Number(detailButton.dataset.detailIndex));
  if (event.target.closest("[data-reset-empty]")) $("#reset-filter").click();
  if (event.target.closest("[data-retry]")) loadQuestions();
});
$("#close-detail").addEventListener("click", () => $("#detail-dialog").close());
$("#detail-dialog").addEventListener("click", (event) => { if (event.target === event.currentTarget) event.currentTarget.close(); });
$("#theme-button").addEventListener("click", () => setTheme(document.documentElement.dataset.theme === "dark" ? "light" : "dark"));
$("#logout-button").addEventListener("click", async (event) => {
  event.currentTarget.disabled = true;
  try { await api("/api/auth/logout", { method: "POST" }); } finally { location.replace("/"); }
});
document.addEventListener("keydown", (event) => {
  if ((event.ctrlKey || event.metaKey) && event.key.toLocaleLowerCase() === "k") {
    event.preventDefault();
    $("#question-search").focus({ preventScroll: true });
    $("#question-search").scrollIntoView({ block: "center", behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });
  }
});

(async function bootstrapAdmin() {
  setTheme(localStorage.getItem("ruang-kata-theme") || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"));
  try {
    const data = await api("/api/auth/me");
    if (data.user.role !== "admin") {
      location.replace("/error.html?status=403&message=Fitur+ini+hanya+tersedia+untuk+admin.");
      return;
    }
    $("#account-name").textContent = data.user.name;
    $("#account-initial").textContent = data.user.name.trim().charAt(0).toLocaleUpperCase() || "A";
    restoreFilters();
    $("#admin-gate").hidden = true;
    $("#admin-shell").hidden = false;
    await loadQuestions();
  } catch (error) {
    location.replace(`/error.html?status=${error.status || 401}&message=${encodeURIComponent(error.message || "Silakan masuk untuk melanjutkan.")}`);
  }
})();
