"use strict";

const maltenProfilePath = "/@malten";
const muMaltenProfile = "https://micro.mu/agent/malten";

function maltenProfileLink(label) {
    const link = document.createElement("a");
    link.href = maltenProfilePath;
    link.className = "profile-link";
    link.textContent = label;
    return link;
}

function linkMaltenMentions(root) {
    for (const link of root.querySelectorAll("a[href]")) {
        if (link.origin + link.pathname.replace(/\/$/, "") === muMaltenProfile && /^@?malten$/i.test(link.textContent.trim())) {
            link.href = maltenProfilePath;
            link.classList.add("profile-link");
        }
    }

    // Match the same standalone handle as server.MentionPrompt. Leave the
    // trailing delimiter for the next match when two mentions are adjacent.
    const mention = /(^|[\s\p{Ps}\p{Pi},"'])(@malten)(?=$|[\s\p{Pe}\p{Pf},:;!?"'…،؛؟]|\.(?=$|\s))/giu;
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node) {
        const text = node;
        node = walker.nextNode();
        if (text.parentElement.closest("a, code, pre")) continue;

        const parts = [];
        let end = 0;
        for (const match of text.data.matchAll(mention)) {
            const start = match.index + match[1].length;
            parts.push(text.data.slice(end, start), maltenProfileLink(match[2]));
            end = start + match[2].length;
        }
        if (parts.length) text.replaceWith(...parts, text.data.slice(end));
    }
}
