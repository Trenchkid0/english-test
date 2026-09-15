(function initErrorPage() {
  // 1. Setup Theme
  const savedTheme = localStorage.getItem("ruang-kata-theme") || (window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
  document.documentElement.dataset.theme = savedTheme;

  const themeBtn = document.getElementById("theme-btn");
  if (themeBtn) {
    themeBtn.addEventListener("click", () => {
      const next = document.documentElement.dataset.theme === "dark" ? "light" : "dark";
      document.documentElement.dataset.theme = next;
      localStorage.setItem("ruang-kata-theme", next);
    });
  }

  // 2. Parse Query Params (Client-Side fallback / SPA direct navigation)
  const params = new URLSearchParams(window.location.search);
  const status = params.get("status") || params.get("code") || document.body.dataset.status;
  const message = params.get("message") || params.get("error");
  const title = params.get("title");
  const detail = params.get("detail");

  const titleEl = document.getElementById("error-title");
  const badgeTextEl = document.getElementById("badge-text");
  const subtitleEl = document.getElementById("error-subtitle");
  const detailEl = document.getElementById("error-detail");
  const pathEl = document.getElementById("request-path");
  const roleRowEl = document.getElementById("role-row");
  const authLinkEl = document.getElementById("auth-link");
  const symbolCaptionEl = document.getElementById("error-symbol-caption");

  // Replace template markers if opened via client script or static file
  if (badgeTextEl && badgeTextEl.textContent.includes("{{STATUS_BADGE}}")) {
    badgeTextEl.textContent = "403 · AKSES DITOLAK";
  }
  if (titleEl && titleEl.textContent.includes("{{TITLE}}")) {
    titleEl.textContent = "Fitur Khusus Admin";
  }
  if (subtitleEl && subtitleEl.textContent.includes("{{MESSAGE}}")) {
    subtitleEl.textContent = "Fitur ini hanya tersedia untuk admin.";
  }
  if (detailEl && detailEl.textContent.includes("{{DETAIL}}")) {
    detailEl.textContent = "Halaman dan katalog bank soal ini hanya dapat diakses oleh akun dengan peran Administrator. Akun Anda saat ini berstatus Learner (Pelajar).";
  }
  if (pathEl && pathEl.textContent.includes("{{REQUEST_PATH}}")) {
    pathEl.textContent = window.location.pathname || "/admin";
  }

  // Override with query parameters if present
  if (status === "401") {
    document.title = "401 Perlu Masuk — Ruang Kata";
    if (badgeTextEl) badgeTextEl.textContent = "401 · PERLU LOGIN";
    if (titleEl) titleEl.textContent = title || "Sesi Belum Aktif";
    if (subtitleEl) subtitleEl.textContent = message || "Silakan masuk untuk melanjutkan.";
    if (detailEl) detailEl.textContent = detail || "Anda perlu masuk ke akun Anda terlebih dahulu untuk membuka halaman ini.";
    if (roleRowEl) roleRowEl.hidden = true;
    if (authLinkEl) authLinkEl.querySelector("span").textContent = "Masuk ke akun";
    if (symbolCaptionEl) symbolCaptionEl.textContent = "Sesi belajar perlu diaktifkan";
  } else if (status === "404") {
    document.title = "404 Tidak Ditemukan — Ruang Kata";
    if (badgeTextEl) badgeTextEl.textContent = "404 · TIDAK DITEMUKAN";
    if (titleEl) titleEl.textContent = title || "Halaman Tidak Ditemukan";
    if (subtitleEl) subtitleEl.textContent = message || "Tautan yang Anda tuju tidak tersedia.";
    if (detailEl) detailEl.textContent = detail || "Periksa kembali URL atau kembali ke halaman beranda latihan.";
    if (roleRowEl) roleRowEl.hidden = true;
    if (authLinkEl) authLinkEl.hidden = true;
    if (symbolCaptionEl) symbolCaptionEl.textContent = "Halaman tidak tersedia";
  } else if (status && status !== "403") {
    if (roleRowEl) roleRowEl.hidden = true;
    if (authLinkEl) authLinkEl.hidden = true;
    if (symbolCaptionEl) symbolCaptionEl.textContent = "Ruang belajar sedang terkendala";
  } else if (message) {
    if (title) titleEl.textContent = title;
    if (message) subtitleEl.textContent = message;
    if (detail) detailEl.textContent = detail;
  }
})();
