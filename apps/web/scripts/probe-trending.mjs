// 一次性探针：登录 admin 后拉 trending 榜单，打印 items/stale/top1。
// 用法: node apps/web/scripts/probe-trending.mjs
const BASE = process.env.BASE_BE || "http://127.0.0.1:7788";
const login = await fetch(BASE + "/api/v1/monitor/auth/login", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ grantType: "password", username: "admin", password: "123456", grant_type: "password" }),
});
const lj = await login.json();
const tok = (lj.data && (lj.data.accessToken || lj.data.access_token)) || lj.accessToken || lj.access_token;
if (!tok) {
  console.log("LOGIN FAIL", JSON.stringify(lj).slice(0, 200));
  process.exit(1);
}
const r = await fetch(BASE + "/api/v1/radar/trending?since=today", { headers: { Authorization: "Bearer " + tok } });
const j = await r.json();
// 响应是透传 protobuf JSON（字段在顶层，camelCase），不是 {data:{...}} 包络。
const items = j.items || (j.data && j.data.items) || [];
const stale = j.stale ?? (j.data && j.data.stale) ?? true;
console.log("board: HTTP", r.status, "items=" + items.length, "stale=" + stale);
if (items[0]) console.log("top1:", items[0].full_name || items[0].fullName);
