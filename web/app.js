(async function () {
  // ---------- i18n ----------
  let i18n = { en: {}, zh: {} };
  try {
    const r = await fetch('/i18n.json', { cache: 'no-cache' });
    if (r.ok) i18n = await r.json();
  } catch (e) {
    console.warn('i18n load failed:', e);
  }

  function format(s, args) {
    if (!args || !args.length) return s;
    return s.replace(/\{(\d+)\}/g, (m, i) => {
      const idx = parseInt(i, 10);
      return args[idx] !== undefined ? args[idx] : m;
    });
  }

  function tr(key, args, fallback) {
    if (!key) return '';
    let s = (i18n[lang] || {})[key];
    if (s === undefined) s = (i18n.en || {})[key];
    if (s === undefined) s = fallback;
    if (s === undefined) s = key;
    return format(s, args);
  }

  // ---------- terminal ----------
  const term = new Terminal({
    cursorBlink: false,
    fontFamily: 'Menlo, Consolas, "Cascadia Mono", monospace',
    fontSize: 14,
    theme: {
      background: '#0b0f12',
      foreground: '#c9d1d9',
      cursor: '#2ea043',
      selectionBackground: '#264f78'
    },
    convertEol: true,
    scrollback: 0
  });

  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(document.getElementById('term'));
  fit.fit();
  window.addEventListener('resize', () => {
    fit.fit();
    if (screen) paint();
  });
  term.focus();

  // ---------- language ----------
  const LANG_KEY = 'bbs_lang';
  function detectInitialLang() {
    const saved = localStorage.getItem(LANG_KEY);
    if (saved === 'zh' || saved === 'en') return saved;
    const nav = (navigator.language || 'en').toLowerCase();
    if (nav.startsWith('zh')) return 'zh';
    return 'en';
  }
  function saveLang(code) {
    if (code === 'zh' || code === 'en') localStorage.setItem(LANG_KEY, code);
  }

  let ws = null;
  let nextId = 1;
  let connected = false;
  let quitting = false;

  let screen = null;
  let lang = detectInitialLang();
  let selected = 0;
  let menuOffset = 0;

  let readLines = [];
  let readOffset = 0;
  let readVisibleRows = 10;

  const overlay = document.getElementById('editor-overlay');
  const overlayTitle = document.getElementById('editor-title');
  const overlayFields = document.getElementById('editor-fields');
  const overlayHint = document.getElementById('editor-hint');
  const btnCancel = document.getElementById('editor-cancel');
  const btnSubmit = document.getElementById('editor-submit');
  const btnDraft = document.getElementById('editor-draft');
  let editorForm = null;

  // ---------- 宽度工具 ----------
  const ANSI_RE = /\x1b\[[0-9;]*[mK]/g;

  function charWidth(code) {
    if (code < 0x20) return 0;
    if (
      (code >= 0x1100 && code <= 0x115F) ||
      (code >= 0x2E80 && code <= 0x303E) ||
      (code >= 0x3041 && code <= 0x33FF) ||
      (code >= 0x3400 && code <= 0x4DBF) ||
      (code >= 0x4E00 && code <= 0x9FFF) ||
      (code >= 0xA000 && code <= 0xA4CF) ||
      (code >= 0xAC00 && code <= 0xD7A3) ||
      (code >= 0xF900 && code <= 0xFAFF) ||
      (code >= 0xFE30 && code <= 0xFE4F) ||
      (code >= 0xFF00 && code <= 0xFF60) ||
      (code >= 0xFFE0 && code <= 0xFFE6) ||
      (code >= 0x1F300 && code <= 0x1F64F) ||
      (code >= 0x1F900 && code <= 0x1F9FF) ||
      (code >= 0x20000 && code <= 0x3FFFD)
    ) return 2;
    return 1;
  }

  function displayWidth(s) {
    const plain = s.replace(ANSI_RE, '');
    let w = 0;
    for (const ch of plain) w += charWidth(ch.codePointAt(0));
    return w;
  }

  function padRight(s, width) {
    const w = displayWidth(s);
    if (w >= width) return s;
    return s + ' '.repeat(width - w);
  }

  function fitWidth(s, width) {
    const parts = [];
    let last = 0;
    let m;
    ANSI_RE.lastIndex = 0;
    while ((m = ANSI_RE.exec(s)) !== null) {
      parts.push({ text: s.slice(last, m.index), ansi: false });
      parts.push({ text: m[0], ansi: true });
      last = m.index + m[0].length;
    }
    if (last < s.length) parts.push({ text: s.slice(last), ansi: false });

    let w = 0;
    let out = '';
    for (const p of parts) {
      if (p.ansi) { out += p.text; continue; }
      for (const ch of p.text) {
        const cw = charWidth(ch.codePointAt(0));
        if (w + cw > width) return out + '\x1b[0m';
        out += ch;
        w += cw;
      }
    }
    return out;
  }

  // ---------- 内容渲染 ----------
  const KIND_COLOR = {
    err: '\x1b[31m',
    ok: '\x1b[32m',
    info: '\x1b[36m',
    author: '\x1b[90m',
    h1: '\x1b[1;36m',
    h2: '\x1b[1;33m',
    quote: '\x1b[32m',
    code: '\x1b[90m',
    hr: '\x1b[90m'
  };

  const SPAN_STYLE = {
    bold:   ['\x1b[1m', '\x1b[0m'],
    italic: ['\x1b[3m', '\x1b[0m'],
    code:   ['\x1b[36m', '\x1b[0m'],
    link:   ['\x1b[4;34m', '\x1b[0m']
  };

  function renderLine(line) {
    const color = KIND_COLOR[line.kind] || '';
    const reset = color ? '\x1b[0m' : '';
    if (line.spans && line.spans.length) {
      let out = color;
      for (const sp of line.spans) {
        const st = SPAN_STYLE[sp.style] || ['', ''];
        out += st[0] + sp.text + st[1];
      }
      return out + reset;
    }
    let text;
    if (line.key) text = tr(line.key, line.args, line.default);
    else text = line.raw || '';
    return color + text + reset;
  }

  function menuLabel(it) {
    const key = it.key ? '[' + it.key.toUpperCase() + '] ' : '';
    const label = it.label_key ? tr(it.label_key, null, it.default) : (it.label || '');
    return { key, label };
  }

  function renderMenuLine(it, isSelected) {
    const { key, label } = menuLabel(it);
    if (isSelected) {
      return '\x1b[1;30;42m ▶ ' + key + label + '\x1b[0m';
    }
    return '   ' + key + label;
  }

  // ---------- 布局 ----------
  function layout() {
    return { cols: term.cols, rows: term.rows };
  }

  function clear() { term.write('\x1b[2J\x1b[H'); }

  function headerText(sc) {
    return '\x1b[1;32m  RETRO BBS\x1b[0m  \x1b[90m· ' +
      tr(sc.title, null, sc.title_default) + '\x1b[0m';
  }

  function statusBarText() {
    const hints = [
      ['↑↓', tr('ui.hint.move', null, 'move')],
      ['Enter', tr('ui.hint.open', null, 'open')],
      ['Esc', tr('ui.hint.back', null, 'back')],
      ['Q', tr('ui.hint.quit', null, 'quit')]
    ];
    return '\x1b[90m  ' + hints.map(h => h[0] + ' ' + h[1]).join('   ') + '\x1b[0m';
  }

  // ---------- 各屏渲染 ----------
  function paint() {
    if (!screen) return;
    const { cols, rows } = layout();
    clear();

    term.write(headerText(screen) + '\r\n\r\n');

    if (screen.form) {
      term.write('\x1b[90m  (editing in overlay...)\x1b[0m\r\n');
      paintStatus();
      return;
    }

    if (screen.screen === 'thread_read') {
      paintThreadRead(cols, rows);
      return;
    }

    if (screen.screen === 'thread_list') {
      paintThreadList(cols, rows);
      return;
    }

    paintSinglePane(cols, rows);
  }

  function paintStatus() {
    term.write('\r\n' + statusBarText() + '\r\n');
  }

  function paintSinglePane(cols, rows) {
    const { innerWidth, contentRows } = singlePaneLayout(cols, rows);

    term.write('  \x1b[90m╭' + '─'.repeat(innerWidth) + '╮\x1b[0m\r\n');

    const lines = [];
    for (const l of (screen.lines || [])) lines.push(renderLine(l));
    lines.push('');

    const menu = screen.menu || [];
    if (menu.length > 0) {
      const menuAreaRows = contentRows - lines.length - 1;
      const visible = Math.max(1, menuAreaRows);
      if (menu.length > visible) {
        if (selected < menuOffset) menuOffset = selected;
        if (selected >= menuOffset + visible) menuOffset = selected - visible + 1;
        const maxOff = menu.length - visible;
        if (menuOffset > maxOff) menuOffset = maxOff;
        if (menuOffset < 0) menuOffset = 0;
      } else {
        menuOffset = 0;
      }
      const end = Math.min(menu.length, menuOffset + visible);
      for (let i = menuOffset; i < end; i++) {
        lines.push(renderMenuLine(menu[i], i === selected));
      }
      if (menu.length > visible) {
        lines.push('\x1b[90m   ' + (menuOffset + 1) + '-' + end + '/' + menu.length +
          '  j/k move · h/l page\x1b[0m');
      }
    }

    for (let i = 0; i < contentRows; i++) {
      const line = lines[i] || '';
      const fitted = fitWidth(line, innerWidth);
      const padded = padRight(fitted, innerWidth);
      term.write('  \x1b[90m│\x1b[0m' + padded + '\x1b[90m│\x1b[0m\r\n');
    }

    term.write('  \x1b[90m╰' + '─'.repeat(innerWidth) + '╯\x1b[0m\r\n');
    paintStatus();
  }

  function singlePaneLayout(cols, rows) {
    const margin = 2;
    const topAndHeader = 2;
    const bottomBorder = 1;
    const statusBar = 1;
    const blankBeforeStatus = 1;
    const contentRows = Math.max(1, rows - topAndHeader - 2 - bottomBorder - statusBar - blankBeforeStatus);
    const innerWidth = Math.max(20, cols - margin * 2 - 2);
    return { innerWidth, contentRows };
  }

  function paintThreadList(cols, rows) {
    const { leftWidth, rightWidth, contentRows } = threadListLayout(cols, rows);

    term.write(
      '  \x1b[90m╭' + '─'.repeat(leftWidth) + '╮\x1b[0m  ' +
      '\x1b[42m╭' + '─'.repeat(rightWidth) + '╮\x1b[0m\r\n'
    );

    const boards = (screen.lines && screen.lines.find(l => l.kind === 'h1')) || null;
    const currentBoard = boards ? boards.raw : '';

    const leftContent = [
      '\x1b[1;36m' + (currentBoard || '') + '\x1b[0m'
    ];

    const menu = screen.menu || [];
    const visible = contentRows;
    if (menu.length > visible) {
      if (selected < menuOffset) menuOffset = selected;
      if (selected >= menuOffset + visible) menuOffset = selected - visible + 1;
      const maxOff = menu.length - visible;
      if (menuOffset > maxOff) menuOffset = maxOff;
      if (menuOffset < 0) menuOffset = 0;
    } else {
      menuOffset = 0;
    }
    const end = Math.min(menu.length, menuOffset + visible);
    const rightContent = [];
    for (let i = menuOffset; i < end; i++) {
      rightContent.push(renderMenuLine(menu[i], i === selected));
    }
    if (menu.length > visible) {
      rightContent.push('\x1b[90m' + (menuOffset + 1) + '-' + end + '/' + menu.length +
        '  j/k · h/l\x1b[0m');
    }

    for (let i = 0; i < contentRows; i++) {
      const l = padRight(fitWidth(leftContent[i] || '', leftWidth), leftWidth);
      const r = padRight(fitWidth(rightContent[i] || '', rightWidth), rightWidth);
      term.write(
        '  \x1b[90m│\x1b[0m' + l + '\x1b[90m│\x1b[0m  ' +
        '\x1b[42m│\x1b[0m' + r + '\x1b[42m│\x1b[0m\r\n'
      );
    }

    term.write(
      '  \x1b[90m╰' + '─'.repeat(leftWidth) + '╯\x1b[0m  ' +
      '\x1b[42m╰' + '─'.repeat(rightWidth) + '╯\x1b[0m\r\n'
    );
    paintStatus();
  }

  function threadListLayout(cols, rows) {
    const margin = 2;
    const gap = 2;
    const leftInner = Math.min(24, Math.max(14, Math.floor(cols * 0.22)));
    const rightInner = Math.max(20, cols - margin * 2 - leftInner - gap - 4);
    const contentRows = Math.max(1, rows - 2 - 2 - 1 - 1);
    return { leftWidth: leftInner, rightWidth: rightInner, contentRows };
  }

  function paintThreadRead(cols, rows) {
    const contentTop = 2;
    const statusBar = 1;
    const menuRow = 1;
    const blankBeforeMenu = 1;
    const visible = Math.max(1, rows - contentTop - statusBar - menuRow - blankBeforeMenu);
    readVisibleRows = visible;

    const maxOff = Math.max(0, readLines.length - visible);
    if (readOffset > maxOff) readOffset = maxOff;
    if (readOffset < 0) readOffset = 0;

    const end = Math.min(readLines.length, readOffset + visible);
    for (let i = readOffset; i < end; i++) {
      const line = readLines[i] || '';
      const fitted = fitWidth(line, cols - 2);
      term.write('  ' + fitted + '\r\n');
    }
    for (let i = end; i < readOffset + visible; i++) {
      term.write('\r\n');
    }

    if (readLines.length > visible) {
      term.write('\x1b[90m  ' + (readOffset + 1) + '/' + readLines.length +
        '  PgUp/PgDn scroll  Home/End\x1b[0m\r\n');
    } else {
      term.write('\r\n');
    }

    const menu = screen.menu || [];
    if (menu.length > 0) {
      const parts = [];
      for (let i = 0; i < menu.length; i++) {
        const { key, label } = menuLabel(menu[i]);
        if (i === selected) {
          parts.push('\x1b[1;30;42m ' + key + label + ' \x1b[0m');
        } else {
          parts.push('\x1b[90m' + key + label + '\x1b[0m');
        }
      }
      term.write('  ' + parts.join('  ') + '\r\n');
    } else {
      term.write('\r\n');
    }

    term.write(statusBarText() + '\r\n');
  }

  // ---------- WebSocket ----------
  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/ws`);

    ws.onopen = () => { connected = true; term.focus(); };

    ws.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      if (msg.type === 'screen') {
        handleScreen(msg.data || null);
      } else if (msg.type === 'output') {
        showToast(msg.text || '');
      } else if (msg.type === 'response') {
        if (msg.ok === false && msg.error) showToast('[error ' + msg.error.code + '] ' + msg.error.message);
        if (msg.data && msg.data.draft_id && editorForm) editorForm.draftId = msg.data.draft_id;
      }
    };

    ws.onclose = () => {
      connected = false;
      closeEditor(false);
      if (quitting) {
        quitting = false;
        paint();
        setTimeout(connect, 400);
        return;
      }
      clear();
      term.write('\x1b[90m[disconnected] press Enter to reconnect\x1b[0m\r\n');
    };
  }

  let toastText = '';
  let toastTimer = null;
  function showToast(text) {
    toastText = text.replace(/[\r\n]+/g, ' ').trim();
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { toastText = ''; }, 3000);
    paint();
  }

  function handleScreen(sc) {
    screen = sc;
    selected = 0;
    menuOffset = 0;

    if (sc && sc.screen === 'thread_read') {
      readLines = [];
      for (const l of (sc.lines || [])) readLines.push(renderLine(l));
      readOffset = 0;
    }

    if (screen && screen.form) {
      openEditor(screen);
    } else {
      closeEditor(false);
    }
    paint();
  }

  function sendAction(action, value) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({
      id: nextId++,
      type: 'action',
      data: { action, value: value || '' }
    }));
  }
  function sendCommand(name) { sendAction('command', name); }
  function sendQuit() { quitting = true; sendAction('quit'); }

  function switchLang(code) {
    if (code !== 'zh' && code !== 'en') return;
    if (lang === code) return;
    lang = code;
    saveLang(code);
    if (screen && screen.screen === 'thread_read') {
      readLines = [];
      for (const l of (screen.lines || [])) readLines.push(renderLine(l));
    }
    paint();
  }
  function toggleLang() { switchLang(lang === 'zh' ? 'en' : 'zh'); }

  function activateSelected() {
    if (!screen || !screen.menu || screen.menu.length === 0) return;
    const it = screen.menu[selected];
    if (!it) return;
    if (it.action === 'lang') { toggleLang(); return; }
    if (it.action && it.action.startsWith('lang:')) { switchLang(it.action.slice(5)); return; }
    if (it.action === 'quit') { sendQuit(); return; }
    sendAction('select', it.action);
  }

  // ---------- 菜单操作 ----------
  function menuLen() { return screen && screen.menu ? screen.menu.length : 0; }

  function visibleMenuRows() {
    const { rows } = layout();
    if (screen && screen.screen === 'thread_list') {
      return Math.max(1, rows - 6);
    }
    return Math.max(1, rows - 8);
  }

  function moveUp() { if (menuLen() === 0) return; selected = (selected - 1 + menuLen()) % menuLen(); paint(); }
  function moveDown() { if (menuLen() === 0) return; selected = (selected + 1) % menuLen(); paint(); }
  function pageUp() { if (menuLen() === 0) return; selected = Math.max(0, selected - visibleMenuRows()); paint(); }
  function pageDown() { if (menuLen() === 0) return; selected = Math.min(menuLen() - 1, selected + visibleMenuRows()); paint(); }
  function menuTop() { if (menuLen() === 0) return; selected = 0; paint(); }
  function menuBottom() { if (menuLen() === 0) return; selected = menuLen() - 1; paint(); }
  function menuHalfUp() { if (menuLen() === 0) return; selected = Math.max(0, selected - Math.floor(visibleMenuRows() / 2)); paint(); }
  function menuHalfDown() { if (menuLen() === 0) return; selected = Math.min(menuLen() - 1, selected + Math.floor(visibleMenuRows() / 2)); paint(); }

  function readScroll(delta) {
    readOffset += delta;
    const maxOff = Math.max(0, readLines.length - readVisibleRows);
    if (readOffset > maxOff) readOffset = maxOff;
    if (readOffset < 0) readOffset = 0;
    paint();
  }
  function readTop() { readOffset = 0; paint(); }
  function readBottom() { readOffset = Math.max(0, readLines.length - readVisibleRows); paint(); }

  // ---------- overlay 编辑器 ----------
  function openEditor(sc) {
    editorForm = {
      screen: sc.screen,
      submit: sc.form.submit,
      canDraft: !!sc.form.can_draft,
      fields: [],
      draftId: ''
    };
    overlayTitle.textContent = tr(sc.title, null, sc.title_default) + '  —  ' + sc.screen;
    overlayHint.textContent = tr(sc.hint, null, sc.hint_default);
    overlayFields.innerHTML = '';

    sc.form.fields.forEach((f) => {
      if (!f.label_key) { editorForm.draftId = f.value || ''; return; }
      const wrap = document.createElement('div');
      wrap.className = 'field';

      const label = document.createElement('label');
      label.textContent = tr(f.label_key, null, f.default);
      label.htmlFor = 'f_' + f.name;
      wrap.appendChild(label);

      let input;
      if (f.multiline) {
        input = document.createElement('textarea');
        input.rows = 12;
      } else {
        input = document.createElement('input');
        input.type = f.secret ? 'password' : 'text';
      }
      input.id = 'f_' + f.name;
      input.name = f.name;
      input.value = f.value || '';
      input.autocomplete = 'off';
      input.spellcheck = false;
      wrap.appendChild(input);

      if (f.multiline) {
        const meta = document.createElement('div');
        meta.className = 'meta';
        const update = () => { meta.textContent = input.value.length + ' bytes'; };
        input.addEventListener('input', update);
        update();
        wrap.appendChild(meta);
      }

      overlayFields.appendChild(wrap);
      editorForm.fields.push({ name: f.name, input });

      input.addEventListener('keydown', (ev) => {
        const ctrl = ev.ctrlKey || ev.metaKey;
        if (ctrl && ev.key.toLowerCase() === 's') { ev.preventDefault(); saveDraft(); return; }
        if (ctrl && ev.key.toLowerCase() === 'd') { ev.preventDefault(); deleteDraft(); return; }
        if (ctrl && ev.key.toLowerCase() === 'g') { ev.preventDefault(); submitEditor(); return; }
        if (ev.key === 'Tab') {
          ev.preventDefault();
          const all = editorForm.fields.map(x => x.input);
          const i = all.indexOf(input);
          all[(i + (ev.shiftKey ? -1 : 1) + all.length) % all.length].focus();
          return;
        }
        if (ev.key === 'Escape') { ev.preventDefault(); closeEditor(true); return; }
        if (ev.key === 'Enter' && input.tagName !== 'TEXTAREA') {
          ev.preventDefault();
          submitEditor();
        }
      });
    });

    btnDraft.style.display = editorForm.canDraft ? '' : 'none';
    overlay.classList.remove('hidden');
    const first = editorForm.fields[0];
    if (first) {
      first.input.focus();
      if (first.input.value) {
        const n = first.input.value.length;
        try { first.input.setSelectionRange(n, n); } catch (e) {}
      }
    }
  }

  function closeEditor(sendBack) {
    if (overlay.classList.contains('hidden')) return;
    overlay.classList.add('hidden');
    overlayFields.innerHTML = '';
    editorForm = null;
    term.focus();
    if (sendBack) sendAction('back');
  }

  function collectValues() {
    const values = {};
    editorForm.fields.forEach(f => { values[f.name] = f.input.value; });
    if (editorForm.draftId) values['draft_id'] = editorForm.draftId;
    return values;
  }
  function submitEditor() { if (!editorForm) return; sendAction('submit', JSON.stringify(collectValues())); }
  function saveDraft() {
    if (!editorForm || !editorForm.canDraft) return;
    sendAction('save_draft', JSON.stringify({
      screen: editorForm.screen,
      fields: collectValues(),
      draft_id: editorForm.draftId,
      board_id: '', thread_id: ''
    }));
  }
  function deleteDraft() { if (!editorForm || !editorForm.draftId) return; sendCommand('draft_del:' + editorForm.draftId); }

  btnCancel.addEventListener('click', () => closeEditor(true));
  btnSubmit.addEventListener('click', submitEditor);
  btnDraft.addEventListener('click', saveDraft);

  // ---------- 键盘 ----------
  term.onKey(({ domEvent }) => {
    const e = domEvent;

    if (!connected) {
      if (e.key === 'Enter') { e.preventDefault(); connect(); }
      return;
    }
    if (!overlay.classList.contains('hidden')) return;

    if (e.key === 'Delete' || e.code === 'Delete') {
      if (screen && screen.screen === 'draft_list' && screen.menu) {
        e.preventDefault();
        const it = screen.menu[selected];
        if (it && it.action && it.action.startsWith('draft_open:')) {
          sendCommand('draft_del:' + it.action.slice('draft_open:'.length));
        }
      }
      return;
    }

    if (e.key === 'ArrowDown')  { e.preventDefault(); moveDown(); return; }
    if (e.key === 'ArrowUp')    { e.preventDefault(); moveUp(); return; }
    if (e.key === 'ArrowLeft')  { e.preventDefault(); sendAction('back', ''); return; }
    if (e.key === 'ArrowRight') { e.preventDefault(); activateSelected(); return; }
    if (e.key === 'Enter')      { e.preventDefault(); activateSelected(); return; }
    if (e.key === 'Escape')     { e.preventDefault(); sendAction('back', ''); return; }

    if (e.key === 'PageDown') {
      e.preventDefault();
      if (screen && screen.screen === 'thread_read') readScroll(readVisibleRows);
      else pageDown();
      return;
    }
    if (e.key === 'PageUp') {
      e.preventDefault();
      if (screen && screen.screen === 'thread_read') readScroll(-readVisibleRows);
      else pageUp();
      return;
    }
    if (e.key === 'Home') {
      e.preventDefault();
      if (screen && screen.screen === 'thread_read') readTop();
      else menuTop();
      return;
    }
    if (e.key === 'End') {
      e.preventDefault();
      if (screen && screen.screen === 'thread_read') readBottom();
      else menuBottom();
      return;
    }

    if (e.key.length === 1) {
      const k = e.key;
      const lk = k.toLowerCase();
      const hasMenu = screen && screen.menu && screen.menu.length > 0;
      const isThreadList = screen && screen.screen === 'thread_list';
      const isThreadRead = screen && screen.screen === 'thread_read';
      const isConfirm = screen && (screen.screen === 'delete_confirm' || screen.screen === 'purge_confirm');

      // 0. confirm 屏：Y/N 直接触发（优先于一切）
      if (isConfirm) {
        if (lk === 'y') { e.preventDefault(); sendCommand('confirm_yes'); return; }
        if (lk === 'n') { e.preventDefault(); sendCommand('confirm_no'); return; }
        if (e.key === 'Escape') { e.preventDefault(); sendCommand('confirm_no'); return; }
        return;
      }

      // 1. 菜单项匹配（大小写不敏感）
      if (hasMenu) {
        const hit = screen.menu.find(it => it.key && it.key.toLowerCase() === lk);
        if (hit) {
          e.preventDefault();
          if (hit.action === 'lang') { toggleLang(); }
          else if (hit.action && hit.action.startsWith('lang:')) switchLang(hit.action.slice(5));
          else if (hit.action === 'quit') sendQuit();
          else sendAction('select', hit.action);
          return;
        }
      }

      // 2. j/k 全局导航
      if (lk === 'j') { e.preventDefault(); moveDown(); return; }
      if (lk === 'k') { e.preventDefault(); moveUp(); return; }

      // 3. thread_list vim 翻页（不含 G）
      if (isThreadList) {
        if (lk === 'h') { e.preventDefault(); pageUp(); return; }
        if (lk === 'l') { e.preventDefault(); pageDown(); return; }
        if (k === 'g')  { e.preventDefault(); menuTop(); return; }
        if (lk === 'u') { e.preventDefault(); menuHalfUp(); return; }
        if (lk === 'd') { e.preventDefault(); menuHalfDown(); return; }
        if (lk === 'f') { e.preventDefault(); pageDown(); return; }
      }

      // 4. 服务端分页
      if (k === '[') { e.preventDefault(); sendAction('page', 'prev'); return; }
      if (k === ']') { e.preventDefault(); sendAction('page', 'next'); return; }

      // 5. 全局命令 fallback
      if (lk === 'n') { e.preventDefault(); sendCommand('new_thread'); return; }
      if (lk === 'r') { e.preventDefault(); sendCommand('reply'); return; }
      if (lk === 'e') { e.preventDefault(); sendCommand('edit_thread'); return; }
      if (lk === 'i') { e.preventDefault(); sendCommand('toggle_images'); return; }
      if (lk === 'p') { e.preventDefault(); sendCommand('profile'); return; }
      if (lk === 's') { e.preventDefault(); sendCommand('search'); return; }
      if (lk === 'b') { e.preventDefault(); sendAction('back', ''); return; }
      if (lk === 'q') { e.preventDefault(); sendQuit(); return; }
      if (k === 'H' || k === '?') { e.preventDefault(); sendCommand('help'); return; }
      if (k === 'G') { e.preventDefault(); toggleLang(); return; }
    }
  });

  connect();
})();
