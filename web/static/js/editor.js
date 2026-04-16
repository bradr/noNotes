import { basicSetup, EditorView } from "https://esm.sh/codemirror@6.0.1";
import { keymap } from "https://esm.sh/@codemirror/view@6.0.1";
import { markdown } from "https://esm.sh/@codemirror/lang-markdown@6.1.1";

let editor;
let timelineData = [];
let fullText = "";
let filteredLineMap = [];

let saveTimeout;
let lastSaveLocal = 0;
let isRemoteUpdate = false;

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
let palettePos = null;
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
            e.preventDefault(); // Prevents editor from losing focus
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
        // Block-style insert (like Code Block), replace the trigger "/"
        view.dispatch({
            changes: { from: slashPos, to: head, insert: item.prefix },
            selection: { anchor: slashPos + item.offset },
            userEvent: 'input.type'
        });
    } else {
        // Line-style prefix (Heading, Checklist, etc.)
        view.dispatch({
            changes: [
                { from: slashPos, to: head, insert: "" },
                { from: line.from, to: line.from, insert: item.prefix }
            ],
            // Adjust selection to maintain relative position
            selection: { anchor: Math.max(0, head - 1 + item.prefix.length) },
            userEvent: 'input.type'
        });
    }
    hidePalette();
    view.focus();
}

// ─── Timeline load ────────────────────────────────────────────────────────────

async function loadTimeline(initialLoad = false) {
    try {
        const res = await fetch('/timeline', { cache: 'no-store' });
        const data = await res.json();
        timelineData = Array.isArray(data) ? data : [];
        window.timelineData = timelineData;
        fullText = timelineData.map(r => r.text || "").join('\n');
    } catch (err) {
        console.error("Failed to load timeline:", err);
    }

    if (initialLoad) {
        initEditor();
        setupHoverTooltip();
    } else {
        updateEditorFromRemote();
    }
}

function initEditor() {
    const container = document.getElementById('editor-container');
    if (!container) return;

    editor = new EditorView({
        doc: fullText,
        extensions: [
            basicSetup,
            markdown(),
            EditorView.lineWrapping,
            EditorView.updateListener.of((update) => {
                if (update.docChanged && !isRemoteUpdate) {
                    scheduleSave();
                }

                if (isRemoteUpdate) return;

                const isFiltering = !document.getElementById('side-panel').classList.contains('closed')
                    && document.getElementById('search-input').value !== '';

                if (isFiltering) return;

                // Command Palette Logic
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
                        paletteEl.style.top = coords.bottom + 'px';
                    }
                } else if (paletteActive) {
                    hidePalette();
                }
            }),
            EditorView.domEventHandlers({
                mousedown(e, view) {
                    if (filteredLineMap.length > 0) {
                        const pos = view.posAtCoords({ x: e.clientX, y: e.clientY });
                        if (pos == null) return false;
                        const line = view.state.doc.lineAt(pos);
                        const originalLineNum = filteredLineMap[line.number - 1];
                        console.log("Clicked filtered line:", line.number, "Mapping to original:", originalLineNum);
                        if (originalLineNum) {
                            if (window.clearSearch) {
                                window.clearSearch();
                                // Wait for editor to update before scrolling
                                setTimeout(() => {
                                    window.scrollToLine(originalLineNum);
                                }, 50);
                                return true;
                            }
                        }
                    }
                    const pos = view.posAtCoords({ x: e.clientX, y: e.clientY });
                    if (pos == null) return false;
                    const line = view.state.doc.lineAt(pos);
                    // Match pattern: optional whitespace, then "- [ ]" or "- [x]"
                    const match = /^\s*(-\s+\[([ x])\])/.exec(line.text);
                    if (match) {
                        const lineBase = view.coordsAtPos(line.from);
                        const relativeX = e.clientX - lineBase.left;
                        // Check if click is in the first 60px of the line
                        if (lineBase && relativeX > 0 && relativeX < 60) {
                            const isChecked = match[2] === 'x';
                            const bracketIndex = line.text.indexOf('[');
                            const togglePos = line.from + bracketIndex + 1;
                            view.dispatch({
                                changes: { from: togglePos, to: togglePos + 1, insert: isChecked ? " " : "x" }
                            });
                            return true; // Prevent default CM behavior
                        }
                    }
                }
            })
        ],
        parent: container
    });

    window.editor = editor;
    
    // Reliable keyboard interception for Command Palette and Tab indentation
    editor.contentDOM.addEventListener('keydown', (e) => {
        // Handle Tab indentation (always active)
        if (e.key === "Tab") {
            e.preventDefault();
            e.stopImmediatePropagation();
            
            const isShift = e.shiftKey;
            const view = editor;
            const { state } = view;
            const changes = [];
            const lines = new Set();
            
            for (const range of state.selection.ranges) {
                const startLine = state.doc.lineAt(range.from).number;
                const endLine = state.doc.lineAt(range.to).number;
                for (let i = startLine; i <= endLine; i++) lines.add(i);
            }
            
            for (const lineNum of lines) {
                const line = state.doc.line(lineNum);
                if (isShift) {
                    // Outdent: remove up to 4 spaces or 1 tab from the start
                    let toRemove = 0;
                    if (line.text.startsWith("    ")) toRemove = 4;
                    else if (line.text.startsWith("\t")) toRemove = 1;
                    else {
                        const match = line.text.match(/^ +/);
                        if (match) toRemove = Math.min(match[0].length, 4);
                    }
                    if (toRemove > 0) {
                        changes.push({ from: line.from, to: line.from + toRemove });
                    }
                } else {
                    // Indent: add 4 spaces at the start of the line
                    changes.push({ from: line.from, insert: "    " });
                }
            }
            
            if (changes.length > 0) {
                view.dispatch({ 
                    changes, 
                    userEvent: isShift ? "input.outdent" : "input.indent",
                    scrollIntoView: true 
                });
            }
            return;
        }

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

    // Auto-scroll to bottom
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
    const isFiltering = !document.getElementById('side-panel').classList.contains('closed')
        && document.getElementById('search-input').value !== '';
    if (isFiltering) return;

    // Only update if we haven't typed for 5 seconds to avoid race conditions
    if (Date.now() - lastSaveLocal > 5000) {
        const localText = editor.state.doc.toString();
        if (localText === fullText) return;

        // If the only difference is trailing newlines at the end of the file, 
        // trust the local version to avoid "flickering" out new lines.
        if (localText.startsWith(fullText) && localText.substring(fullText.length).match(/^\n+$/)) {
            return;
        }

        const anchor = editor.state.selection.main.anchor;
        const scrollTop = editor.scrollDOM.scrollTop;
        isRemoteUpdate = true;
        editor.dispatch({
            changes: { from: 0, to: editor.state.doc.length, insert: fullText },
            selection: { anchor: Math.min(anchor, fullText.length) }
        });
        isRemoteUpdate = false;

        requestAnimationFrame(() => {
            if (editor.scrollDOM) {
                editor.scrollDOM.scrollTop = scrollTop;
            }
        });
    }
}

// ─── Hover tooltip ────────────────────────────────────────────────────────────

function setupHoverTooltip() {
    const tooltip = document.createElement('div');
    tooltip.id = 'line-tooltip';
    tooltip.style.cursor = 'pointer';
    tooltip.title = 'Click to edit date';
    document.body.appendChild(tooltip);

    tooltip.onclick = () => {
        const lineNum = tooltip.getAttribute('data-line');
        if (lineNum && window.openDateModal) {
            window.openDateModal(parseInt(lineNum));
        }
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

            // Ignore empty lines for hover info
            if (!lineText.trim()) { tooltip.style.display = 'none'; return; }

            const match = timelineData.find(r => r.line_num === lineNum);

            // To be robust, let's verify text still matches (account for pending saves)
            if (match && match.text === lineText) {
                tooltip.textContent = new Date(match.timestamp * 1000).toLocaleDateString('en-US', {
                    year: 'numeric', month: 'short', day: 'numeric'
                });
                tooltip.setAttribute('data-line', lineNum);
                const coords = editor.coordsAtPos(editor.lineBlockAt(pos).from);
                tooltip.style.display = 'block';
                tooltip.style.right = (window.innerWidth - container.getBoundingClientRect().right + 20) + 'px';
                tooltip.style.top = coords.top + 'px';
                tooltip.style.pointerEvents = 'auto';
            } else {
                tooltip.style.display = 'none';
            }
        } catch (e) { tooltip.style.display = 'none'; }
    });
    // Don't hide on mouseleave if we are hovering over the tooltip itself
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
    const sidePanel = document.getElementById('side-panel');

    function clearSearch() {
        input.value = '';
        applyFilter('');
    }

    // Expose globally so the close-panel button in index.html can clear filter too
    window.clearSearch = clearSearch;

    function applyFilter(query) {
        if (!editor) return;
        const container = document.getElementById('editor-container');
        const isCurrentlyFiltering = container.classList.contains('filtering-active');

        if (!query && !isCurrentlyFiltering) return; // Nothing to do

        let textToShow = fullText;
        let totalMatches = 0;
        filteredLineMap = [];

        if (query) {
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
            container.classList.remove('filtering-active');
        }

        const anchor = editor.state.selection.main.anchor;
        isRemoteUpdate = true;
        editor.dispatch({
            changes: { from: 0, to: editor.state.doc.length, insert: textToShow },
            selection: { anchor: Math.min(anchor, textToShow.length) }
        });
        isRemoteUpdate = false;

        count.textContent = query ? `${totalMatches} results` : '';
    }

    input.addEventListener('input', () => applyFilter(input.value));
    input.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            clearSearch();
            if (editor) editor.contentDOM.focus();
        }
    });

    window.addEventListener('keydown', (e) => {
        if ((e.metaKey || e.ctrlKey) && e.key === 'f') {
            e.preventDefault();
            // Open side panel if closed
            if (sidePanel) sidePanel.classList.remove('closed');
            input.focus();
            input.select();
        }
    });
}

// ─── SSE ─────────────────────────────────────────────────────────────────────

try {
    const evtSource = new EventSource('/events');
    evtSource.addEventListener('update', () => {
        loadTimeline(false);
        const taskList = document.getElementById('task-list');
        if (taskList && window.htmx) htmx.trigger(taskList, 'update-tasks');
    });
} catch (e) { }

// ─── Boot ───────────────────────────────────────────────────────────────────

setupSearchOverlay();
setupCommandPalette();
loadTimeline(true);

window.scrollToLine = (lineNum) => {
    if (!editor) return;
    try {
        const line = editor.state.doc.line(lineNum);
        editor.dispatch({
            selection: { anchor: line.from },
            effects: [
                EditorView.scrollIntoView(line.from, { y: 'start' })
            ],
            userEvent: 'select'
        });
        editor.contentDOM.focus();
    } catch (e) {
        console.error("Scroll to line failed", e);
    }
};

window.addNewNote = () => {
    if (!editor) return;

    // Clear search if active
    if (window.clearSearch) {
        window.clearSearch();
    }

    const docLen = editor.state.doc.length;
    const content = editor.state.doc.toString();
    let insertText = "\n\n";

    // Adjust based on current ending
    if (content.endsWith("\n\n")) insertText = "";
    else if (content.endsWith("\n")) insertText = "\n";
    else if (docLen === 0) insertText = "";

    const newPos = docLen + insertText.length;
    editor.dispatch({
        changes: { from: docLen, insert: insertText },
        selection: { anchor: newPos },
        effects: [
            EditorView.scrollIntoView(newPos, { y: 'start' })
        ]
    });

    editor.contentDOM.focus();
};
