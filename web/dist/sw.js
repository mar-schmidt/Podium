// Podium service worker: renders OS-level Web Push notifications for
// attention-required agent events, and routes taps back to the right session.
//
// De-dup rule: if a dashboard window is currently visible, the in-app toast
// already covers the event, so we forward it to the page and skip the OS
// notification. When no window is visible (tab backgrounded or closed) we show
// the system notification — the whole point of Web Push.

self.addEventListener("install", () => {
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
  event.waitUntil(handlePush(event));
});

async function handlePush(event) {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch (_e) {
    data = {};
  }

  const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
  const visible = windows.find((c) => c.visibilityState === "visible");
  if (visible) {
    // A dashboard tab is in front; let the in-app toast handle it.
    visible.postMessage({ type: "push-preview", session_id: data.session_id, kind: data.kind });
    return;
  }

  await self.registration.showNotification(data.title || "Podium", {
    body: data.body || "",
    tag: data.session_id || "podium",
    renotify: true,
    icon: "/favicon.svg",
    badge: "/favicon.svg",
    data,
  });
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  event.waitUntil(focusSession(event.notification.data || {}));
});

async function focusSession(data) {
  const sessionId = data.session_id;
  const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
  for (const client of windows) {
    if ("focus" in client) {
      await client.focus();
      client.postMessage({ type: "notification-click", session_id: sessionId });
      return;
    }
  }
  if (self.clients.openWindow) {
    await self.clients.openWindow("/");
  }
}
