// The service worker that removes itself (SERVICE_WORKER=off): served as {base}/sw.js, it takes the place of web/sw.js
// in the browsers that have it (they ask for sw.js again at each visit), deletes its caches and unregisters. The pages it
// controls go to the network as without it until they close; none is reloaded (a navigation goes on).
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil((async () => {
  for (const k of await caches.keys()) if (k.startsWith("hm-")) await caches.delete(k);
  await self.clients.claim();
  await self.registration.unregister();
})()));
