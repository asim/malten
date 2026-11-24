"use strict";

const el = id => document.getElementById(id);
const params = new URLSearchParams(location.search);
let room = params.get("room") || "";
let invite = new URLSearchParams(location.hash.slice(1)).get("invite");
const inviteKey = "malten-invite-" + room;
if (invite && room) {
  sessionStorage.setItem(inviteKey, invite);
  history.replaceState(null, "", location.pathname + location.search);
} else if (room) {
  invite = sessionStorage.getItem(inviteKey);
}
let current;
let busy = false;
let loading = false;
let expiryTimer;
let lastInvites;
let lastMessages;

function errorMessage(text = "") {
  el("error").textContent = text;
  el("error").hidden = !text;
}

async function request(path, data) {
  const options = data ? {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(data)} : {};
  const response = await fetch("/session/" + path, {cache: "no-store", ...options});
  if (response.status === 204) return null;
  if (!response.ok) {
    const error = new Error((await response.text()).trim() || "Couldn’t load the session.");
    error.status = response.status;
    error.posted = response.headers.get("X-Malten-Posted") === "1";
    throw error;
  }
  return response.json();
}

function button(text, handler) {
  const node = document.createElement("button");
  node.type = "button";
  node.className = "secondary";
  node.textContent = text;
  node.onclick = handler;
  return node;
}

function showInvites(invites) {
  const key = JSON.stringify(invites);
  if (key === lastInvites) return;
  lastInvites = key;
  el("guests").hidden = !invites.length;
  el("seats").replaceChildren(...invites.map((token, i) => {
    const li = document.createElement("li");
    li.textContent = "Guest " + (i + 1) + (token ? "" : " · Joined");
    if (token) {
      const url = new URL("/session", location.origin);
      url.searchParams.set("room", room);
      url.hash = new URLSearchParams({invite: token});
      const copy = button("Copy invite", async () => {
        try {
          await navigator.clipboard.writeText(url.href);
          copy.textContent = "Copied";
        } catch {
          let input = li.querySelector("input");
          if (!input) { input = document.createElement("input"); input.readOnly = true; input.setAttribute("aria-label", "Guest invite link"); li.append(input); }
          input.value = url.href;
          input.select();
        }
      });
      li.append(copy);
    }
    return li;
  }));
}

function showMessages(messages) {
  const key = JSON.stringify(messages);
  if (key === lastMessages) return;
  lastMessages = key;
  el("messages").replaceChildren(...messages.map(m => {
    const li = document.createElement("li");
    const name = document.createElement(m.author === "Malten" ? "a" : "strong");
    if (m.author === "Malten") name.href = "https://micro.mu/agent/malten";
    name.textContent = m.author;
    const time = document.createElement("time");
    const at = new Date(m.created * 1000);
    time.dateTime = at.toISOString();
    time.textContent = at.toLocaleTimeString([], {hour: "2-digit", minute: "2-digit"});
    const text = document.createElement(m.html && m.author === "Malten" ? "div" : "p");
    if (m.html && m.author === "Malten") {
      // The server renders agent Markdown with raw HTML and unsafe URLs disabled.
      text.className = "agent-reply";
      text.innerHTML = m.html;
    } else { text.textContent = m.text; }
    li.append(name, time, text);
    return li;
  }));
}

function render(data) {
  current = data;
  room = data.room;
  const active = data.status === "active";
  const paid = data.status === "paid";
  const pending = data.status === "pending";
  const unconfirmed = data.status === "unconfirmed";
  const end = new Date((data.ends_at || 0) * 1000);
  el("invitation").hidden = el("home").hidden = true;
  el("room").hidden = false;
  el("title").textContent = "Private session with Malten";
  const states = {
    unconfirmed: "Your booking is saved in this browser. Check again to get the price.",
    pending: "Awaiting payment. Return here after checkout to confirm your booking.",
    paid: data.host ? "Payment confirmed. Start when everyone is ready." : "Your place is saved. The host will start the session.",
    active: "This session ends at " + end.toLocaleTimeString([], {hour: "2-digit", minute: "2-digit", timeZoneName: "short"}) + ".",
    expired: "This session has ended."
  };
  el("status").textContent = states[data.status];
  el("quote").hidden = !(data.host && data.amount && (pending || paid));
  if (!el("quote").hidden) {
    const price = new Intl.NumberFormat(undefined, {style: "currency", currency: data.currency});
    el("quote").textContent = data.minutes + " minutes · " + data.participants + (data.participants === 1 ? " place" : " places") +
      " at " + price.format(data.amount / data.participants / 100) + " each · " + price.format(data.amount / 100) + " total";
  }
  el("checkout").hidden = !data.checkout;
  if (data.checkout) el("checkout").href = data.checkout;
  else el("checkout").removeAttribute("href");
  el("refresh").hidden = !(data.host && (pending || unconfirmed));
  el("refresh").textContent = unconfirmed ? "Get price" : "Check payment";
  el("start").hidden = !(data.host && paid);
  el("chat").hidden = !active;
  if (!active) el("message").value = "";
  showInvites(data.invites || []);
  showMessages(active ? (data.messages || []) : []);
  clearTimeout(expiryTimer);
  if (active) {
    const delay = Math.max(0, (data.ends_at - data.now) * 1000);
    expiryTimer = setTimeout(() => render({...data, status: "expired", invites: [], messages: []}), delay);
  }
}

function selectRoom(data) {
  history.replaceState(null, "", "/session?" + new URLSearchParams({room: data.room}));
  render(data);
}

async function act(task) {
  if (busy) return;
  busy = true;
  document.querySelectorAll("form button, #claim, #refresh, #start").forEach(b => b.disabled = true);
  errorMessage();
  try { await task(); }
  catch (error) { errorMessage(error.message); }
  finally {
    busy = false;
    document.querySelectorAll("form button, #claim, #refresh, #start").forEach(b => b.disabled = false);
  }
}

el("book").onsubmit = event => {
  event.preventDefault();
  act(async () => {
    const participants = Number(el("places").value);
    const key = "malten-checkout-" + participants;
    const ref = sessionStorage.getItem(key) || crypto.randomUUID();
    sessionStorage.setItem(key, ref);
    const data = await request("book", {room: ref, participants});
    sessionStorage.removeItem(key);
    selectRoom(data);
  });
};
el("refresh").onclick = () => act(async () => render(await request("refresh", {room})));
el("start").onclick = () => act(async () => render(await request("start", {room})));
el("claim").onclick = () => act(async () => {
  const data = await request("claim", {room, invite});
  sessionStorage.removeItem(inviteKey);
  invite = null;
  selectRoom(data);
});
el("compose").onsubmit = event => {
  event.preventDefault();
  const ask = event.submitter?.value === "ask";
  act(async () => {
    const text = el("message").value.trim();
    if (!text) return;
    if (new TextEncoder().encode(text).length > 4096) throw new Error("Write a message of up to 4,096 UTF-8 bytes.");
    try {
      const data = await request("post", {room, text, ask});
      if (el("message").value.trim() === text) el("message").value = "";
      render(data);
    } catch (error) {
      if (error.posted && el("message").value.trim() === text) el("message").value = "";
      throw error;
    }
  });
};

async function load() {
  const returned = params.get("session");
  if (room && invite) {
    el("title").textContent = "Join a private session";
    el("status").textContent = "Claim your place on this browser. You can come back to it during the session.";
    el("invitation").hidden = false;
    return;
  }
  if (returned || room) {
    const data = returned ? await request("return", {payment: returned}) : await request("state?" + new URLSearchParams({room}));
    selectRoom(data);
    if (!returned && data.host && ["paid", "pending", "unconfirmed"].includes(data.status)) {
      render(await request("refresh", {room}));
    }
    return;
  }
  const data = await request("state");
  el("status").textContent = data.available ? "Book time with Malten, on your own or with guests. The clock starts when you press Start." : "Booking is currently unavailable.";
  el("home").hidden = false;
  el("book").hidden = !data.available;
  el("your-bookings").hidden = !data.rooms?.length;
  for (const b of data.rooms || []) {
    const li = document.createElement("li");
    const a = document.createElement("a");
    a.href = "/session?" + new URLSearchParams({room: b.room});
    a.textContent = (b.host ? "Hosting" : "Guest") + " · " + b.participants + (b.participants === 1 ? " place" : " places") + " · " + b.status;
    li.append(a);
    el("bookings").append(li);
  }
}
async function poll() {
  if (loading || busy || document.hidden || !room || !["active", "paid"].includes(current?.status)) return;
  loading = true;
  const previous = current;
  try {
    const data = await request("state?" + new URLSearchParams({room, since: previous.revision}));
    if (data && previous === current && !busy) { errorMessage(); render(data); }
  } catch (error) {
    if (error.status === 404) render({...previous, status: "expired", invites: [], messages: []});
    else errorMessage(error.message);
  } finally { loading = false; }
}
load().catch(error => { if (!current) el("status").textContent = ""; errorMessage(error.message); });
setInterval(poll, 5000);
document.addEventListener("visibilitychange", poll);
window.addEventListener("pageshow", event => { if (event.persisted) location.reload(); });
