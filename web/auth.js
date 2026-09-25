/* Hallmark · authentication + account workspace state */

const authState = { user: null };

function setAuthTab(name) {
  $$('[data-auth-tab]').forEach((tab) => {
    const active = tab.dataset.authTab === name;
    tab.setAttribute("aria-selected", String(active));
    tab.tabIndex = active ? 0 : -1;
  });
  $$('[data-auth-panel]').forEach((panel) => { panel.hidden = panel.dataset.authPanel !== name; });
  $(`[data-auth-panel="${name}"] input`)?.focus({ preventScroll: true });
}

function showAuthenticated(user) {
  authState.user = user;
  $("#session-gate").hidden = true;
  $("#auth-shell").hidden = true;
  $("#product-shell").hidden = false;
  $("#account-name").textContent = user.name;
  $("#account-role").textContent = user.role === "admin" ? "Admin" : "Learner";
  $("#account-initial").textContent = user.name.trim().charAt(0).toLocaleUpperCase() || "U";
  $("#bank-tab").hidden = user.role !== "admin";
  $("#admin-page-link").hidden = user.role !== "admin";
  showSetup();
}

function showSignedOut() {
  authState.user = null;
  clearInterval(state.timerID);
  state.session = null;
  $("#session-gate").hidden = true;
  $("#product-shell").hidden = true;
  $("#auth-shell").hidden = false;
  setAuthTab("login");
}

function setAuthFormState(form, stateName, message = "") {
  const button = $('button[type="submit"]', form);
  const status = $('[role="status"]', form);
  button.disabled = stateName === "loading";
  if (stateName) button.dataset.state = stateName; else delete button.dataset.state;
  status.textContent = message;
}

$$('[data-auth-tab]').forEach((tab) => tab.addEventListener("click", () => setAuthTab(tab.dataset.authTab)));

$("#login-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  if (!form.reportValidity()) return;
  setAuthFormState(form, "loading", "Memeriksa akun…");
  try {
    const data = await api("/api/auth/login", { method: "POST", body: JSON.stringify({ email: form.email.value, password: form.password.value }) });
    form.reset();
    setAuthFormState(form, "success", "Berhasil masuk.");
    showAuthenticated(data.user);
  } catch (error) {
    setAuthFormState(form, "error", error.message);
  }
});

$("#register-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  if (!form.reportValidity()) return;
  if (form.password.value !== form.confirmPassword.value) {
    form.confirmPassword.setAttribute("aria-invalid", "true");
    setAuthFormState(form, "error", "Ulangi password belum sama. Ketik password yang persis sama.");
    form.confirmPassword.focus();
    return;
  }
  form.confirmPassword.removeAttribute("aria-invalid");
  setAuthFormState(form, "loading", "Membuat ruang belajar…");
  try {
    const data = await api("/api/auth/register", { method: "POST", body: JSON.stringify({ name: form.name.value, email: form.email.value, password: form.password.value }) });
    form.reset();
    setAuthFormState(form, "success", "Akun berhasil dibuat.");
    showAuthenticated(data.user);
  } catch (error) {
    setAuthFormState(form, "error", error.message);
  }
});

$("#logout-button").addEventListener("click", async (event) => {
  const button = event.currentTarget;
  button.disabled = true;
  button.dataset.state = "loading";
  try {
    await api("/api/auth/logout", { method: "POST" });
    showSignedOut();
  } catch (error) {
    button.dataset.state = "error";
    showToast(error.message);
  } finally {
    button.disabled = false;
    if (button.dataset.state !== "error") delete button.dataset.state;
  }
});

document.addEventListener("auth:expired", () => {
  showSignedOut();
  $('[role="status"]', $("#login-form")).textContent = "Sesi berakhir. Silakan masuk kembali untuk melanjutkan.";
});

async function loadQuestionBank() {
  const target = $("#bank-stats");
  target.innerHTML = '<p class="empty-copy">Memuat bank soal…</p>';
  try {
    const data = await api("/api/question-bank");
    target.innerHTML = data.stats.length ? data.stats.map((item) => `<article class="bank-stat"><span>${escapeHTML(item.level)} · ${escapeHTML(typeNames[item.type] || item.type)}</span><strong>${item.count}</strong><small>${escapeHTML(item.source)}</small></article>`).join("") : '<p class="empty-copy">Bank masih kosong. Tambahkan paket soal pertama.</p>';
  } catch (error) {
    target.innerHTML = `<p class="empty-copy">${escapeHTML(error.message)}</p>`;
  }
}

$("#refresh-bank").addEventListener("click", loadQuestionBank);
$("#bank-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const button = $('button[type="submit"]', form);
  const types = $$('input[name="types"]:checked', form).map((input) => input.value);
  if (!types.length) {
    $("#bank-status").textContent = "Pilih minimal satu tipe soal.";
    return;
  }
  button.disabled = true;
  button.dataset.state = "loading";
  $("#bank-status").textContent = "Membuat dan memeriksa paket soal…";
  try {
    const data = await api("/api/question-bank/generate", { method: "POST", body: JSON.stringify({ level: form.level.value, ieltsTarget: Number(form.ieltsTarget.value), count: Number(form.count.value), durationMinutes: 20, types }) });
    button.dataset.state = "success";
    const successMessage = `${data.added} dari ${data.generated} soal baru disimpan · sumber ${data.source}.`;
    $("#bank-status").textContent = successMessage;
    showToast(successMessage, 2000);
    await loadQuestionBank();
  } catch (error) {
    button.dataset.state = "error";
    $("#bank-status").textContent = error.message;
  } finally {
    button.disabled = false;
  }
});

document.addEventListener("learning:tool", (event) => {
  if (event.detail?.name === "bank") loadQuestionBank();
});

(async function bootstrapAuth() {
  try {
    const data = await api("/api/auth/me");
    showAuthenticated(data.user);
  } catch (error) {
    showSignedOut();
  }
})();
