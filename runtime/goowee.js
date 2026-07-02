const nodeMap = {};
let root = null;

// Hydration: capture nodes from SSR-rendered HTML
const preexistingNodes = {};
document.querySelectorAll("[data-node-id]").forEach(el => {
    const id = parseInt(el.getAttribute("data-node-id"), 10);
    preexistingNodes[id] = el;
});

function findNodeID(el) {
    while (el) {
        if (el._nodeID !== undefined) return el._nodeID;
        el = el.parentElement;
    }
    return -1;
}

const EVENT_PROPS = ["clientX","clientY","button","key","code","value","ctrlKey","shiftKey","altKey","metaKey"];

document.addEventListener("click", e => {
    const nodeID = findNodeID(e.target);
    if (nodeID === -1) return;
    const data = {};
    for (const p of EVENT_PROPS) {
        if (typeof e[p] !== "undefined") data[p] = e[p];
    }

    // Prevent default navigation for <a> clicks handled by Go
    const anchor = e.target.closest("a");
    if (anchor && anchor._nodeID !== undefined) {
        e.preventDefault();
    }

    handleEvent(nodeID, "click", JSON.stringify(data));
});

document.addEventListener("input", e => {
    const nodeID = findNodeID(e.target);
    if (nodeID === -1) return;
    const target = e.target;
    let data;
    if (target.type === "checkbox" || target.type === "radio") {
        data = {value: target.checked ? "on" : "off", checked: target.checked};
    } else {
        data = {value: target.value};
    }
    handleEvent(nodeID, "input", JSON.stringify(data));
});

document.addEventListener("submit", e => {
    const nodeID = findNodeID(e.target);
    if (nodeID === -1) return;
    e.preventDefault();
    const form = e.target;
    const formData = {};
    for (const el of form.elements) {
        if (el.name) {
            if (el.type === "checkbox" || el.type === "radio") {
                formData[el.name] = el.checked ? "on" : "off";
            } else {
                formData[el.name] = el.value;
            }
        }
    }
    handleEvent(nodeID, "submit", JSON.stringify({values: formData}));
});

// Scroll events don't bubble — use capture phase
document.addEventListener("scroll", e => {
    const nodeID = findNodeID(e.target);
    if (nodeID === -1) return;
    const target = e.target;
    handleEvent(nodeID, "scroll", JSON.stringify({
        scrollTop: target.scrollTop, scrollLeft: target.scrollLeft
    }));
}, true);

function applyMutations(json) {
    const muts = JSON.parse(json);
    let firstNodeID = null;
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
                if (firstNodeID === null) firstNodeID = mut.nodeId;
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
                if (el) el[mut.key] = mut.value;
                break;
            case 4: // AppendChild
                const parent = nodeMap[mut.nodeId];
                const child = nodeMap[mut.childId];
                if (parent && child && !child.parentNode) {
                    parent.appendChild(child);
                }
                break;
            case 5: // InsertBefore
                const parent = nodeMap[mut.nodeId];
                const child = nodeMap[mut.childId];
                const idx = parseInt(mut.key);
                const ref = !isNaN(idx) ? parent.children[idx] : null;
                if (parent && child) {
                    parent.insertBefore(child, ref);
                }
                break;
        }
    }
    if (firstNodeID !== null && nodeMap[firstNodeID] && nodeMap[firstNodeID].parentNode === null) {
        if (!root) root = document.getElementById("root");
        if (root) root.appendChild(nodeMap[firstNodeID]);
    }
}
