const nodeMap = {};
let rootNode = null;

function getRoot() {
    if (!rootNode) rootNode = document.getElementById("root");
    return rootNode;
}

const preexistingNodes = {};
let hydrated = false;

// Collect server-rendered nodes for reuse, the first time mutations are
// applied. This is deferred rather than run at module load because this
// script sits in <head>, before <div id="root"> (and its SSR content) exists;
// by the first applyMutations the WASM app has booted and the DOM is present.
// For a pure client app (no SSR) there simply are no nodes to claim.
function hydrateOnce() {
    if (hydrated) return;
    hydrated = true;
    const root = getRoot();
    if (!root) return;

    root.querySelectorAll("[data-node-id]").forEach(el => {
        preexistingNodes[parseInt(el.getAttribute("data-node-id"), 10)] = el;
    });

    // Text nodes can't carry attributes; SSR marks each with a preceding
    // <!--g{id}--> comment. Claim the comment's next sibling, then drop the
    // marker so the live DOM matches the client tree.
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_COMMENT);
    const markers = [];
    while (walker.nextNode()) {
        const m = /^g(\d+)$/.exec(walker.currentNode.data);
        if (m) markers.push([parseInt(m[1], 10), walker.currentNode]);
    }
    for (const [id, comment] of markers) {
        const next = comment.nextSibling;
        if (next && next.nodeType === 3 /* TEXT_NODE */) {
            preexistingNodes[id] = next;
        }
        comment.remove();
    }
}

const listening = {};
window.goListen = function (type, capture) {
    if (listening[type]) return;
    listening[type] = true;
    document.addEventListener(type, e => dispatchToGo(type, e), capture);
};

function buildPayload(type, e) {
    switch (type) {
        case "click": case "dblclick":
        case "pointerdown": case "pointerup": case "pointermove":
            return {clientX: e.clientX, clientY: e.clientY, button: e.button,
                    ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, altKey: e.altKey, metaKey: e.metaKey};
        case "input": case "change": {
            const t = e.target;
            if (t.type === "checkbox" || t.type === "radio")
                return {value: t.value, checked: t.checked};
            return {value: t.value};
        }
        case "submit": {
            const values = {};
            for (const el of e.target.elements) {
                if (!el.name) continue;
                values[el.name] = (el.type === "checkbox" || el.type === "radio")
                    ? (el.checked ? "on" : "off") : el.value;
            }
            return {values};
        }
        case "keydown": case "keyup":
            return {key: e.key, code: e.code,
                    ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, altKey: e.altKey, metaKey: e.metaKey};
        case "scroll":
            return {scrollTop: e.target.scrollTop, scrollLeft: e.target.scrollLeft};
        default:
            return {};
    }
}

function dispatchToGo(type, e) {
	const payload = JSON.stringify(buildPayload(type, e));
	let el = e.target;
	while (el) {
		if (el._nodeID !== undefined) {
			const r = handleEvent(el._nodeID, type, payload);
			if (r && r.handled) {
				if (r.preventDefault) e.preventDefault();
				if (r.stopPropagation) e.stopPropagation();
				if (r.selectOnFocus && type === "focus") {
					el.select();
				}
				return;
			}
		}
		el = el.parentElement;
	}
}

window.applyMutations = function applyMutations(json) {
    hydrateOnce();
    const muts = JSON.parse(json);
    for (const mut of muts) {
        let el;
        switch (mut.type) {
            case 0: // CreateElement
                if (preexistingNodes[mut.nodeId]) {
                    el = preexistingNodes[mut.nodeId];
                    delete preexistingNodes[mut.nodeId];
                } else if (mut.value === "#text") {
                    el = document.createTextNode("");
                } else {
                    el = document.createElement(mut.value);
                }
                el._nodeID = mut.nodeId;
                nodeMap[mut.nodeId] = el;
                break;
            case 1: // RemoveNode
                el = nodeMap[mut.nodeId];
                if (el && el.parentNode) el.parentNode.removeChild(el);
                delete nodeMap[mut.nodeId];
                break;
            case 2: // SetAttribute
                el = nodeMap[mut.nodeId];
                if (el) el.setAttribute(mut.key, mut.value);
                break;
		case 3: // SetProperty
				el = nodeMap[mut.nodeId];
				if (el) {
					// Preserve cursor position when a signal update sets the
					// value of a focused input — skip the DOM write so the
					// cursor doesn't jump to the end while the user types.
					if (mut.key === "value" && el === document.activeElement) {
						break;
					}
					el[mut.key] = mut.value;
				}
				break;
            case 4: { // AppendChild
                const parent = mut.nodeId === 0 ? getRoot() : nodeMap[mut.nodeId];
                const child = nodeMap[mut.childId];
                if (parent && child && !child.parentNode) {
                    parent.appendChild(child);
                }
                break;
            }
            case 5: { // InsertBefore
                const parent = mut.nodeId === 0 ? getRoot() : nodeMap[mut.nodeId];
                const child = nodeMap[mut.childId];
                const ref = mut.refId ? nodeMap[mut.refId] : null;
                if (parent && child) {
                    parent.insertBefore(child, ref);
                }
                break;
            }
            case 6: { // RemoveAttribute
                el = nodeMap[mut.nodeId];
                if (el) el.removeAttribute(mut.key);
                break;
            }
            case 7: // Hydrate — claim a server-rendered node by id
                el = preexistingNodes[mut.nodeId];
                if (el) {
                    delete preexistingNodes[mut.nodeId];
                } else {
                    // No server node for this id (SSR/client divergence). Fall
                    // back to a fresh node so we don't crash; it will be bare.
                    console.warn("goowee: hydration miss for node", mut.nodeId, mut.value);
                    el = mut.value === "#text"
                        ? document.createTextNode("")
                        : document.createElement(mut.value);
                }
                el._nodeID = mut.nodeId;
                nodeMap[mut.nodeId] = el;
                break;
        }
    }
};
