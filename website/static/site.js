(function () {
  "use strict";

  var root = document.documentElement;

  function store(key, value) {
    try {
      if (value === null) localStorage.removeItem(key);
      else localStorage.setItem(key, value);
    } catch (e) {}
  }

  function load(key) {
    try { return localStorage.getItem(key); } catch (e) { return null; }
  }

  // Theme: system -> light -> dark -> system. No stored value means system.
  var themeButton = document.querySelector(".theme");
  function applyTheme(choice) {
    if (choice === "light" || choice === "dark") root.dataset.theme = choice;
    else delete root.dataset.theme;
    if (!themeButton) return;
    var label = themeButton.dataset[choice] || themeButton.dataset.system;
    themeButton.setAttribute("aria-label", label);
    themeButton.title = label;
  }
  if (themeButton) {
    applyTheme(load("quosparr-theme") || "system");
    themeButton.addEventListener("click", function () {
      var next = { system: "light", light: "dark", dark: "system" }[root.dataset.theme || "system"];
      store("quosparr-theme", next === "system" ? null : next);
      applyTheme(next);
    });
  }

  // An explicit language choice stops the root page from redirecting again.
  document.querySelectorAll("a[data-lang]").forEach(function (link) {
    link.addEventListener("click", function () {
      store("quosparr-lang", link.dataset.lang);
      link.href = link.getAttribute("href").split("#")[0] + location.hash;
    });
  });

  var menu = document.querySelector(".lang");
  if (menu) {
    document.addEventListener("click", function (event) {
      if (menu.open && !menu.contains(event.target)) menu.open = false;
    });
    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && menu.open) {
        menu.open = false;
        menu.querySelector("summary").focus();
      }
    });
  }

  // Hero panel replica: provider tabs, chart views and the reset clock.
  document.querySelectorAll("[data-app]").forEach(function (app) {
    var tabs = Array.prototype.slice.call(app.querySelectorAll('[role="tab"]'));
    function select(tab) {
      tabs.forEach(function (t) {
        var on = t === tab;
        t.setAttribute("aria-selected", on ? "true" : "false");
        t.tabIndex = on ? 0 : -1;
        document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
      });
    }
    tabs.forEach(function (tab, i) {
      tab.addEventListener("click", function () { select(tab); });
      tab.addEventListener("keydown", function (event) {
        var step = { ArrowLeft: -1, ArrowRight: 1 }[event.key];
        var next = step ? tabs[(i + step + tabs.length) % tabs.length]
          : event.key === "Home" ? tabs[0] : event.key === "End" ? tabs[tabs.length - 1] : null;
        if (!next) return;
        event.preventDefault();
        select(next);
        next.focus();
      });
    });

    // Chart and reset choices are shared by every provider, as in the app.
    app.querySelectorAll(".seg button").forEach(function (button) {
      button.addEventListener("click", function () {
        var view = button.dataset.view;
        app.querySelectorAll(".seg button").forEach(function (b) {
          b.setAttribute("aria-pressed", b.dataset.view === view ? "true" : "false");
        });
        app.querySelectorAll(".chart").forEach(function (chart) {
          chart.hidden = chart.dataset.view !== view;
        });
      });
    });
    app.querySelectorAll(".reset").forEach(function (button) {
      button.addEventListener("click", function () { app.classList.toggle("exact"); });
    });
  });

  // Recommend a download. Architecture is only trusted when the browser
  // states it; otherwise the hero button jumps to the platform card.
  function detectOS() {
    var platform = (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || "";
    var ua = navigator.userAgent;
    if (/iPhone|iPad|iPod|Android/i.test(ua)) return "";
    if (/Mac/i.test(platform) || /Macintosh/.test(ua)) return "mac";
    if (/Win/i.test(platform) || /Windows/.test(ua)) return "win";
    if (/Linux|X11/i.test(platform) || /Linux|X11/.test(ua)) return "linux";
    return "";
  }

  function macArchFromGPU() {
    try {
      var gl = document.createElement("canvas").getContext("webgl");
      var ext = gl && gl.getExtension("WEBGL_debug_renderer_info");
      var renderer = ext ? gl.getParameter(ext.UNMASKED_RENDERER_WEBGL) : "";
      if (/Apple (M\d|GPU)/i.test(renderer)) return "arm64";
      if (/Intel|AMD|Radeon|NVIDIA/i.test(renderer)) return "x64";
    } catch (e) {}
    return "";
  }

  function detectArch(os) {
    var hints = navigator.userAgentData && navigator.userAgentData.getHighEntropyValues;
    var fallback = function () {
      if (os === "mac") return macArchFromGPU();
      if (os === "linux") {
        if (/aarch64|arm64/i.test(navigator.userAgent)) return "arm64";
        if (/x86_64|amd64/i.test(navigator.userAgent)) return "x64";
      }
      if (os === "win" && /ARM64/i.test(navigator.userAgent)) return "arm64";
      if (os === "win" && /Win64|x64|WOW64/i.test(navigator.userAgent)) return "x64";
      return "";
    };
    if (!hints) return Promise.resolve(fallback());
    return navigator.userAgentData.getHighEntropyValues(["architecture", "bitness"]).then(function (v) {
      if (v.architecture === "arm") return "arm64";
      if (v.architecture === "x86" && v.bitness === "64") return "x64";
      return fallback();
    }, fallback);
  }

  var os = detectOS();
  if (!os) return;
  var card = document.querySelector('.platform[data-os="' + os + '"]');
  if (!card) return;
  card.classList.add("recommended");
  card.parentElement.prepend(card);

  var hero = document.getElementById("hero-download");
  if (!hero) return;
  hero.href = "#" + card.id;
  detectArch(os).then(function (arch) {
    if (!arch) return;
    var link = card.querySelector('[data-asset="' + os + "-" + arch + '"]');
    if (!link) return;
    hero.href = link.href;
    hero.querySelector("span").textContent = hero.dataset.label.replace("{os}", link.dataset.name);
  });
})();
