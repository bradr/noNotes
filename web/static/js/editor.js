import { basicSetup, EditorView } from "https://esm.sh/codemirror@6.0.1";
import { Decoration, ViewPlugin } from "https://esm.sh/@codemirror/view@6.0.1";
import { RangeSetBuilder, StateField, StateEffect } from "https://esm.sh/@codemirror/state@6.0.1";
import { search, SearchQuery, setSearchQuery, getSearchQuery } from "https://esm.sh/@codemirror/search@6.5.6";
import { markdown } from "https://esm.sh/@codemirror/lang-markdown@6.1.1";






let editor;
let timelineData = [];
let fullText = "";
let filteredLineMap = [];

let saveTimeout;
let lastSaveLocal = 0;
let isRemoteUpdate = false;

// searchMatchDeco for search highlights - use inline styles for guaranteed visibility
const searchMatchDeco = Decoration.mark({ 
    attributes: { style: "background-color: #ffd54f !important; color: #000 !important; border-radius: 3px; font-weight: 500;" },
    class: "cm-searchMatch" 
});


// searchHighlighter uses CodeMirror's own SearchQuery to find matches
const searchHighlighter = ViewPlugin.fromClass(class {
    constructor(view) {
        this.decorations = this.getDecos(view);
    }
    update(update) {
        if (update.docChanged || update.transactions.some(tr => tr.effects.some(e => e.is(setSearchQuery)))) {
            this.decorations = this.getDecos(update.view);
        }
    }
    getDecos(view) {
        const query = getSearchQuery(view.state);
        console.log("Search Query in Editor:", query?.search);
        if (!query || !query.valid || !query.search) return Decoration.none;

        
        const builder = new RangeSetBuilder();
        const cursor = query.getCursor(view.state);
        while (true) {
            const { value, done } = cursor.next();
            if (done) break;
            builder.add(value.from, value.to, searchMatchDeco);
        }
        return builder.finish();
    }
}, {
    decorations: v => v.decorations
});

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

    editor = new EditorView({
        doc: fullText,
        extensions: [
            basicSetup,
            markdown(),
            search({ top: true }),
            searchHighlighter,
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
    }, 1000);
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
        // Dispatch the official setSearchQuery effect to trigger built-in highlighting
        const queryObj = new SearchQuery({ search: query, caseSensitive: false, literal: true });
        editor.dispatch({
            changes: { from: 0, to: editor.state.doc.length, insert: textToShow },
            effects: setSearchQuery.of(queryObj)
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
            item.innerHTML = `<span class="history-time">${new Date(entry.timestamp * 1000).toLocaleString()}</span>`;
            item.onclick = () => console.log("Diff logic omitted for simplicity");
            list.appendChild(item);
        });
    };
    document.getElementById('history-close').onclick = () => modal.style.display = 'none';
}
