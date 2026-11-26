"use strict";

function replyQuote(quote, parent, href) {
    const block = document.createElement("blockquote");
    block.className = "reply-quote";
    if (!quote || (quote.expires && quote.expires <= Date.now())) {
        block.textContent = "Original reflection is no longer available.";
        return block;
    }
    block.dataset.parent = parent;
    if (quote.expires) block.dataset.expires = quote.expires;
    const content = document.createElement(href ? "a" : "div");
    if (href) content.href = href;
    const label = document.createElement("span");
    label.className = "reply-label";
    label.textContent = "In reply to " + (quote.author || "a reflection");
    const excerpt = document.createElement("span");
    excerpt.dir = "auto";
    const chars = Array.from(quote.text);
    excerpt.textContent = chars.length > 180 ? chars.slice(0, 179).join("") + "…" : quote.text;
    content.append(label, excerpt);
    block.append(content);
    return block;
}

function replyButton(select) {
    const button = document.createElement("button");
    button.className = "message-action";
    button.type = "button";
    button.textContent = "Reply";
    button.onclick = select;
    return button;
}

function replyComposer(panel, textarea) {
    const preview = panel.querySelector(".reply-preview");
    let parent = "";
    function clear() {
        parent = "";
        preview.replaceChildren();
        panel.hidden = true;
    }
    panel.querySelector("button").onclick = () => {
        clear();
        textarea.focus();
    };
    return {
        get parent() { return parent; },
        select(id, quote) {
            parent = id;
            preview.replaceChildren(replyQuote(quote, id));
            panel.hidden = false;
            textarea.focus();
            panel.scrollIntoView({block: "nearest"});
        },
        unavailable() { preview.replaceChildren(replyQuote(null)); },
        clear
    };
}
