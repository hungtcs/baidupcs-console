// PWA 只求「能安装」：浏览器要求页面注册一个带 fetch 处理器的 service worker。
// 这里刻意不缓存任何东西，所有请求照常走网络——后端升级后前端绝不会命中旧资源。
self.addEventListener("install", () => {
  // 装好就接管，不等待旧 worker 退休。
  void self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

// 不调用 respondWith，浏览器按默认方式请求网络；这个监听器只是用来满足
// 可安装性对 fetch 处理器的要求。
self.addEventListener("fetch", () => {});
