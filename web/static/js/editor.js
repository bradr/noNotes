import { basicSetup, EditorView } from "https://esm.sh/codemirror@6.0.2?deps=@codemirror/view@6.38.4,@codemirror/state@6.5.2";
import { Decoration, ViewPlugin } from "https://esm.sh/@codemirror/view@6.38.4?deps=@codemirror/state@6.5.2";
import { StateField, StateEffect, RangeSetBuilder } from "https://esm.sh/@codemirror/state@6.5.2";
import { markdown } from "https://esm.sh/@codemirror/lang-markdown@6.3.4?deps=@codemirror/view@6.38.4,@codemirror/state@6.5.2";

const setSearchTerm = StateEffect.define();

const searchTermField = StateField.define({
    create: () => "",
    update(value, tr) {
        for (const e of tr.effects) if (e.is(setSearchTerm)) return e.value;
        return value;
    }
});

const searchMatchDeco = Decoration.mark({
    class: "cm-searchMatch",
    attributes: { style: "background-color: #ffd54f; color: #000; border-radius: 2px;" }
});

function buildSearchDecos(view) {
    const term = view.state.field(searchTermField);
    if (!term) return Decoration.none;
    const needle = term.toLowerCase();
    const builder = new RangeSetBuilder();
    for (const { from, to } of view.visibleRanges) {
        const text = view.state.doc.sliceString(from, to).toLowerCase();
        let i = 0;
        while ((i = text.indexOf(needle, i)) !== -1) {
            builder.add(from + i, from + i + needle.length, searchMatchDeco);
            i += needle.length || 1;
        }
    }
    return builder.finish();
}

const searchHighlighter = ViewPlugin.fromClass(class {
    constructor(view) { this.decorations = buildSearchDecos(view); }
    update(update) {
        const termChanged = update.state.field(searchTermField) !== update.startState.field(searchTermField);
        if (update.docChanged || update.viewportChanged || termChanged) {
            this.decorations = buildSearchDecos(update.view);
        }
    }
}, { decorations: v => v.decorations });

const checkboxClicker = EditorView.domEventHandlers({
    mousedown(event, view) {
        if (event.button !== 0 || event.altKey || event.metaKey || event.ctrlKey || event.shiftKey) return false;

        if (filteredLineMap.length > 0) {
            const p = view.posAtCoords({ x: event.clientX, y: event.clientY });
            if (p == null) return false;
            const editorLineNum = view.state.doc.lineAt(p).number;
            const fileLineNum = filteredLineMap[editorLineNum - 1];
            if (fileLineNum) {
                event.preventDefault();
                window.jumpToLine(fileLineNum);
                return true;
            }
            return false;
        }

        const pos = view.posAtCoords({ x: event.clientX, y: event.clientY });
        if (pos == null) return false;
        const line = view.state.doc.lineAt(pos);
        const idx = line.text.search(/\[[ x]\]/);
        if (idx < 0) return false;
        const boxStart = line.from + idx;
        const boxEnd = boxStart + 3;
        if (pos < boxStart || pos > boxEnd) return false;
        const current = view.state.doc.sliceString(boxStart + 1, boxStart + 2);
        const next = current === ' ' ? 'x' : ' ';
        view.dispatch({
            changes: { from: boxStart + 1, to: boxStart + 2, insert: next }
        });

        const editorLineNum = line.number;
        const fileLineNum = filteredLineMap.length > 0
            ? filteredLineMap[editorLineNum - 1]
            : editorLineNum;
        const taskItem = document.querySelector(`.task-item[data-line="${fileLineNum}"]`);
        if (taskItem) {
            if (next === 'x') {
                taskItem.style.pointerEvents = 'none';
                taskItem.style.opacity = '0.4';
                taskItem.style.transform = 'translateX(20px)';
                setTimeout(() => taskItem.remove(), 400);
            } else {
                taskItem.classList.remove('done');
                const cb = taskItem.querySelector('input[type=checkbox]');
                if (cb) cb.checked = false;
            }
        }

        event.preventDefault();
        return true;
    }
});






let editor;
let timelineData = [];
let fullText = "";
let filteredLineMap = [];

let saveTimeout;
let lastSaveLocal = 0;
let isRemoteUpdate = false;

let searchQuery = "";






// ─── Command Palette ─────────────────────────────────────────────────────────

const paletteItems = [
    { label: "Checklist", prefix: "- [ ] ", icon: "□" },
    { label: "Heading 1", prefix: "# ", icon: "H1" },
    { label: "Heading 2", prefix: "## ", icon: "H2" },
    { label: "Heading 3", prefix: "### ", icon: "H3" },
    { label: "Bullet List", prefix: "- ", icon: "•" },
    { label: "Blockquote", prefix: "> ", icon: "“" },
    { label: "Code Block", prefix: "```\n\n```", offset: 4, icon: "</>" }
];

let paletteActive = false;
let paletteIndex = 0;
let paletteEl = null;

function setupCommandPalette() {
    paletteEl = document.createElement('div');
    paletteEl.className = 'command-palette';
    paletteEl.style.display = 'none';
    document.body.appendChild(paletteEl);
}

function updatePaletteUI() {
    if (!paletteEl) return;
    paletteEl.innerHTML = '';

    paletteItems.forEach((item, i) => {
        const div = document.createElement('div');
        div.className = `palette-item ${i === paletteIndex ? 'active' : ''}`;

        const icon = document.createElement('span');
        icon.className = 'palette-icon';
        icon.textContent = item.icon;

        const label = document.createElement('span');
        label.className = 'palette-label';
        label.textContent = item.label;

        div.appendChild(icon);
        div.appendChild(label);

        div.onmousedown = (e) => {
            e.preventDefault();
            paletteIndex = i;
            applyPaletteSelection(editor);
        };
        paletteEl.appendChild(div);
    });
}

function hidePalette() {
    paletteActive = false;
    if (paletteEl) paletteEl.style.display = 'none';
}

document.addEventListener('mousedown', (e) => {
    if (paletteActive && paletteEl && !paletteEl.contains(e.target)) {
        hidePalette();
    }
});

function applyPaletteSelection(view) {
    if (!paletteActive || !view) return;

    const head = view.state.selection.main.head;
    const line = view.state.doc.lineAt(head);
    const slashPos = head - 1;
    const item = paletteItems[paletteIndex];

    if (item.offset) {
        view.dispatch({
            changes: { from: slashPos, to: head, insert: item.prefix },
            selection: { anchor: slashPos + item.offset },
            userEvent: 'input.type'
        });
    } else {
        view.dispatch({
            changes: [
                { from: slashPos, to: head, insert: "" },
                { from: line.from, to: line.from, insert: item.prefix }
            ],
            selection: { anchor: Math.max(0, head - 1 + item.prefix.length) },
            userEvent: 'input.type'
        });
    }
    hidePalette();
    view.focus();
}

// ─── Timeline / Saving ───────────────────────────────────────────────────────

async function loadTasks() {
    try {
        const res = await fetch('/tasks');
        const html = await res.text();
        const list = document.getElementById('task-list');
        if (list) {
            list.innerHTML = html;
        }
    } catch (err) {
        console.error('Failed to load tasks:', err);
    }
}

async function loadTimeline(initialLoad = false) {
    try {
        const res = await fetch('/timeline', { cache: 'no-store' });
        const data = await res.json();
        timelineData = Array.isArray(data) ? data : [];
        window.timelineData = timelineData;
        fullText = timelineData.map(r => (r.text || "").replace(/\r/g, "")).join('\n');
    } catch (err) {
        console.error("Failed to load timeline:", err);
    }

    if (initialLoad) {
        initEditor();
        setupHoverTooltip();
    } else {
        updateEditorFromRemote();
    }
    if (window.fetchActivity) window.fetchActivity();
}

function initEditor() {
    const container = document.getElementById('editor-container');
    if (!container) return;
    if (editor) { editor.destroy(); editor = null; }

    editor = new EditorView({
        doc: fullText,
        extensions: [
            basicSetup,
            markdown(),
            searchTermField,
            searchHighlighter,
            checkboxClicker,
            EditorView.lineWrapping,
            EditorView.updateListener.of((update) => {
                if (update.docChanged && !isRemoteUpdate) {
                    scheduleSave();
                }

                if (isRemoteUpdate) return;

                const isFiltering = !document.getElementById('side-panel').classList.contains('closed')
                    && document.getElementById('search-input').value !== '';

                if (isFiltering) return;

                const head = update.state.selection.main.head;
                const line = update.state.doc.lineAt(head);
                const textBefore = line.text.substring(0, head - line.from);

                if (textBefore.endsWith('/')) {
                    const coords = editor.coordsAtPos(head);
                    if (coords) {
                        if (!paletteActive) {
                            paletteActive = true;
                            paletteIndex = 0;
                            updatePaletteUI();
                        }
                        paletteEl.style.display = 'block';
                        paletteEl.style.left = coords.left + 'px';
                        paletteEl.style.top = (coords.bottom + 5) + 'px';
                    }
                } else if (paletteActive) {
                    hidePalette();
                }
            })
        ],
        parent: container
    });
    window.editor = editor;

    window.addEventListener('keydown', (e) => {
        if (!paletteActive) return;
        if (e.key === "ArrowDown") {
            e.preventDefault();
            e.stopImmediatePropagation();
            paletteIndex = (paletteIndex + 1) % paletteItems.length;
            updatePaletteUI();
        } else if (e.key === "ArrowUp") {
            e.preventDefault();
            e.stopImmediatePropagation();
            paletteIndex = (paletteIndex - 1 + paletteItems.length) % paletteItems.length;
            updatePaletteUI();
        } else if (e.key === "Enter") {
            e.preventDefault();
            e.stopImmediatePropagation();
            applyPaletteSelection(editor);
        } else if (e.key === "Escape") {
            e.preventDefault();
            e.stopImmediatePropagation();
            hidePalette();
        }
    }, true);

    setTimeout(() => {
        editor.dispatch({
            selection: { anchor: editor.state.doc.length },
            scrollIntoView: true
        });
    }, 100);
}

function scheduleSave() {
    lastSaveLocal = Date.now();
    clearTimeout(saveTimeout);
    saveTimeout = setTimeout(async () => {
        await fetch('/update', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: `text=${encodeURIComponent(editor.state.doc.toString())}`
        });
    }, 300);
}

function updateEditorFromRemote() {
    if (!editor) return;
    if (Date.now() - lastSaveLocal > 5000) {
        const localText = editor.state.doc.toString();
        if (localText === fullText) return;

        let from = 0, to = localText.length;
        let insert = fullText;

        let prefixLen = 0;
        const minLen = Math.min(localText.length, fullText.length);
        while (prefixLen < minLen && localText[prefixLen] === fullText[prefixLen]) {
            prefixLen++;
        }

        if (prefixLen > 0) {
            from = prefixLen;
            const remainingLocal = localText.substring(prefixLen);
            const remainingRemote = fullText.substring(prefixLen);
            let suffixLen = 0;
            const minRem = Math.min(remainingLocal.length, remainingRemote.length);
            while (suffixLen < minRem && remainingLocal[remainingLocal.length - 1 - suffixLen] === remainingRemote[remainingRemote.length - 1 - suffixLen]) {
                suffixLen++;
            }
            to = localText.length - suffixLen;
            insert = fullText.substring(prefixLen, fullText.length - suffixLen);
        }

        const anchor = editor.state.selection.main.anchor;
        const scrollTop = editor.scrollDOM.scrollTop;
        isRemoteUpdate = true;
        editor.dispatch({
            changes: { from, to, insert },
            selection: { anchor: Math.min(anchor, fullText.length) }
        });
        isRemoteUpdate = false;
        requestAnimationFrame(() => {
            if (editor.scrollDOM) editor.scrollDOM.scrollTop = scrollTop;
        });
    }
}

// ─── Hover tooltip ────────────────────────────────────────────────────────────

function setupHoverTooltip() {
    const tooltip = document.createElement('div');
    tooltip.id = 'line-tooltip';
    document.body.appendChild(tooltip);

    tooltip.onclick = () => {
        const lineNum = tooltip.getAttribute('data-line');
        if (lineNum && window.openDateModal) window.openDateModal(parseInt(lineNum));
    };

    const container = document.getElementById('editor-container');
    if (!container) return;

    container.addEventListener('mousemove', (e) => {
        if (!editor || !timelineData.length) return;
        const el = document.elementFromPoint(e.clientX, e.clientY);
        if (!el || !el.closest('.cm-line')) { tooltip.style.display = 'none'; return; }

        const pos = editor.posAtCoords({ x: e.clientX, y: e.clientY });
        if (pos == null) { tooltip.style.display = 'none'; return; }

        try {
            const editorLine = editor.state.doc.lineAt(pos).number;
            const lineNum = filteredLineMap.length > 0 ? filteredLineMap[editorLine - 1] : editorLine;
            const lineText = editor.state.doc.line(editorLine).text;
            if (!lineText.trim()) { tooltip.style.display = 'none'; return; }

            const match = timelineData.find(r => r.line_num === lineNum);
            if (match && match.text === lineText) {
                tooltip.textContent = new Date(match.timestamp * 1000).toLocaleDateString('en-US', {
                    year: 'numeric', month: 'short', day: 'numeric'
                });
                tooltip.setAttribute('data-line', lineNum);
                const coords = editor.coordsAtPos(editor.lineBlockAt(pos).from);
                tooltip.style.display = 'block';
                tooltip.style.right = (window.innerWidth - container.getBoundingClientRect().right + 20) + 'px';
                tooltip.style.top = coords.top + 'px';
            } else {
                tooltip.style.display = 'none';
            }
        } catch (e) { tooltip.style.display = 'none'; }
    });
    container.addEventListener('mouseleave', (e) => {
        if (e.relatedTarget === tooltip) return;
        tooltip.style.display = 'none';
    });
    tooltip.onmouseleave = () => { tooltip.style.display = 'none'; };
}

// ─── Search filtering ────────────────────────────────────────────────────────

function setupSearchOverlay() {
    const input = document.getElementById('search-input');
    const count = document.getElementById('search-count');

    function applyFilter(query) {
        if (!editor) return;
        const container = document.getElementById('editor-container');
        filteredLineMap = [];
        let textToShow = fullText;
        let totalMatches = 0;

        if (query) {
            searchQuery = query;
            const lines = fullText.split('\n');
            const matched = [];
            lines.forEach((l, i) => {
                if (l.toLowerCase().includes(query.toLowerCase())) {
                    matched.push(l);
                    filteredLineMap.push(i + 1);
                }
            });
            textToShow = matched.join('\n');
            totalMatches = matched.length;
            container.classList.add('filtering-active');
        } else {
            searchQuery = "";
            container.classList.remove('filtering-active');
        }

        isRemoteUpdate = true;
        editor.dispatch({
            changes: { from: 0, to: editor.state.doc.length, insert: textToShow },
            effects: setSearchTerm.of(query || "")
        });
        isRemoteUpdate = false;
        count.textContent = query ? `${totalMatches} results` : '';
        
        // Also refresh tasks with filtering
        if (window.htmx) {
            htmx.ajax('GET', '/tasks?q=' + encodeURIComponent(query), { target: '#task-list', swap: 'innerHTML' });
        }
    }

    input.addEventListener('input', () => applyFilter(input.value));
    window.clearSearch = () => { input.value = ''; applyFilter(''); };
}

// ─── SSE / History ───────────────────────────────────────────────────────────

try {
    const evtSource = new EventSource('/events');
    evtSource.addEventListener('update', () => {
        loadTimeline(false);
        loadTasks();
    });
} catch (e) { }

setupSearchOverlay();
setupCommandPalette();
setupHistoryModal();
loadTimeline(true);

window.scrollToLine = (lineNum) => {
    if (!editor) return;
    try {
        const line = editor.state.doc.line(lineNum);
        editor.dispatch({
            selection: { anchor: line.from },
            effects: [EditorView.scrollIntoView(line.from, { y: 'start' })]
        });
        editor.contentDOM.focus();
    } catch (e) { }
};

window.jumpToLine = (fileLineNum) => {
    if (!editor) return;
    const input = document.getElementById('search-input');
    const count = document.getElementById('search-count');
    const container = document.getElementById('editor-container');
    if (input) input.value = '';
    if (count) count.textContent = '';
    container?.classList.remove('filtering-active');
    filteredLineMap = [];
    searchQuery = '';

    const lines = fullText.split('\n');
    let pos = 0;
    for (let i = 0; i < fileLineNum - 1 && i < lines.length; i++) {
        pos += lines[i].length + 1;
    }

    isRemoteUpdate = true;
    editor.dispatch({
        changes: { from: 0, to: editor.state.doc.length, insert: fullText },
        selection: { anchor: pos },
        effects: [setSearchTerm.of(""), EditorView.scrollIntoView(pos, { y: 'start' })]
    });
    isRemoteUpdate = false;

    requestAnimationFrame(() => {
        try {
            const line = editor.state.doc.line(fileLineNum);
            editor.dispatch({
                effects: EditorView.scrollIntoView(line.from, { y: 'start' })
            });
            editor.contentDOM.focus();
        } catch (e) { }
    });

    if (window.htmx) {
        htmx.ajax('GET', '/tasks', { target: '#task-list', swap: 'innerHTML' });
    }
};

window.toggleTaskInEditor = (fileLineNum) => {
    if (!editor) return;
    const editorLineNum = filteredLineMap.length > 0
        ? filteredLineMap.indexOf(fileLineNum) + 1
        : fileLineNum;
    if (editorLineNum < 1) return;
    let line;
    try { line = editor.state.doc.line(editorLineNum); } catch (e) { return; }
    let newText;
    if (line.text.includes("[ ]")) newText = line.text.replace("[ ]", "[x]");
    else if (line.text.includes("[x]")) newText = line.text.replace("[x]", "[ ]");
    else return;

    editor.dispatch({
        changes: { from: line.from, to: line.to, insert: newText }
    });

    const lines = fullText.split("\n");
    if (lines[fileLineNum - 1] !== undefined) {
        lines[fileLineNum - 1] = newText;
        fullText = lines.join("\n");
    }
};

window.addNewNote = () => {
    if (!editor) return;
    if (window.clearSearch) window.clearSearch();
    const docLen = editor.state.doc.length;
    const content = editor.state.doc.toString();
    let insertText = content.endsWith("\n\n") ? "" : (content.endsWith("\n") ? "\n" : "\n\n");
    editor.dispatch({
        changes: { from: docLen, insert: insertText },
        selection: { anchor: docLen + insertText.length },
        effects: [EditorView.scrollIntoView(docLen + insertText.length, { y: 'start' })]
    });
    editor.contentDOM.focus();
};

function setupHistoryModal() {
    const btnHistory = document.getElementById('btn-history');
    const modal = document.getElementById('history-modal');
    const list = document.getElementById('history-list');
    if (!btnHistory || !modal) return;

    btnHistory.onclick = async () => {
        modal.style.display = 'flex';
        list.innerHTML = '<div class="empty-state">Loading history...</div>';
        const res = await fetch('/history');
        const data = await res.json();
        list.innerHTML = '';
        data.forEach(entry => {
            const item = document.createElement('div');
            item.className = 'history-item';

            const escHtml = s => s.replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
            const previewLines = (entry.preview || []).map(l => {
                const isAdd = l.startsWith('+');
                const color = isAdd ? 'var(--accent, #4caf50)' : '#f44336';
                const escaped = l.replace(/</g, '&lt;').replace(/>/g, '&gt;');
                return `<div style="color:${color};font-size:11px;font-family:monospace;white-space:pre-wrap;">${escaped}</div>`;
            }).join('');

            item.innerHTML = `
                <div class="history-item-header">
                    <span class="history-time">${new Date(entry.timestamp * 1000).toLocaleString()}</span>
                    <span style="display:flex;align-items:center;gap:8px;">
                        <span class="history-stats">
                            ${entry.additions ? `<span class="stat-add">+${entry.additions}</span>` : ''}
                            ${entry.deletions ? `<span class="stat-del">-${entry.deletions}</span>` : ''}
                        </span>
                        <button class="btn-revert" data-hash="${entry.hash}">Revert</button>
                    </span>
                </div>
                ${entry.subject ? `<div style="font-size:12px;color:var(--text-muted);margin-top:4px;">${escHtml(entry.subject)}</div>` : ''}
                ${previewLines ? `<div class="history-preview-list">${previewLines}</div>` : ''}`;

            item.querySelector('.btn-revert').onclick = async (e) => {
                e.stopPropagation();
                if (!confirm('Revert this change? This will undo that specific commit.')) return;
                const btn = e.currentTarget;
                btn.textContent = 'Reverting…';
                btn.disabled = true;
                const r = await fetch('/revert', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: `hash=${entry.hash}`
                });
                if (r.ok) {
                    modal.style.display = 'none';
                    loadTimeline(false);
                } else {
                    const msg = await r.text();
                    btn.textContent = 'Revert';
                    btn.disabled = false;
                    alert(msg);
                }
            };

            item.style.cursor = 'pointer';
            let diffEl = null;
            item.onclick = async () => {
                if (diffEl) { diffEl.remove(); diffEl = null; return; }
                diffEl = document.createElement('div');
                diffEl.className = 'history-item-diff';
                diffEl.innerHTML = '<div style="padding:8px;color:var(--text-muted);font-size:12px;">Loading diff…</div>';
                item.appendChild(diffEl);
                const r = await fetch(`/diff?hash=${entry.hash}`);
                const text = await r.text();
                const escaped = text.replace(/</g, '&lt;').replace(/>/g, '&gt;');
                const colored = escaped.split('\n').map(l => {
                    if (l.startsWith('+') && !l.startsWith('+++')) return `<span style="color:#4caf50">${l}</span>`;
                    if (l.startsWith('-') && !l.startsWith('---')) return `<span style="color:#f44336">${l}</span>`;
                    return `<span style="color:var(--text-muted)">${l}</span>`;
                }).join('\n');
                diffEl.innerHTML = `<pre class="diff-content-inner">${colored}</pre>`;
            };

            list.appendChild(item);
        });
    };
    document.getElementById('history-close').onclick = () => modal.style.display = 'none';
}
